# KNet

## Description

`KNet` is a Go library for exchanging packets between nodes over gRPC, with a command layer on top of the transport. Nodes register with a password and are divided into the roles SERVER and CLIENT; the server acts as a hub and forwards packets addressed to other nodes. Events are written to the journal via [`KJournal`](https://github.com/KrigMoon/KJournal).

## Settings

The behavior of `KNet` is defined by `KNSettings`, which is passed to `Run`.

| Name | Type | Default | Description |
|---|---|---|---|
| `Journal` | `*KJournal.KJournal` | — (required) | Shared journal |
| `Core` | `KNetCore.KNCSettings` | — | Network core settings: `Journal`, `MyNode`, `Password` |
| `ExternalCommandsMap` | `map[string]struct{}` | empty map | Names of commands executed by an external integrator via `ExternalPrograms()`/`WriteResponse()` |
| `AutoRun` | `[]string` | `nil` | List of commands executed via `Execute` at startup; after each one — a 200 ms pause |
| `FileStorageDir` | `string` | `"./"` → absolute path | File storage directory; used by `CreateFileCollector` and the `/file` command |
| `RemoteControl` | `bool` | `false` | Allows receiving `PackageType_COMMAND` from the network and executing them via `Execute` |

`KNetCore.KNCSettings` describes the node and the core's shared resources: `Journal`, `MyNode` (identifier, role `SERVER`/`CLIENT`, listening address), and `Password` for `Register`.

## Lifecycle

The state model: `OFF → RUNNING → ON → STOP → OFF`. Control is via `ctx/cancel`, `once.Once` (one-time stop), and `sync.WaitGroup`.

### Startup

`KNet.Run` checks the status (if `status != OFF` — exit), saves the settings, initializes `ctx`, `stopOnce`, `awaitStop` and calls `Journal.NewWriter()`. Then it creates `cursor = CursorNil`, the channels `cursorQueue` (512) and `responses` (256), the command map `GetCommandsMap(FileStorageDir)` and moves the status to RUNNING. After that, `KNetCore.Run` initializes its own `ctx/stopOnce/awaitStop`, calls `Journal.NewWriter()`, creates the channels `entry` (256) and `exit` (256), brings up the gRPC server (`runServer`) and starts `sender()`. In parallel, `KNet` moves the status to ON, starts `executerPrograms()` and `routePackages()`, calls `CreateFileCollector()` (an error is written to the journal but does not interrupt startup) and runs `AutoRun` with a 200 ms pause between commands.

### Shutdown

When `status == ON`, `KNet.Endwork` calls `cancel()` via `stopOnce.Do` and waits on `<-awaitStop`. In `stop()`, `KNet` first calls `knc.Endwork()`: `KNetCore` cancels `ctx` via its own `stopOnce`, moves the status to STOP, calls `RemovePeer` for all peers and waits for `processes.Wait()`. Then `KNetCore` performs `stopServer()` (`srv.GracefulStop()`), moves the status to OFF, calls `Journal.DelWriter()` and closes `awaitStop`. After returning from `knc.Endwork()`, `KNet` moves the status to STOP, waits for `processes.Wait()`, moves the status to OFF, calls `Journal.DelWriter()` and closes `awaitStop`.

## Interaction

### Commands

`KNet.Execute` uses `KCommandParser.Detect` to distinguish `InputCommand`, `InputProgram` and `InputUnknown` (→ error). In `ProgramModeSeq` mode, a separate cursor is created **for each command of each of the `Count` repetitions**: `Executer` = LOCAL if the name is in `localCommandsMap`, otherwise EXTERNAL if it is in `ExternalCommandsMap`, otherwise it accumulates in `errorBlock` (`unknown command`). In `ProgramModePar` mode, the commands are split into `loc`/`ext` once, `Executer` is chosen from them (LOCAL/EXTERNAL/BOTH; empty → `Unknown executer`), and for each `i` of `Count` a **new** `groupID` is created: clones via `CloneCommands`, all `item.ID = groupID`, `Wait = len(loc)+len(ext)`. The cursor is placed into `cursorQueue` (512) via `pasteCursor`; overflow → `cursor chan busy`. The list of commands and their contract is in [`Commands`](KNet/Commands/Commands.md).

### Responses

A response to a command is a `*KCommandParser.Response` linked to the command by its `ID`. The user reads responses non-blockingly via `KNet.Response()`: if the `responses` channel (256) is empty, `nil` is returned; otherwise — the next `*Response`. Responses from built-in commands are generated automatically when `Body` executes and land in `responses`; if the channel overflows, the response is dropped (`responses chan busy`). For external commands (`ExecuterExternal`, `ExecuterBoth`), the user receives them via `ExternalPrograms()`, executes them, and returns the result via `KNet.WriteResponse(resp)` with the same `CommandID` as the command.

### Packets

A packet is a `*knetpb.Package`. The user sends a packet via `KNetCore.Write(pkg)` (non-blocking; on overflow — WARNING) and receives them via readers: `AddReaderPackages(id, buff, rules)` registers a subscription with the rule `Rules(pkg) bool` (`rules == nil` → accept everything; `id == nil` → uuid; a repeated `id` does not overwrite), `DelReaderPackages(id)` removes the subscription, `Package(id)` reads the next packet non-blockingly. Incoming `PackageType_COMMAND` are handled separately: with `RemoteControl=true` and `DataType_STRING` they go to `Execute(pkg.GetText())`; with `RemoteControl=false` they are not processed. Other types (`MESSAGE`, `JSON`, `FILE`) are distributed to readers whose `Rules` returned `true`; if a reader's channel overflows — WARNING "The packet buffer is full".

### Files

Files are transferred by the `/file` command as `PackageType_FILE` packets with `DataType_BYTES` and `FileMeta` metadata (JSON in `Metadata`): the file is split into 4 KiB chunks, each sent as a separate packet. On the receiving side, `CreateFileCollector` registers a reader (rule: `DataType == BYTES` and `Type == FILE`) and assembles the file in the directory from `PrepareFileStorage(FileStorageDir)`. The collector requires a non-empty `FileStorageDir` and a set `MyNode`; if startup fails, an ERROR is written to the journal and startup is not interrupted. During assembly, `assembleAndSaveFile` orders chunks by `ChunkID`, verifies each chunk's `ChunkHash` and the file's `FileHash`, writes via `.part` and renames to the final name.

## Metrics

`KNet.GetMetrics()` returns `KNetMetrics` — a snapshot of the layer's state: `Status`, `Cursor` (ID, `Executer`, lengths of the local/external programs, `len/cap` of `cursorQueue`), `Responses` (`len/cap`), `Core` (`KNetCore` metrics), `ReadersCount` and `Readers` (ID, `len/cap` of channels). The getters `GetJournal()` and `GetInterface()` provide access to the journal and to `Commands.Interface` — the extension point for commands.

## Example

```go
journal := &KJournal.KJournal{}
journal.Run()
defer journal.EndWork()

node := &knetpb.Node{
	Id:            "node-1",
	Name:          "example",
	Type:          knetpb.TypeNode_CLIENT,
	ListeningIP:   "127.0.0.1",
	ListeningPort: 0,
}

var kn KNet.KNet
kn.Run(KNet.KNSettings{
	Journal:             journal,
	ExternalCommandsMap: map[string]struct{}{"/ping": {}},
	Core: KNetCore.KNCSettings{
		Journal:  journal,
		MyNode:   node,
		Password: "s3cret",
	},
	AutoRun:        nil,
	RemoteControl:  true,
	FileStorageDir: "./files",
})
defer kn.EndWork()

_ = kn.Execute(`/connect 127.0.0.1:9000 "s3cret"`)

if resp := kn.Response(); resp != nil {
	fmt.Printf("response cmd=%s: %s\n", resp.CommandID, resp.ResponseMessage)
}

readerID := kn.AddReaderPackages(nil, 256, func(pkg *knetpb.Package) bool {
	return pkg.Type != knetpb.PackageType_COMMAND
})
defer kn.DelReaderPackages(readerID)

if pkg := kn.Package(readerID); pkg != nil {
	fmt.Printf("package %s -> %s type=%s\n", pkg.From, pkg.To, pkg.Type)
}
```

---

 - This documentation is also available in [Russian](./src/docs/README.ru.md).

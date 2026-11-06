# KNet Commands

The module describes the built-in `KNet` commands, their arguments and methods. Commands are parsed by the [`KCommandParser`](https://github.com/KrigMoon/KCommandParser) library and executed via `CommandContainer`, which contains the functions `Help`, `Check` and `Body`.

## Commands

### `/connect`

```
/connect <peerAddress> <password>
```

Connects to a remote node and registers with a password.

| Argument | Type | Example | Description |
|---|---|---|---|
| peerAddress | Address | 192.168.1.10:8080 | Address of the remote node |
| password | String | "s3cret" | Password for registration |

### `/disconnect`

```
/disconnect <peerID>
```

Disconnects from a node and removes it from the peer list.

| Argument | Type | Example | Description |
|---|---|---|---|
| peerID | ID | $node-1 | Identifier of the remote node |

### `/send`

```
/send <peerID|-b> <data> [-m <metadata>]
```

Sends a text message (`PackageType_MESSAGE`). `-b` broadcasts to all known peers; `-m` attaches metadata.

| Argument | Type | Example | Description |
|---|---|---|---|
| peerID\|-b | ID \| Key | $node-1 | Recipient ID or `-b` for broadcast |
| data | String | "hello" | Message text |
| -m metadata | Key, String | -m "tag" | Optional metadata |

### `/json`

```
/json <peerID|-b> <json> [-m <metadata>]
```

Sends a JSON document (`PackageType_JSON`). The payload must be valid JSON.

| Argument | Type | Example | Description |
|---|---|---|---|
| peerID\|-b | ID \| Key | $node-1 | Recipient ID or `-b` for broadcast |
| json | String | `'{"a":1}'` | Valid JSON |
| -m metadata | Key, String | -m "tag" | Optional metadata |

### `/remote`

```
/remote <peerID|-b> <command> [-m <metadata>]
```

Sends a command (`PackageType_COMMAND`); executed on the receiver only when `RemoteControl=true`.

| Argument | Type | Example | Description |
|---|---|---|---|
| peerID\|-b | ID \| Key | $node-1 | Recipient ID or `-b` for broadcast |
| command | String | "/send node-2 hi" | Command string |
| -m metadata | Key, String | -m "tag" | Optional metadata |

### `/file`

```
/file <peerID> <path>
```

Sends a file in 4 KiB chunks (`PackageType_FILE`, `DataType_BYTES`); each chunk's `Metadata` holds a JSON `FileMeta`. Relative paths are resolved via `FileStorageDir` (`PrepareFileStorage`).

| Argument | Type | Example | Description |
|---|---|---|---|
| peerID | ID | $node-1 | Recipient ID |
| path | String | "docs/a.txt" | File path (relative to `FileStorageDir` or absolute) |

### `/help`

```
/help -<name>
```

Shows help for a KNet command, e.g. `/help -connect`.

| Argument | Type | Example | Description |
|---|---|---|---|
| -name | Key | -connect | Command name with a `-` prefix |

### `/sleep`

```
/sleep <seconds>
```

Pauses execution for the given number of seconds.

| Argument | Type | Example | Description |
|---|---|---|---|
| seconds | Number | 5 | Pause duration in seconds |

---

 - This documentation is also available in [Russian](../../src/docs/Commands.ru.md).
 - Back to [README](../../README.md).
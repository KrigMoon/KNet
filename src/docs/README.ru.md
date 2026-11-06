# KNet

## Описание

`KNet` — библиотека на Go для обмена пакетами между узлами по gRPC с командным слоем поверх транспорта. Узлы регистрируются по паролю и делятся на роли SERVER и CLIENT; сервер выступает хабом и пересылает пакеты, адресованные другим узлам. События пишутся в журнал через [`KJournal`](https://github.com/KrigMoon/KJournal).

## Настройки

Поведение `KNet` задаётся `KNSettings`, которая передаётся в `Run`.

| Название | Тип | По умолчанию | Описание |
|---|---|---|---|
| `Journal` | `*KJournal.KJournal` | — (обязательно) | Общий журнал |
| `Core` | `KNetCore.KNCSettings` | — | Настройки сетевого ядра: `Journal`, `MyNode`, `Password` |
| `ExternalCommandsMap` | `map[string]struct{}` | пустая карта | Имена команд, выполняемых внешним интегратором через `ExternalPrograms()`/`WriteResponse()` |
| `AutoRun` | `[]string` | `nil` | Список команд, выполняемых через `Execute` при старте; после каждой — пауза 200 мс |
| `FileStorageDir` | `string` | `"./"` → абсолютный путь | Каталог файлового хранилища; используется `CreateFileCollector` и командой `/file` |
| `RemoteControl` | `bool` | `false` | Разрешает принимать `PackageType_COMMAND` из сети и выполнять их через `Execute` |

`KNetCore.KNCSettings` описывает узел и общие ресурсы ядра: `Journal`, `MyNode` (идентификатор, роль `SERVER`/`CLIENT`, адрес прослушивания) и `Password` для `Register`.

## Жизненный цикл

Модель состояний: `OFF → RUNNING → ON → STOP → OFF`. Управление — через `ctx/cancel`, `once.Once` (однократная остановка) и `sync.WaitGroup`.

### Запуск

`KNet.Run` проверяет статус (если `status != OFF` — выход), сохраняет настройки, инициализирует `ctx`, `stopOnce`, `awaitStop` и вызывает `Journal.NewWriter()`. Затем создаёт `cursor = CursorNil`, каналы `cursorQueue` (512) и `responses` (256), карту команд `GetCommandsMap(FileStorageDir)` и переводит статус в RUNNING. После этого `KNetCore.Run` инициализирует свои `ctx/stopOnce/awaitStop`, вызывает `Journal.NewWriter()`, создаёт каналы `entry` (256) и `exit` (256), поднимает gRPC-сервер (`runServer`) и запускает `sender()`. Параллельно `KNet` переводит статус в ON, запускает `executerPrograms()` и `routePackages()`, вызывает `CreateFileCollector()` (ошибка пишется в журнал, но не прерывает старт) и выполняет `AutoRun` с паузой 200 мс между командами.

### Остановка

При `status == ON` `KNet.Endwork` через `stopOnce.Do` вызывает `cancel()` и ждёт `<-awaitStop`. В `stop()` `KNet` сначала вызывает `knc.Endwork()`: `KNetCore` отменяет `ctx` через свой `stopOnce`, переводит статус в STOP, вызывает `RemovePeer` для всех пиров и ждёт `processes.Wait()`. Затем `KNetCore` выполняет `stopServer()` (`srv.GracefulStop()`), переводит статус в OFF, вызывает `Journal.DelWriter()` и закрывает `awaitStop`. После возврата из `knc.Endwork()` `KNet` переводит статус в STOP, ждёт `processes.Wait()`, переводит статус в OFF, вызывает `Journal.DelWriter()` и закрывает `awaitStop`.

## Взаимодействие

### Команды

`KNet.Execute` через `KCommandParser.Detect` различает `InputCommand`, `InputProgram` и `InputUnknown` (→ ошибка). В режиме `ProgramModeSeq` отдельный курсор создаётся **на каждую команду каждой из `Count` репетиций**: `Executer` = LOCAL, если имя есть в `localCommandsMap`, иначе EXTERNAL, если есть в `ExternalCommandsMap`, иначе копится в `errorBlock` (`unknown command`). В режиме `ProgramModePar` команды один раз делятся на `loc`/`ext`, по ним выбирается `Executer` (LOCAL/EXTERNAL/BOTH; пусто → `Unknown executer`), и на каждую `i` из `Count` создаётся **новый** `groupID`: клоны через `CloneCommands`, всем `item.ID = groupID`, `Wait = len(loc)+len(ext)`. Курсор кладётся в `cursorQueue` (512) через `pasteCursor`; переполнение → `cursor chan busy`. Список команд и их контракт — в [`Commands`](./Commands.ru.md).

### Ответы

Ответ на команду — `*KCommandParser.Response`, связанный с командой по `ID`. Пользователь читает ответы неблокирующе через `KNet.Response()`: если канал `responses` (256) пуст — возвращается `nil`, иначе — следующий `*Response`. Ответы встроенных команд генерируются автоматически при выполнении `Body` и попадают в `responses`; при переполнении канала ответ отбрасывается (`responses chan busy`). Для внешних команд (`ExecuterExternal`, `ExecuterBoth`) пользователь получает их через `ExternalPrograms()`, выполняет и возвращает результат через `KNet.WriteResponse(resp)` с тем же `CommandID`, что у команды.

### Пакеты

Пакет — `*knetpb.Package`. Пользователь отправляет пакет через `KNetCore.Write(pkg)` (неблокирующе; при переполнении — WARNING) и принимает через ридеры: `AddReaderPackages(id, buff, rules)` регистрирует подписку с правилом `Rules(pkg) bool` (`rules == nil` → принимать всё; `id == nil` → uuid; повторный `id` не перезаписывает), `DelReaderPackages(id)` удаляет подписку, `Package(id)` неблокирующе читает следующий пакет. Входящие `PackageType_COMMAND` обрабатываются отдельно: при `RemoteControl=true` и `DataType_STRING` уходят в `Execute(pkg.GetText())`; при `RemoteControl=false` не обрабатываются. Остальные типы (`MESSAGE`, `JSON`, `FILE`) раздаются ридерам, чьи `Rules` вернули `true`; при переполнении канала ридера — WARNING «The packet buffer is full».

### Файлы

Файлы передаются командой `/file` пакетами `PackageType_FILE` с `DataType_BYTES` и метаданными `FileMeta` (JSON в `Metadata`): файл режется на чанки по 4 КиБ, каждый отправляется отдельным пакетом. На принимающей стороне `CreateFileCollector` регистрирует ридер (правило: `DataType == BYTES` и `Type == FILE`) и собирает файл в каталоге из `PrepareFileStorage(FileStorageDir)`. Коллектор требует непустой `FileStorageDir` и заданный `MyNode`; при ошибке старта пишется ERROR в журнал, старт не прерывается. При сборке `assembleAndSaveFile` упорядочивает чанки по `ChunkID`, проверяет `ChunkHash` каждого чанка и `FileHash` всего файла, пишет через `.part` и переименовывает в итоговое имя.

## Метрики

`KNet.GetMetrics()` возвращает `KNetMetrics` — снимок состояния слоя: `Status`, `Cursor` (ID, `Executer`, длины локальной/внешней программ, `len/cap` `cursorQueue`), `Responses` (`len/cap`), `Core` (метрики `KNetCore`), `ReadersCount` и `Readers` (ID, `len/cap` каналов). Геттеры `GetJournal()` и `GetInterface()` дают доступ к журналу и к `Commands.Interface` — точке расширения для команд.

## Пример

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

 - Данная документация доступна на [английском языке](../../README.md).

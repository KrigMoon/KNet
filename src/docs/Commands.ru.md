# KNet Команды

Модуль описывает встроенные команды `KNet`, их аргументы и методы. Команды разбираются библиотекой [`KCommandParser`](https://github.com/KrigMoon/KCommandParser) и выполняются через `CommandContainer`, который содержит функции `Help`, `Check` и `Body`.

## Команды

### `/connect`

```
/connect <peerAddress> <password>
```

Подключается к удалённому узлу и проходит регистрацию по паролю.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| peerAddress | Address | 192.168.1.10:8080 | Адрес удалённого узла |
| password | String | "s3cret" | Пароль для регистрации |

### `/disconnect`

```
/disconnect <peerID>
```

Отключается от узла и удаляет его из списка пиров.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| peerID | ID | $node-1 | Идентификатор удалённого узла |

### `/send`

```
/send <peerID|-b> <data> [-m <metadata>]
```

Отправляет текстовое сообщение (`PackageType_MESSAGE`). `-b` — рассылка всем известным пирам; `-m` — метаданные.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| peerID\|-b | ID \| Key | $node-1 | ID получателя или `-b` для рассылки |
| data | String | "hello" | Текст сообщения |
| -m metadata | Key, String | -m "tag" | Необязательные метаданные |

### `/json`

```
/json <peerID|-b> <json> [-m <metadata>]
```

Отправляет JSON-документ (`PackageType_JSON`). Полезная нагрузка должна быть валидным JSON.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| peerID\|-b | ID \| Key | $node-1 | ID получателя или `-b` для рассылки |
| json | String | `'{"a":1}'` | Валидный JSON |
| -m metadata | Key, String | -m "tag" | Необязательные метаданные |

### `/remote`

```
/remote <peerID|-b> <command> [-m <metadata>]
```

Отправляет команду (`PackageType_COMMAND`); на получателе выполняется только при `RemoteControl=true`.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| peerID\|-b | ID \| Key | $node-1 | ID получателя или `-b` для рассылки |
| command | String | "/send node-2 hi" | Строка команды |
| -m metadata | Key, String | -m "tag" | Необязательные метаданные |

### `/file`

```
/file <peerID> <path>
```

Отправляет файл чанками по 4 КиБ (`PackageType_FILE`, `DataType_BYTES`); `Metadata` каждого чанка содержит JSON `FileMeta`. Относительные пути разрешаются через `FileStorageDir` (`PrepareFileStorage`).

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| peerID | ID | $node-1 | ID получателя |
| path | String | "docs/a.txt" | Путь к файлу (относительно `FileStorageDir` или абсолютный) |

### `/help`

```
/help -<name>
```

Показывает справку по команде KNet, например `/help -connect`.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| -name | Key | -connect | Имя команды с префиксом `-` |

### `/sleep`

```
/sleep <seconds>
```

Приостанавливает выполнение на заданное число секунд.

| Аргумент | Тип | Пример | Описание |
|---|---|---|---|
| seconds | Number | 5 | Длительность паузы в секундах |

---

 - Данная документация доступна на [английском языке](../../KNet/Commands/Commands.md).
 - Назад в [README](./README.ru.md).
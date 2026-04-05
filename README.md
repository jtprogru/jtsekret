# jtsekret

CLI-утилита для централизованного и безопасного управления личными секретами: паролями, OAuth-токенами, API-ключами, токенами ботов.

Абстрагирует бэкенд хранилища за единым интерфейсом, реализует локальный шифрованный кэш и позволяет передавать секреты в другие процессы через Unix-пайп.

**Поддерживаемые бэкенды:** Yandex Cloud Lockbox

## Установка

```bash
git clone https://github.com/jtprogru/jtsekret
cd jtsekret
make install   # устанавливает в $GOPATH/bin
```

## Быстрый старт

```bash
# Создать конфиг
jtsekret config init

# Отредактировать ~/.config/jtsekret/jtsekret.yaml — указать folder_id и тип аутентификации

# Проверить подключение
YC_OAUTH_TOKEN=<token> jtsekret config health

# Получить список секретов
jtsekret list

# Получить значение ключа
jtsekret get my-api-token --key token

# Передать секрет в другой процесс через env-переменную
jtsekret exec --secret my-api-token --key token --env API_TOKEN -- curl https://api.example.com
```

## Конфигурация

Файл конфигурации ищется в следующем порядке:
1. `--config <path>` (флаг)
2. `~/.config/jtsekret/jtsekret.yaml`
3. `~/.jtsekret.yaml`
4. `./.jtsekret.yaml`

Любое значение можно переопределить через переменную окружения с префиксом `JTSEKRET_`
(например, `JTSEKRET_BACKEND_LOCKBOX_FOLDER_ID`).

### Аутентификация в Yandex Cloud

| `auth.type` | Описание | Переменная окружения |
|---|---|---|
| `oauth` | Постоянный OAuth-токен | `YC_OAUTH_TOKEN` |
| `iam_token` | Краткосрочный IAM-токен | `YC_IAM_TOKEN` |
| `service_account_key` | Ключ сервисного аккаунта (JSON-файл) | `YC_SERVICE_ACCOUNT_KEY_FILE` |
| `instance_service_account` | Метаданные ВМ Yandex Cloud | — |

Пример конфига с OAuth-токеном:

```yaml
backend:
  type: lockbox
  lockbox:
    folder_id: "b1g1234567890abcdefgh"
    auth:
      type: oauth
      # token: ""  # либо задайте через YC_OAUTH_TOKEN

cache:
  enabled: true
  ttl: 3600
  path: "~/.cache/jtsekret/cache.enc"
  # master_password: ""  # задавайте через JTSEKRET_CACHE_MASTER_PASSWORD

output:
  format: plain   # plain | table | json

log:
  level: warn
```

Полный пример со всеми опциями — `configs/jtsekret.example.yaml`.

## Команды

```
jtsekret list                            # список всех секретов
jtsekret get <name>                      # получить все ключи секрета
jtsekret get <name> --key <key>          # получить конкретный ключ
jtsekret get <name> --key <key> --raw    # только значение (для пайпа)
jtsekret set <name> <key> <value>        # добавить/обновить ключ
jtsekret create <name>                   # создать новый секрет
jtsekret delete <name>                   # удалить секрет
jtsekret exec --secret <name> --key <key> --env VAR -- <cmd> [args]
jtsekret exec --secret <name> --key <key> --stdin -- <cmd> [args]

jtsekret config init                     # создать файл конфига
jtsekret config show                     # показать текущий конфиг
jtsekret config validate                 # валидировать конфиг
jtsekret config health                   # проверить подключение к бэкенду

jtsekret dump <name>                     # сохранить все ключи секрета в файлы (в текущую папку)
jtsekret dump <name> --dir ~/.ssh        # сохранить в указанную директорию
jtsekret dump <name> --key id_rsa --output ~/.ssh/id_rsa  # конкретный ключ в конкретный файл
jtsekret dump <name> --key id_rsa --output -  # вывести в stdout

jtsekret cache status                    # статус кэша
jtsekret cache clear                     # очистить кэш
```

Глобальные флаги:

```
--config <path>        путь к файлу конфига
--output plain|table|json  формат вывода
--no-cache             не использовать кэш
--debug                включить debug-логирование
```

## Кэш

Секреты кэшируются локально в зашифрованном файле (AES-256-GCM, ключ выводится через Argon2id из мастер-пароля).

Мастер-пароль задаётся через переменную окружения `JTSEKRET_CACHE_MASTER_PASSWORD`. При отсутствии мастер-пароля кэш не используется.

## Сборка

```bash
make build            # текущая платформа
make build-linux      # linux/amd64
make build-darwin     # darwin/arm64
make test             # тесты с race detector
make lint             # golangci-lint
```

## Лицензия

MIT © Mikhail Savin

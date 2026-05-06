# jtsekret

CLI-утилита для централизованного и безопасного управления личными секретами: паролями, OAuth-токенами, API-ключами, токенами ботов.

Абстрагирует бэкенд хранилища за единым интерфейсом, реализует локальный шифрованный кэш и позволяет передавать секреты в другие процессы через Unix-пайп.

**Поддерживаемые бэкенды:** Yandex Cloud Lockbox, GitHub private repo (зашифрованные файлы в вашем приватном репозитории)

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

`YC_OAUTH_TOKEN` и `YC_IAM_TOKEN` — **разные токены, не взаимозаменяемы**:

| Токен | Что это | Срок | Где брать |
|---|---|---|---|
| `YC_OAUTH_TOKEN` | OAuth-токен Yandex Passport (общий аккаунт Yandex) | ~1 год | [oauth.yandex.ru/authorize](https://oauth.yandex.ru/authorize?response_type=token&client_id=1a6990aa636648e9b2ef855fa7bec2fb) |
| `YC_IAM_TOKEN` | IAM-токен, привязанный к Yandex Cloud | ~12 часов | `yc iam create-token` |

| `auth.type` | Поведение |
|---|---|
| `auto` (рекомендуется) | Резолв по цепочке: explicit `auth.token` → `YC_IAM_TOKEN` → `YC_OAUTH_TOKEN` → SA-key → `yc iam create-token` через локальный `yc` CLI. |
| `oauth` | Только OAuth-токен (Passport). |
| `iam_token` | Только IAM-токен. |
| `service_account_key` | JSON ключ сервисного аккаунта (`yc iam key create`). |
| `instance_service_account` | Метаданные ВМ Yandex Cloud. |

**Быстрый старт без ручных токенов:**

```bash
jtsekret login yc          # запускает yc init (браузерный OAuth)
jtsekret list              # auth.type=auto автоматически вызывает yc iam create-token
```

Пример конфига:

```yaml
backend:
  type: lockbox
  lockbox:
    folder_id: "b1g1234567890abcdefgh"
    auth:
      type: auto             # cм. таблицу выше

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

### GitHub private repo backend

Хранит секреты как зашифрованные файлы (`secrets/<name>.enc` + `secrets/<name>.meta.json`) в вашем приватном GitHub-репо. Шифрование — AES-256-GCM, ключ выводится Argon2id из мастер-пароля и per-secret salt. Repo-URL поддерживает форматы `owner/repo`, полный HTTPS/SSH или `file://` (для локальной синхронизации/тестов).

```yaml
backend:
  type: github
  github:
    repo: "jtprogru/my-secrets"
    branch: main
    local_path: "~/.cache/jtsekret/repo"
    auto_pull: true
    auto_push: true
    auth:
      type: token   # token | ssh | none
```

Мастер-пароль: `JTSEKRET_GITHUB_MASTER_PASSWORD` (fallback — `JTSEKRET_CACHE_MASTER_PASSWORD`). Токен GitHub: `JTSEKRET_GITHUB_TOKEN` (PAT с `contents:write` на репо).

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

jtsekret sync                            # явный pull+push (для github backend; no-op для lockbox)
jtsekret migrate --target-config <path>  # скопировать все секреты в другой бэкенд
jtsekret migrate --target-config <path> --update           # перезаписать существующие
jtsekret migrate --target-config <path> --dry-run          # показать план без записи
jtsekret migrate --target-config <path> --only name1,name2 # только эти секреты
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

# jtsekret

CLI-утилита для централизованного и безопасного управления личными секретами: паролями, OAuth-токенами, API-ключами, токенами ботов.

Абстрагирует бэкенд хранилища за единым интерфейсом, реализует локальный шифрованный кэш и позволяет передавать секреты в другие процессы через Unix-пайп.

**Поддерживаемые бэкенды:** Yandex Cloud Lockbox, GitHub private repo, HashiCorp Vault (KV v2), локальный `file` (полностью офлайн)

## Модель данных

Запоминается одна простая иерархия:

```
backend (где хранится)
└── secret (контейнер по имени, например "my-api-token")
    ├── entry: key="token"  → value=<bytes>
    ├── entry: key="user"   → value=<bytes>
    └── entry: key="…"      → value=<bytes>
```

- **secret** — именованный контейнер. Имя задаётся пользователем (`my-api-token`,
  `prod-db`, …). У каждого секрета есть версия: при перезаписи payload Lockbox/github
  заводит новую версию, старая остаётся.
- **entry** — пара `key → value` внутри секрета. У одного секрета может быть много
  entries: `username` + `password`, `token` + `refresh_token`, или один `id_rsa`.
- Флаг **`--key <name>`** во всех командах выбирает одну entry внутри секрета.
  Без `--key` команды работают со всеми entry секрета сразу.

## Установка

```bash
brew tap jtprogru/tap
brew install --cask jtsekret
```

Из исходников:

```bash
git clone https://github.com/jtprogru/jtsekret && cd jtsekret
make build                 # бинарь в ./jtsekret
go install ./...           # или установить в $GOBIN/$GOPATH/bin
```

## Быстрый старт

```bash
# 1. Сгенерировать стартовый конфиг (бэкенд по умолчанию = github private repo)
jtsekret config init

# 2a. Yandex Cloud Lockbox: разово авторизоваться через браузер
jtsekret login yc          # обёртка над `yc init`; auth.type=auto подхватит

# 2b. GitHub repo: задать PAT и мастер-пароль
export JTSEKRET_GITHUB_TOKEN=ghp_…
export JTSEKRET_GITHUB_MASTER_PASSWORD='…'

# 3. Создать секрет с одним полем
jtsekret create my-api-token --key token --value 'abc-123'

# 4. Достать значение конкретного поля (--raw без декораций — для пайпов)
jtsekret get my-api-token --key token --raw

# 5. Пробросить значение поля в дочерний процесс через env-переменную
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

### HashiCorp Vault backend

Поверх KV v2 secret engine. Каждый секрет → `<mount>/data/<prefix>/<name>`, его entries — это ключи в `data`. Версионирование берёт на себя сам Vault: `AddVersion` — это просто новый `Put`. Значения должны быть **UTF-8** — для бинарных данных используйте github/file (Vault хранит payload как JSON-строки).

```yaml
backend:
  type: vault
  vault:
    address: "https://vault.example.com:8200"   # или env VAULT_ADDR
    mount: "secret"
    prefix: "personal/"
    auth:
      type: token              # token | approle | userpass
      # token: ""              # prefer VAULT_TOKEN env var
```

Auth-методы:

| `auth.type` | Поля | Env-перегрузки |
|---|---|---|
| `token` | `token` | `VAULT_TOKEN` |
| `approle` | `role_id`, `secret_id`, `path` (default `approle`) | — |
| `userpass` | `username`, `password`, `path` (default `userpass`) | — |

TLS: `tls.ca_cert` (PEM) для self-hosted CA; `tls.insecure: true` отключает проверку сертификата (только для отладки).

### Локальный `file` backend

Полностью офлайн: на диске лежит та же раскладка, что у github-бэкенда (`<root>/secrets/<name>.{enc,meta.json}`), без git-слоя. Удобен для air-gapped-сценариев и как целевой бэкенд для `jtsekret migrate` (экспорт/импорт зашифрованного blob).

```yaml
backend:
  type: file
  file:
    path: "~/.local/share/jtsekret/store"
```

Мастер-пароль: `JTSEKRET_FILE_MASTER_PASSWORD` (fallback — `JTSEKRET_CACHE_MASTER_PASSWORD`).

## Команды

> Везде ниже `<name>` — имя **секрета** (контейнер), а `<key>` — имя **entry**
> (поля внутри секрета). См. раздел «Модель данных».

### Чтение

```
jtsekret list                                # список всех секретов
jtsekret search <pattern>                    # секреты, чьё имя содержит pattern (case-insensitive)
jtsekret search <pattern> --include-keys     # также матчить по именам entry-ключей

jtsekret get <name>                          # все entries секрета (key → value)
jtsekret get <name> --key <key>              # одно поле в формате "key: value"
jtsekret get <name> --key <key> --raw        # только значение, без декораций (для пайпа)
jtsekret get <name> --key <key> --copy       # копировать значение в буфер обмена и
                                             # очистить через 30s (--copy-clear-after N)
jtsekret get <name> --version <id>           # читать конкретную версию (если бэкенд её хранит)
```

`<name>` поддерживает tab-completion в `get`/`set`/`delete`/`dump`/`exec --secret`
(имена кэшируются на 5 минут в `~/.cache/jtsekret/completion.json`; create/set/delete
сбрасывают кеш автоматически).

### Запись

```
jtsekret create <name>                       # пустой секрет
jtsekret create <name> --key <k> --value <v> # секрет с одним полем (если --value не задан — спросит интерактивно)
jtsekret create <name> --desc "..." --label k=v --label k2=v2

jtsekret set <name> <key> <value>            # добавить новое поле или перезаписать существующее
                                              # (заводит новую версию секрета, старые поля сохраняются)

jtsekret delete <name>                       # удалить секрет целиком (с подтверждением)
```

### Запуск процессов с секретами

```
jtsekret exec --secret <name> --key <key> --env VAR -- <cmd> [args]
                # пробросить значение поля в env-переменную VAR дочернего процесса

jtsekret exec --secret <name> --key <key> --stdin -- <cmd> [args]
                # отправить значение поля на stdin дочернего процесса
```

### Дамп в файлы

```
jtsekret dump <name>                         # все поля секрета → файлы <key> в текущей папке
jtsekret dump <name> --dir ~/.ssh            # все поля → файлы в ~/.ssh
jtsekret dump <name> --key id_rsa --output ~/.ssh/id_rsa   # одно поле в конкретный файл
jtsekret dump <name> --key id_rsa --output -               # одно поле в stdout
```

### Конфиг и здоровье

```
jtsekret config init                         # создать файл конфига
jtsekret config show                         # показать текущий конфиг
jtsekret config validate                     # валидировать конфиг
jtsekret config health                       # проверить подключение к бэкенду
jtsekret login yc                            # запустить yc init (браузерная YC-аутентификация)
```

### Кэш

```
jtsekret cache status                        # статус кэша
jtsekret cache clear                         # очистить кэш
```

### Sync и миграция между бэкендами

```
jtsekret sync                                # явный pull+push (актуально для github; no-op для lockbox)

jtsekret migrate --target-config <path>      # скопировать все секреты в бэкенд из второго конфиг-файла
jtsekret migrate --target-config <path> --update           # перезаписать существующие в target
jtsekret migrate --target-config <path> --dry-run          # показать план без записи
jtsekret migrate --target-config <path> --only n1,n2       # только эти секреты
```

### Ротация мастер-пароля

```
jtsekret rotate-master                       # github/file: расшифровать всё текущим паролем
                                              # и пере-зашифровать новым (с новым per-secret salt).
                                              # lockbox/vault не имеют локального мастер-пароля,
                                              # команда отклоняется.
```

Текущий пароль берётся из `JTSEKRET_<BACKEND>_MASTER_PASSWORD` (с fallback'ом на `JTSEKRET_CACHE_MASTER_PASSWORD`); новый запрашивается интерактивно (дважды для подтверждения). После успешной ротации обновите env-переменную в шелле/keychain.

### macOS Keychain (вместо `JTSEKRET_*_MASTER_PASSWORD` env vars)

На macOS можно сохранить мастер-пароли в keychain — jtsekret подхватит их автоматически, если env var не задан. Слоты: `cache`, `github`, `file` (по одному на бэкенд с локальным шифрованием).

```
jtsekret keychain set <slot>     # запросит пароль интерактивно (дважды)
jtsekret keychain get <slot>     # прочитать (Keychain попросит подтверждение)
jtsekret keychain unset <slot>   # удалить слот
jtsekret keychain list           # какие слоты заведены
```

Цепочка резолва на каждый запуск: `JTSEKRET_<SLOT>_MASTER_PASSWORD` → macOS Keychain (`Service=jtsekret`, `Account=<slot>`). На non-darwin платформах keychain-шаг пропускается молча.

При первом доступе из новой сборки jtsekret macOS покажет системный prompt — выберите «Always Allow», чтобы не подтверждать каждый раз. Reализовано через `/usr/bin/security` (без CGO/Security.framework).

### Audit-лог

Каждый `get/create/set/delete/dump/exec` пишет одну JSON-строку в локальный append-only лог. Значения секретов **не пишутся** — только action/backend/secret/key/result. По умолчанию путь — `$XDG_STATE_HOME/jtsekret/audit.log` (fallback `~/.local/state/jtsekret/audit.log`); можно переопределить через `JTSEKRET_AUDIT_LOG`.

```
jtsekret audit show                          # последние 20 записей в табличном виде
jtsekret audit show -n 100 --json            # 100 записей в формате JSON Lines
jtsekret audit path                          # путь к log-файлу
jtsekret --no-audit get my-token --key tok   # отключить аудит на одну команду
```

### Глобальные флаги

```
--config <path>            путь к файлу конфига
--output plain|table|json  формат вывода (по умолчанию авто-детект)
--no-cache                 не использовать кэш для этой команды
--no-audit                 не писать в audit-лог для этой команды
--debug                    debug-логирование на stderr
```

## Кэш

Секреты кэшируются локально в зашифрованном файле (AES-256-GCM, ключ выводится через Argon2id из мастер-пароля).

Мастер-пароль задаётся через переменную окружения `JTSEKRET_CACHE_MASTER_PASSWORD`. При отсутствии мастер-пароля кэш не используется.

## Сборка

```bash
make build            # текущая платформа
make build-linux      # linux/amd64
make build-darwin     # darwin/arm64
make test             # unit-тесты с race detector
make test-integration # + integration-тесты (см. ниже)
make lint             # golangci-lint
```

### Integration-тесты

Прячутся за build-tag `integration`, по умолчанию не запускаются. Требуют живых сервисов:

- **vault** — нужен `vault` бинарь на PATH; тест поднимает свой `vault server -dev` на свободном порту.
- **lockbox** — нужен `JTSEKRET_TEST_FOLDER_ID=b1g...` и аутентифицированный `yc` CLI (или один из YC_*_TOKEN env vars).
- **github** — `file://`-bare-remote вариант запускается всегда; HTTPS-вариант опционально через `JTSEKRET_TEST_GITHUB_REPO=owner/repo` и `JTSEKRET_TEST_GITHUB_TOKEN`.

```bash
make test-integration
JTSEKRET_TEST_FOLDER_ID=b1g... make test-integration
```

## Лицензия

MIT © Mikhail Savin

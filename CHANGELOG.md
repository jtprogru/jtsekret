# Changelog

Все заметные изменения jtsekret. Формат — обратный хронологический,
от свежих версий к старым. Версионирование — semver.

С v1.0.0 публичный конфиг-API считается стабильным: добавление новых
бэкендов и опциональных полей — minor; ломающие изменения существующих
полей или поведения — major.

## v1.0.1 — фикс path traversal в `dump` + тесты

- **Security: path traversal через ключи записей в `jtsekret dump`.** `dump <name> --dir <dir>` без `--key` писал каждую запись в `filepath.Join(dir, e.Key)`, где `e.Key` приходил из payload бэкенда (Lockbox/Vault — со стороны облака) без валидации. Ключ вида `../../../.ssh/authorized_keys` мог записать контролируемое значение вне `--dir` (перезапись SSH-ключей / shell-rc / конфига). Добавлен `validateEntryKey` (отвергает пустые ключи, разделители пути и `..`); применяется в `dumpEntry` и на этапе записи в `set`/`create`. Ветка `--output <path>` (доверенный CLI-флаг) не затронута.
- Тесты: `validateEntryKey`, отклонение traversal-ключа в `dumpEntry`/`runSet`/`runCreate` (через mock-бэкенд), ветки `--output`/`--output -`; добавлен отсутствовавший `TestValidateName` для file-бэкенда.
- Security (deps): обновлены `golang.org/x/crypto` v0.49.0 → v0.52.0 и `golang.org/x/net` v0.51.0 → v0.55.0 — закрывают 9 CVE (GO-2026-5013…5026, GO-2026-4918) в `x/crypto/ssh*` и `x/net/idna`, которые ловил `govulncheck` в CI. `vendor/` пересинхронизирован.
- Lint: ложное срабатывание gosec G117 на `audit.Entry.Secret` (поле хранит имя секрета, не значение) исключено на уровне `.golangci.yaml` (`gosec.excludes`), а не через `//nolint`. Версия golangci-lint в CI выровнена с локальной (v2.5.0 → v2.12.2): на v2.5.0 правила G117 не было, из-за чего ломались и `//nolint` (unused), и сам `gosec.excludes` (schema verify). `make` по умолчанию вызывает `help`.

## v1.0.0 — стабилизация публичного конфига и фаза релиза

- **Жёсткий `Validate()` для каждого бэкенда** (`internal/config/validate.go`).
  Теперь `jtsekret config validate` ловит:
  - неизвестный `backend.type` (с подсказкой supported values)
  - `github` без `repo` или без master-пароля
  - `github` с неподдерживаемым `auth.type`
  - `file` без master-пароля
  - `vault` без `address` или с неподдерживаемым `auth.type`
  - дефолт `lockbox.auth.type` теперь `auto` (было `oauth` — опасно, ломалось при `yc iam create-token`)
- Публичные сентинельные ошибки `ErrMissingGithubRepo`, `ErrMissingVaultAddress` и т.д. — для матчинга в скриптах.
- `SupportedBackendTypes` экспортирован — append-only контракт.
- Дополнительные unit-тесты: 12 кейсов на per-backend Validate, 6 пакетов с 0% coverage подняты до 53–95%.
- Integration-тесты за build-tag `integration` для каждого бэкенда: vault через `vault server -dev` на свободном порту, lockbox против реального YC folder, github через `file://` bare-remote.

## v0.7.0 — macOS Keychain (последний пункт Phase F)

- `jtsekret keychain {set,get,unset,list}` — хранение мастер-паролей в macOS Keychain через системный `/usr/bin/security` (без CGO).
- Цепочка резолва на каждый запуск: `JTSEKRET_<SLOT>_MASTER_PASSWORD` env → macOS Keychain. Non-darwin платформы пропускают шаг 2.
- Слоты: `cache`, `github`, `file`. Один пароль раз — и можно убрать env-vars из shell-rc.

## v0.6.0 — HashiCorp Vault backend (Phase C)

- KV v2 поверх `github.com/hashicorp/vault/api`. Layout: `<mount>/data/<prefix>/<name>`. Версионирование нативное.
- Auth: `token` (env `VAULT_TOKEN`), `approle` (`role_id`+`secret_id`), `userpass` (`username`+`password`).
- TLS: `ca_cert` (PEM bundle), `insecure` (skip-verify, debug only).
- UTF-8 only для значений; бинарные payload — через github/file backend.
- Тесты: 5 unit-кейсов через httptest fake KV v2 + ручной smoke против `vault server -dev`.

## v0.5.0 — `rotate-master` + append-only audit log (Phase F)

- `jtsekret rotate-master` — расшифровать всё текущим паролем и пере-зашифровать новым (свежий per-secret salt). Реализован для file и github через новый optional `MasterPasswordRotator` интерфейс. Lockbox/Vault явно отклоняют — у них нет локального мастера.
- `internal/audit` — JSON-Lines append-only лог в `$XDG_STATE_HOME/jtsekret/audit.log`. Фиксирует action/backend/secret/key/result, **никогда не значения**. Hooks в `get/set/create/delete/dump/exec`. `--no-audit` для отдельной команды.
- `jtsekret audit show [-n N] [--json]`, `jtsekret audit path`.

## v0.4.0 — UX-полировка (Phase F start)

- `jtsekret search <pattern>` — substring-поиск по именам, `--include-keys` для матча и по entry-ключам.
- Tab-completion имён секретов на `get/set/delete/dump/exec --secret`. Кеш на 5 мин в `~/.cache/jtsekret/completion.json`, авто-инвалидация в create/set/delete.
- `--copy` + auto-clear: значение в буфер обмена, очистка через 30s (`--copy-clear-after N`). UX как у `pass -c`. Платформенные тулы (`pbcopy`/`wl-copy`/`xclip`/`xsel`).

## v0.3.0 — `file` backend + security baseline (Phase D + G)

- **`file` backend**: фульно-офлайн, та же AES-256-GCM + Argon2id схема что в github, без git-слоя. Layout `<root>/secrets/<name>.{enc,meta.json}`. Master-password из `JTSEKRET_FILE_MASTER_PASSWORD`.
- **CVE-патчи** транзитивных зависимостей: grpc 1.66.2 → 1.81.0 (authorization bypass), jwt/v4 4.5.1 → 4.5.2 (excessive memory allocation).
- **CI workflow**: vet → build → race-tests → golangci-lint v2 → govulncheck. PR-блокеры.
- **SBOM** (syft, JSON) на каждый архив релиза.
- 133 lint-issues → 0. `.golangci.yaml` тонко настроен под cobra-CLI паттерн.

## v0.2.2 — fix: lockbox payload + name resolution

- `CreateSecret`/`AddVersion` теперь действительно пробрасывают entries в `VersionPayloadEntries`/`PayloadEntries` (было: запросы уходили без payload).
- `resolveID(name)` для get/delete/AddVersion (Lockbox API принимает только ID, а CLI передавал имена).
- UTF-8 → `TextValue`, бинарь → `BinaryValue`.

## v0.2.1 — clean stderr by default

- `slog`-handler читает `slog.LevelVar` вместо хардкода `LevelInfo`. Дефолт — `Warn`. `--debug` → Debug.
- "starting jtsekret" → debug-level. "Using config file" → debug-level. `eval "$(jtsekret completion zsh)"` в `.zshrc` теперь не сорит на стартап.

## v0.2.0 — `sync` + cross-backend `migrate` (Phase B + E)

- **`jtsekret sync`** — explicit pull+push через optional `Syncer` интерфейс. Работает для github, no-op для lockbox.
- **`jtsekret migrate --target-config <path>`** — копирование между бэкендами через два конфиг-файла. `--update`, `--dry-run`, `--only n1,n2`. Тесты против двух mock-бэкендов.
- **GitHub repo backend** (Phase B): AES-256-GCM, per-secret salt, Argon2id из master-password. Layout `secrets/<name>.{enc,meta.json}`. URL-форматы `owner/repo`/HTTPS/SSH/`file://`. Auth `token`/`ssh`/`none`. auto-pull/auto-push.
- Refactor `cmd/backendcfg.go`: централизация построения backend-конфига (8 cmd-файлов больше не дублируют map).

## v0.1.1 — homebrew tap publication

- Заменили сломанный `homebrew_casks` блок (генерил Cask, но в Formula/) на `homebrew_casks` с `directory: Casks` и post-install xattr-хуком (убирает quarantine на macOS).
- `HOMEBREW_TAP_GITHUB_TOKEN` пробрасывается в env goreleaser.
- Установка: `brew tap jtprogru/tap && brew install --cask jtsekret`.

## v0.1.0 — первый релиз через goreleaser

- 6 архивов (Darwin/Linux/FreeBSD × amd64/arm64) + GPG-подписанный `checksums.txt`.
- Cobra+Viper CLI, AES-256-GCM + Argon2id encrypted local cache, Yandex Cloud Lockbox backend (4 типа auth), все базовые команды (`get/set/list/create/delete/exec/dump`), output layer (plain/table/json), mock-backend для тестов.

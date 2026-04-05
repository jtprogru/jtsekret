# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build            # Build binary with git-injected version/commit/buildTime
make test             # All tests with race detector + coverage → coverage.out
make test-unit        # Unit tests only
make test-integration # Integration tests (requires Yandex Cloud credentials)
make lint             # golangci-lint
make fmt              # gofmt + goimports
make vet              # go vet
make clean            # Remove dist/

# Run a single test
go test -v ./internal/crypto/... -run TestEncrypt

# Integration tests use build tag
go test -v -tags=integration ./...
```

## Architecture

The app is a CLI (`cmd/` via Cobra + Viper) that reads/writes secrets through a **pluggable backend** layer with an optional **encrypted local cache** in front of it.

```
cmd/ → config.Load() → Cache → Backend → Yandex Cloud Lockbox (gRPC)
```

### Backend Registry

`internal/backend/backend.go` defines the `Backend` interface and a global registry (`Register` / `New`). Implementations must call `Register` in their `init()`.

**Critical**: every backend implementation must be blank-imported in `main.go` to trigger its `init()`:
```go
import _ "github.com/jtprogru/jtsekret/internal/backend/lockbox"
```

Without this, `New("lockbox", ...)` returns `unknown backend: lockbox`.

Current implementations: `lockbox/` (Yandex Cloud, gRPC) and `mock/` (in-memory, for tests).

### Lockbox Auth

`internal/backend/lockbox/client.go` supports four auth types (configured via `backend.lockbox.auth.type`):

| type | token source | env var |
|------|-------------|---------|
| `oauth` | permanent OAuth token | `YC_OAUTH_TOKEN` |
| `iam_token` | short-lived IAM token | `YC_IAM_TOKEN` |
| `service_account_key` | SA key JSON file | `YC_SERVICE_ACCOUNT_KEY_FILE` |
| `instance_service_account` | VM metadata | *(none)* |

The SDK is built with `ycsdk.Config{Credentials: creds}` — **no custom Endpoint**. Setting a global endpoint breaks service discovery (causes gRPC `Unimplemented` errors).

### Config → Backend wiring

All `cmd/*.go` files build a `map[string]interface{}` config that is passed to `backend.New()`. The auth sub-map must also be `map[string]interface{}`, not `map[string]string` — a type mismatch silently fails the type assertion in `backend/lockbox/backend.go`, causing auth config to be ignored.

```go
lockboxCfg := map[string]interface{}{
    "folder_id": cfg.Backend.Lockbox.FolderID,
    "auth": map[string]interface{}{
        "type":                 cfg.Backend.Lockbox.Auth.Type,
        "token":                cfg.Backend.Lockbox.Auth.Token,
        "service_account_file": cfg.Backend.Lockbox.Auth.ServiceAccountFile,
    },
}
```

### Cache

`internal/cache/` — two implementations selected by config:
- `EncryptedFile`: AES-256-GCM with Argon2id KDF, format `[16B salt][12B nonce][ciphertext]`
- `Noop`: pass-through when cache is disabled

### Output

`internal/output/output.go` — `Outputter` interface with `Plain`, `Table`, `JSON` implementations. `DetectFormat()` auto-selects based on `term.IsTerminal(os.Stdout.Fd())`.

## Code Style

- Imports: stdlib → external → internal (use `goimports`)
- Error wrapping: `fmt.Errorf("operation %q: %w", name, err)`
- Integration tests: `//go:build integration` build tag
- Test naming: `Test<FunctionName>_<Scenario>`
- All new files need the MIT license header (see any existing file)

## Config

Default config path: `~/.config/jtsekret/jtsekret.yaml`. See `configs/jtsekret.example.yaml` for all options. Env override prefix: `JTSEKRET_`.

Config struct lives in `internal/config/config.go`. `LockboxAuth.ServiceAccountFile` maps to `backend.lockbox.auth.service_account_file` in YAML.

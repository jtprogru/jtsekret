# AGENTS.md — Guide for Agentic Coding in jtsekret

This file provides guidelines and commands for agents operating in this repository.

---

## Build, Test, and Lint Commands

All build, test, and development operations should be performed using the Makefile:

```bash
# Show available targets
make help

# Build binary (with version info from git)
make build

# Run tests with race detector
make test

# Run unit tests only
make test-unit

# Run linter
make lint

# Format code
make fmt

# Run go vet
make vet

# Clean build artifacts
make clean

# Install binary to $GOPATH/bin
make install
```

### Manual commands (if Makefile not available)

If Makefile is not available, use these commands directly:

```bash
# Build with version info (for release)
go build -ldflags="-X main.version=1.0.0 -X main.commit=$(git rev-parse HEAD)" -o jtsekret .

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o jtsekret-linux-amd64 .
GOOS=darwin GOARCH=arm64 go build -o jtsekret-darwin-arm64 .
GOOS=windows GOARCH=amd64 go build -o jtsekret.exe .
```

### Run
```bash
# Run locally
go run .

# Run with config
go run . --config /path/to/config.yaml
```

### Test
```bash
# Run all tests
go test -v ./...

# Run tests in specific package
go test -v ./internal/crypto/...

# Run single test by name
go test -v ./internal/crypto/... -run TestEncrypt

# Run tests with verbose output
go test -v -race ./...

# Run integration tests (requires credentials)
go test -v -tags=integration ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go test -covermode=atomic ./...
```

### Lint and Format
```bash
# Format code
gofmt -w .
goimports -w .

# Run linter (if .golangci.yaml exists)
golangci-lint run

# Run go vet
go vet ./...

# Run all checks
go vet ./... && golangci-lint run && go test ./...
```

### Dependencies
```bash
# Download dependencies
go mod download

# Tidy dependencies
go mod tidy

# Verify dependencies
go mod verify
```

---

## Code Style Guidelines

### General Principles

- **Go version**: 1.25.0 (minimum)
- **License**: MIT — include header in all new files
- **Line length**: Prefer 80-100 characters, but no hard limit
- **Comments**: Write comments for exported functions/types; explain "why", not "what"

### Imports

Group imports in the following order (blank line between groups):

```go
import (
    // Standard library
    "context"
    "encoding/json"
    "fmt"
    "os"

    // External packages
    "github.com/spf13/cobra"
    "github.com/spf13/viper"

    // Internal packages
    "github.com/jtprogru/jtsekret/internal/config"
    "github.com/jtprogru/jtsekret/internal/domain"
)
```

Use `goimports` to automatically organize imports.

### Naming Conventions

| Element | Convention | Example |
|---------|------------|---------|
| Variables | camelCase | `configFile`, `secretName` |
| Functions | camelCase | `getSecret()`, `parseConfig()` |
| Exported Types | PascalCase | `Secret`, `Backend`, `Config` |
| Exported Functions | PascalCase | `Execute()`, `NewBackend()` |
| Constants | PascalCase | `DefaultCacheTTL`, `MaxRetries` |
| Package names | lowercase | `backend`, `crypto`, `domain` |
| File names | snake_case | `backend.go`, `cache_test.go` |

### Types and Declarations

- Prefer concrete types over interfaces unless polymorphism is needed
- Use pointer receivers (`*Type`) for methods that modify the receiver or for nilability
- Use value receivers for methods that don't modify the receiver
- Declare variables as close to their first use as possible
- Prefer short variable declarations (`:=`) when type is obvious

```go
// Good
cfg, err := config.Load()
if err != nil {
    return fmt.Errorf("load config: %w", err)
}

// Avoid
var cfg config.Config
var err error
```

### Error Handling

- **Never ignore errors** with `_` unless explicitly documented
- **Wrap errors** with context using `fmt.Errorf("...: %w", err)`
- **Check errors early** and return early
- **Use sentinel errors** for public API errors (e.g., `var ErrNotFound = errors.New("not found")`)
- **Avoid panic** except for unrecoverable conditions

```go
// Good
func GetSecret(name string) (*domain.Secret, error) {
    secret, err := backend.GetSecret(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("get secret %q: %w", name, err)
    }
    return secret, nil
}

// Bad
secret, _ := backend.GetSecret(ctx, name) // Never do this
```

### Context and Concurrency

- Pass `context.Context` as the first argument to functions that perform I/O
- Use `context.WithTimeout` or `context.WithCancel` for operation deadlines
- Use `sync.WaitGroup` for coordinating goroutines
- Use `sync.Mutex` or `sync.RWMutex` for shared state protection
- Prefer `errgroup` for parallel operations

### Testing

- Test files should be named `*_test.go`
- Use table-driven tests when testing multiple cases
- Use `//go:build integration` tag for integration tests requiring external services
- Mock external dependencies (backend, cache) via interfaces
- Test name format: `Test<FunctionName>_<Scenario>`

```go
func TestEncrypt(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid", "secret data", false},
        {"empty", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := Encrypt([]byte(tt.input), key)
            if (err != nil) != tt.wantErr {
                t.Errorf("Encrypt() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Project Structure

```
jtsekret/
├── cmd/                 # CLI commands (Cobra)
│   ├── root.go
│   ├── get.go
│   └── ...
├── internal/
│   ├── config/          # Configuration loading and validation
│   ├── backend/         # Backend interface and implementations
│   │   ├── backend.go   # Interface + Registry
│   │   ├── lockbox/     # Yandex Cloud Lockbox backend
│   │   └── mock/        # Mock backend for tests
│   ├── cache/           # Cache implementations
│   ├── crypto/          # Cryptographic utilities
│   ├── output/          # Output formatters
│   └── domain/          # Domain models
├── configs/             # Example configuration files
├── docs/                # Documentation
├── main.go              # Entry point
└── go.mod
```

---

## Git Conventions

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

Types: feat, fix, docs, style, refactor, test, chore

Examples:
feat(cache): add encrypted file cache implementation
fix(lockbox): handle missing folder_id gracefully
docs(readme): update installation instructions
```

### Branch Naming

```
feature/<description>
bugfix/<description>
hotfix/<description>
```

### Pull Requests

- PR title follows conventional commits
- Description includes: summary, changes, testing performed
- Run `go vet`, `gofmt`, and tests before submitting

---

## Working with Secrets

Since this project deals with secrets:

- **Never log secret values** — even in debug mode
- **Clear sensitive data** from memory when not needed
- **Use `runtime.SetFinalizer`** for cleanup of sensitive buffers
- **Prefer `crypto/rand`** over `math/rand` for security-sensitive operations

---

## Additional Resources

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout)

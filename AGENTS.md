# AGENTS.md — Guide for Agentic Coding in jtsekret

CLI utility for secrets management with Yandex Cloud Lockbox backend.

---

## Build, Test, Lint Commands

Use Makefile for all operations:

```bash
make help        # Show available targets
make build       # Build binary (auto-versions from git)
make test        # Run tests with race detector + coverage
make test-unit   # Run unit tests only (no integration)
make test-integration  # Run integration tests (requires credentials)
make lint        # Run golangci-lint
make fmt         # Format code (gofmt + goimports)
make vet         # Run go vet
make clean       # Remove build artifacts
make install     # Install to $GOPATH/bin
make release     # Create release with goreleaser
make release-dry # Dry-run release
```

### Manual commands

```bash
# Build with version info
go build -ldflags="-X main.version=1.0.0 -X main.commit=$(git rev-parse HEAD)" -o jtsekret .

# Single test by name
go test -v ./internal/crypto/... -run TestEncrypt

# Single package test
go test -v ./internal/backend/mock/...

# Test with race detector
go test -v -race ./...

# Integration tests
go test -v -tags=integration ./...

# Format
gofmt -w .
goimports -w .
```

---

## Code Style

### General
- **Go version**: 1.25.0
- **License**: MIT — include header in all new files
- **Line length**: Prefer 80-100 chars
- **Comments**: Document exported functions; explain "why", not "what"

### Imports (order: stdlib → external → internal)

```go
import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"

    "github.com/jtprogru/jtsekret/internal/config"
    "github.com/jtprogru/jtsekret/internal/domain"
)
```

Use `goimports` to auto-organize.

### Naming

| Element | Convention | Example |
|---------|------------|---------|
| Variables | camelCase | `configFile` |
| Exported Types | PascalCase | `Secret`, `Backend` |
| Constants | PascalCase | `DefaultCacheTTL` |
| Packages | lowercase | `backend`, `crypto` |
| Files | snake_case | `backend.go`, `cache_test.go` |

### Error Handling

- Never ignore errors with `_`
- Wrap with context: `fmt.Errorf("get secret %q: %w", name, err)`
- Use sentinel errors for public APIs
- Avoid panic except for unrecoverable conditions

### Testing

- Test files: `*_test.go`
- Use table-driven tests
- Tag integration tests: `//go:build integration`
- Format: `Test<FunctionName>_<Scenario>`

---

## Project Structure

```
jtsekret/
├── cmd/              # CLI commands (Cobra)
├── internal/
│   ├── backend/      # Backend interface + implementations
│   │   ├── backend.go
│   │   ├── lockbox/  # Yandex Cloud Lockbox
│   │   └── mock/     # Mock for tests
│   ├── cache/        # Cache implementations
│   ├── config/       # Config loading
│   ├── crypto/       # Cryptographic utilities
│   ├── domain/       # Domain models
│   └── output/       # Output formatters
├── main.go
└── go.mod
```

---

## Git Conventions

### Commit Messages
```
<type>(<scope>): <description>

Types: feat, fix, docs, style, refactor, test, chore

Examples:
feat(lockbox): add secret versioning support
fix(cache): handle missing encryption key gracefully
docs(readme): update installation instructions
```

### Branch Naming
```
feature/<description>
bugfix/<description>
hotfix/<description>
```

---

## Security

- Never log secret values
- Clear sensitive data from memory when done
- Prefer `crypto/rand` over `math/rand` for security operations

---

## References

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
- [Conventional Commits](https://www.conventionalcommits.org/)

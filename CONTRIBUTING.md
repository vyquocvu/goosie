# Contributing to Goosie

## Quick Start

```bash
git clone https://github.com/vyquocvu/goosie
cd goosie
make build
./bin/goosie -backend headless
```

## Development Workflow

1. Check the issue tracker and confirm the scope before starting work.
2. Write tests first. Include normal and edge cases.
3. Run the full check suite before committing:

```bash
gofmt -w .
go vet ./...
go test -count=1 ./...
go test -race ./...
```

4. Use a concise conventional commit message, such as `fix(raster): correct tile damage tracking`.

## Code Conventions

- **Imports:** stdlib, then external, then internal — grouped by blank lines.
- **Errors:** Return errors rather than panicking. Use `fmt.Errorf("context: %w", err)` for wrap.
- **Context:** Propagate `context.Context` for all cancellable work.
- **Ownership:** One owner goroutine per mutable state. Use `sync.RWMutex` for concurrent read access.
- **Import graph:** Follows the layer rule: `frame` <- `paint` <- `surface` <- `raster` <- {`platform/*`, `cmd/*`}. Enforced by `go test ./internal/archtest/`.
- **cgo:** Only `internal/platform/darwin` may use cgo.

## Testing

| Test type | Command | When to run |
|---|---|---|
| Unit tests | `go test ./...` | Every commit |
| Race detector | `go test -race ./...` | PRs touching concurrent code |
| Gate suite | `go test ./test/gate -run TestGate -v` | Frame path changes |
| Benchmarks | `go run ./cmd/goosie -bench -frames 2000` | Performance changes |

## Pull Request Process

1. One commit per logical change. Squash before merging.
2. Commit messages must describe the affected area and outcome.
3. All CI gates must pass (build, test, race, arch).

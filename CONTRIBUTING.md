# Contributing to Goosie

## Quick Start

```bash
git clone https://github.com/vyquocvu/goosie
cd goosie
make build
./bin/goosie -version
```

## Development Workflow

1. Read `ROADMAP_V2.md` to understand the current milestone and pick an unblocked task.
2. Read `PACKAGE_OWNERSHIP.md` to find the owning package for your change.
3. Read `.prompt` for the engineering workflow (if using an AI coding agent).
4. Write tests first (TDD). Include normal, edge, and cancellation cases.
5. Run the full check suite before committing:

```bash
gofmt -w .
go vet ./...
go test -short ./...
go test -race -short ./internal/engine/... ./internal/js/... ./internal/net/...
make smoke-test
```

6. Compare benchmark results against `main` for performance-sensitive changes:

```bash
go test -bench=. -benchmem ./path/to/package
```

7. Commit with a message prefixed by the roadmap milestone, e.g. `roadmap-v2/m4-fix-table-layout`.

## Code Conventions

- **Imports:** stdlib, then external, then internal — grouped by blank lines.
- **Errors:** Return errors rather than panicking. Use `fmt.Errorf("context: %w", err)` for wrap.
- **Context:** Propagate `context.Context` for all cancellable work.
- **Ownership:** One owner goroutine per mutable state. Use `sync.RWMutex` for concurrent read access.
- **Allocations:** Avoid heap-allocated child slices. Prefer compact index-based storage (see ADR 0001).
- **No Fyne in engine:** Core packages (`internal/dom`, `internal/css`, `internal/renderer/frame/*`, `internal/engine/*`, `internal/js`, `internal/net`) must never import `fyne.io/fyne/v2`.

## Testing

| Test type | Command | When to run |
|---|---|---|
| Unit tests | `go test ./internal/...` | Every commit |
| Race detector | `go test -race -short ./internal/engine/...` | PRs touching concurrent code |
| Golden images | `go test ./internal/renderer/frame/golden/...` | PRs touching raster/display list |
| Fuzz tests | `go test -fuzz=FuzzHTMLParse -fuzztime=30s ./internal/dom/` | Parser/selector changes |
| E2E | `go test -tags=e2e ./test/e2e/` | Full pipeline changes |
| Memory growth | `go test -v ./internal/engine/session/ -run TestMemory` | Changes to caches or session lifecycle |

## Documentation

- Update `ROADMAP_V2.md` (mark checkboxes) when completing a roadmap task.
- Add ADR `website/docs/adr/NNNN-title.md` for significant architecture decisions (see `website/docs/adr/` for examples).
- Update `website/docs/supported-web-platform.md` when adding or changing supported features.
- Add package doc comments (`// Package X provides ...`) for new packages.

## Pull Request Process

1. One commit per logical change. Squash before merging.
2. Commit messages must reference the roadmap milestone.
3. PR description must include benchmark comparison for performance changes.
4. All CI gates must pass (build, test, race, golden).
5. Architecture documentation must be updated when ownership or data flow changes.

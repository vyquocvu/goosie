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
- **Formatting:** gofmt. `gofmt -w .` above rewrites files and always exits 0, so it is the fix, not the check; the check is the same `go test ./internal/archtest/`, which fails on any file gofmt would rewrite.
- **cgo:** Only `internal/platform/darwin` may use cgo.
- **Global state:** No package under `internal/` may assign a package-level var after `init()`. Pass the value to the code that needs it — per-document state rides on the cascade or the session, not on a package var. Enforced by the same `go test ./internal/archtest/`, which walks assignments (including writes through a map, slice or struct field) and ignores test files. What the walk cannot see is a write through an address the var handed away (`return &counter`) or into an exported var from another package, so those two shapes are review's job.

## Testing

| Test type | Command | When to run |
|---|---|---|
| Unit tests | `go test ./...` | Every commit |
| Race detector | `go test -race ./...` | PRs touching concurrent code |
| Gate suite | `go test ./test/gate -run TestGate -v` | Frame path changes |
| Benchmarks | `go run ./cmd/goosie -bench -frames 2000` | Performance changes |
| Fuzz targets | `go test -fuzz='^FuzzParseDocument$' -fuzztime=120s ./test/dom` | HTML/CSS parser or cascade changes |

Three fuzz targets cover malformed input at the entry points that take untrusted
bytes: `FuzzParseDocument` (`test/dom`) requires `dom.ParseBounded` to hold every limit it
quotes — an input it refuses must be one a looser parse builds, and that tree must break
the refused bound (an attribute refusal may instead be explained by a token the tree
builder dropped) — `FuzzParseStylesheet` (`test/css`) requires that parsing a sheet is
unchanged by an unrelated parse in between, and `FuzzResolveCascade` (`test/style`)
requires the same of the selector matcher. Their seeds run inside `go test ./...`; only
`-fuzz` searches, which the nightly workflow does for 120 s per target. A finding is a
file the fuzzer prints the path to — committing it under `<pkg>/testdata/fuzz/<Target>/`
turns it into a permanent seed.

For fixture documents in tests, use `domtest.Parse` (`test/domtest`) rather than reaching
for an unbounded parse: `internal/dom` exports `ParseBounded` alone, and a test that wants
a document past a cap is asking about the cap, so it passes its own limits.

## Pull Request Process

1. One commit per logical change. Squash before merging.
2. Commit messages must describe the affected area and outcome.
3. All CI gates must pass (build, test, race, arch).

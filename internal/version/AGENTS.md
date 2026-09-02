# internal/version — Agent Constraints & Architecture

## Core Responsibilities

The `internal/version` package exposes build metadata for Goosie binaries. It is a single-file (`version.go`), standard-library-only package with no dependencies on other internal packages.

## Components

- Build-time variables: `Version` (`"dev"`), `Commit` (`"none"`), `BuildTime` (`"unknown"`). These are defaults meant to be overridden via `-ldflags "-X github.com/vyquocvu/goosie/internal/version.Version=..."` at link time.
- `String()`: formats `Version (commit Commit, built BuildTime, <Go runtime version>)`.
- `ReadBuildInfo()`: prefers `runtime/debug.ReadBuildInfo()`; if available, reports the module's Go version and falls back to the `vcs.revision` build setting when `Commit` is still `"none"`. Returns `String()` when build info is unavailable (e.g. `go test` binaries). Currently has no callers in the repo.

## Consumers

- `cmd/browser` and `cmd/headless` print this metadata via their `--version` flag (`version.String()`).

## Testing & Verification

The package has no dedicated test directory; its correctness is exercised indirectly by the cmd binaries. After changing it, build the commands that consume it:

```bash
go build ./cmd/...
go vet ./internal/version/
```

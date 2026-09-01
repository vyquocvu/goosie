# internal/engine — Agent Constraints

## No UI Framework Imports

NEVER import `fyne.io/fyne/v2` or any GUI framework (hard rule in CONTRIBUTING.md).
All types crossing this boundary must be pure value types — no pointers to UI objects.

## Portability Boundary

Must remain usable headless, server-side, and with alternative UIs. Communicate
exclusively through JSON-serializable value types and `io.Reader`/`io.Writer`.

## Allowed External Dependencies

Only `internal/dom`, `internal/net`, and `golang.org/x/net/publicsuffix`. Do not
import `internal/renderer`, `internal/css` (production), or any UI-adjacent package.
Dependency direction is one-way: engine imports dom/net, never the reverse.

## IPC Contract

The `message` package defines the versioned wire protocol. Only value-type fields
cross the parent/child boundary. Pointer fields or non-JSON-safe types break
process isolation.

## Testing

Race detector mandatory: `go test -race -short ./internal/engine/...`

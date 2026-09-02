# internal/engine — Agent Constraints & Architecture

## Core Responsibilities

The `internal/engine` package orchestrates the browser engine lifecycle, coordinating document loading, event loops, session states, navigation pipelines, metrics, and IPC messaging while remaining strictly decoupled from any graphical user interface frameworks.

## No UI Framework Imports

- **NEVER** import `fyne.io/fyne/v2` or any GUI framework (hard rule in CONTRIBUTING.md).
- All types crossing the engine boundary must be pure, JSON-serializable value types or standard `io.Reader`/`io.Writer` streams.
- Pointers to UI objects, window handles, or widget instances are strictly forbidden in engine packages.

## Portability Boundary

- The engine must remain fully usable headless, server-side, and with alternative UI frontends (CLI, MCP server, webview, native GUI).
- Engine state transitions communicate exclusively through value-type event notifications and channels.

## Subpackage Directory Structure

- `documentloader`: Asynchronous document fetching, response stream dispatch, and MIME-type routing.
- `eventloop`: Microtask queues, macrotask scheduling, timer coordination, and animation frame callbacks.
- `message`: IPC protocol definition; versioned wire protocol using JSON-serializable value types crossing process boundaries.
- `metrics`: Navigation timing, parse metrics, layout duration telemetry, and performance tracking.
- `navigation`: Navigation state machine (`Scheduler`), URL resolution, redirect validation, and history integration.
- `renderer`: Abstract engine renderer integration interface.
- `session`: Session state machine (`StateCreated`, `StateNavigating`, `StateParsing`, `StateInteractive`, `StateComplete`, `StateCancelled`, `StateFailed`, `StateClosed`).
- `testpages`: Local fixture servers and embedded test page utilities for engine verification.

## Allowed External Dependencies

- Only `internal/dom`, `internal/net`, and `golang.org/x/net/publicsuffix`.
- Do NOT import `internal/renderer`, `internal/css` (production code), or UI packages.
- Dependency direction is one-way: engine imports `dom` and `net`, never the reverse.

## IPC Contract & Process Isolation

- The `message` package defines the versioned wire protocol.
- Only value-type fields cross the parent/child boundary.
- Pointer fields or non-JSON-safe types break process isolation and must not be added to IPC messages.

## Testing & Verification

All unit and integration tests for engine subpackages reside in `test/internal/engine/...`.

Run the full engine test suite with race detector enabled:
```bash
go test -race -short ./test/internal/engine/...
```

Run targeted subpackage tests:
```bash
go test ./test/internal/engine/session/...
go test ./test/internal/engine/navigation/...
go test ./test/internal/engine/eventloop/...
go test ./test/internal/engine/documentloader/...
go test ./test/internal/engine/message/...
go test ./test/internal/engine/metrics/...
go test ./test/internal/engine/renderer/...
go test ./test/internal/engine/testpages/...
```

# Goosie MCP Integration Roadmap

**Status:** Phase 4 complete — Phase 5+ remaining

**Phase 0 accepted:** 2026-07-28
**Research cut-off:** 2026-07-15

**Protocol baseline:** MCP `2025-11-25`
**Primary goal:** Expose Goosie as a safe, deterministic, headless MCP browser server without coupling MCP to Fyne.

This roadmap is the execution index for MCP work. The supporting documents are normative for implementation:

- [Architecture and boundaries](website/docs/mcp-architecture.md)
- [Tool and resource contracts](website/docs/mcp-protocol-contracts.md)
- [Threat model and security requirements](website/docs/mcp-security.md)
- [TDD and verification plan](website/docs/mcp-tdd-plan.md)
- [Architecture decision record](website/docs/adr/0004-mcp-browser-control-boundary.md)

No MCP code should be written until the Phase 0 documentation exit gate is accepted.

## 1. Executive decision

Build an **MCP server**, not an MCP client, as a new `cmd/mcp-server` entry point. The server will adapt a new UI-independent browser-control package to the official MCP Go SDK. It must not call `internal/ui` or expose Fyne objects.

The implementation order is deliberately two-layered:

1. Extract a deterministic `internal/browsercontrol` service from the currently shell-owned load path.
2. Add a thin `internal/mcpserver` protocol adapter over that service.

This is required because the current full navigation pipeline is coordinated by `loadPageAsync` in `cmd/browser/main.go`, while the reusable lifecycle, network, DOM, JavaScript, and renderer primitives live below it. Binding MCP directly to the shell would make headless automation depend on UI-thread timing and would duplicate behavior when another protocol is added.

## 2. Evidence from the repository

| Existing capability | Current seam | MCP consequence |
|---|---|---|
| Navigation lifecycle, cancellation, events | `internal/engine/session.Session` | Reuse for one active navigation per browser context. |
| Streaming fetch and metadata | `internal/net.Fetcher` / `Service` | Reuse; add browser-control ownership and policy checks. |
| Full load orchestration | `cmd/browser/main.go:loadPageAsync` | Extract before MCP; command packages must remain composition roots. |
| DOM and raw document | `internal/dom`, tab renderer state | Define a stable semantic snapshot independent of Fyne/render nodes. |
| JavaScript runtime and bounded owner queue | `internal/js.Session` | All evaluation must run through the owner queue with cancellation and limits. |
| Headless raster output | `internal/renderer.RenderHTMLToImage` | Reuse the raster path, but add a loaded-page screenshot API. |
| Input and viewport wire types | `internal/engine/message` | Reuse concepts; do not expose the internal IPC schema as the MCP contract. |
| Renderer child protocol | `internal/engine/renderer` | Potential future isolation boundary; not the MCP transport. |
| Console/network/security data | JS runtime, network service, session events | Expose as bounded snapshots/resources after ownership is clarified. |

Known gaps are first-class roadmap work: stable element references, selector/query actions, loaded-page screenshots, deterministic readiness, multi-context ownership, unified policy, and automation-safe downloads do not yet form a single browser API.

## 3. Standards and dependency baseline

MCP uses JSON-RPC 2.0, initialization/capability negotiation, and standard stdio and Streamable HTTP transports. The specification recommends stdio support where possible and imposes Origin validation, loopback binding, and authentication requirements on Streamable HTTP. See the official [lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle), [transports](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports), [tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools), [resources](https://modelcontextprotocol.io/specification/2025-11-25/server/resources), and [authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization) specifications.

Use the official [`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk). There is a temporary toolchain constraint:

| Choice | Go requirement | Protocol support | Roadmap use |
|---|---:|---|---|
| SDK v1.4.0 | Go 1.24 | 2025-11-25 | Permitted for the stdio-only spike and first local release. |
| SDK v1.4.1+ (including v1.6.1) | Go 1.25 | 2025-11-25 | Preferred after Goosie upgrades to Go 1.25. |

Goosie currently declares Go 1.24.9. SDK v1.4.0 is the last reviewed tag compatible with it; [v1.4.0 declares Go 1.24](https://github.com/modelcontextprotocol/go-sdk/blob/v1.4.0/go.mod), while [v1.4.1](https://github.com/modelcontextprotocol/go-sdk/blob/v1.4.1/go.mod) and [v1.6.1](https://github.com/modelcontextprotocol/go-sdk/blob/v1.6.1/go.mod) declare Go 1.25.

**Decision:** do not block the stdio proof on a global toolchain upgrade, but do not ship Streamable HTTP on the older SDK. Before each implementation phase, re-check the latest released (not unreleased/main-branch) MCP revision and SDK tag. Do not silently change the pinned protocol version.

## 4. Product scope

### In scope for v1

- Local stdio MCP server.
- Explicit browser-context creation and closure.
- Navigate and wait for deterministic lifecycle states.
- Semantic DOM/accessibility-style snapshot with stable references.
- Query, click, type, key press, scroll, viewport, and JavaScript evaluation as separate tools.
- PNG screenshots returned as MCP image content, with size limits.
- Bounded console, network, page metadata, and security snapshots.
- Cancellation, deadlines, structured errors, audit logging to stderr, and graceful shutdown.
- Hermetic contract/conformance tests and local-fixture end-to-end tests.

### Deferred

- Streamable HTTP and OAuth until the Go 1.25/security gate.
- Remote hosting, multi-tenant operation, public interfaces, and cloud deployment.
- Automatic downloads or arbitrary host file writes.
- MCP client features such as sampling, roots, or elicitation.
- Prompts; browser actions are tools and browser state is resources.
- Compatibility with deprecated HTTP+SSE transport.
- Pixel-coordinate actions as the primary API; semantic references come first.
- Parallel mutation of one browser context.

## 5. Milestones and TDD gates

Every implementation slice follows red → green → refactor. A phase cannot start until its predecessor's tests and documents pass.

### Phase 0 — Documentation and decision lock ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ Accepted this roadmap and the four supporting documents.
- ✅ Resolved open decisions listed in Section 8.
- ✅ Added decision log entry for chosen SDK/toolchain versions.
- ✅ Defined the supported-platform matrix and CI jobs.

**Exit gate**

- ✅ Tool contracts have examples, limits, error codes, and risk classifications.
- ✅ Security requirements have owners and executable-test mappings.
- ✅ No supporting document contradicts package ownership rules.

### Phase 1 — Browser-control contract, fake first ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ `internal/browsercontrol` package with `Service` and `Context` interfaces
- ✅ Typed error codes (11 stable codes) in `errors.go`
- ✅ Type definitions with limits in `types.go`
- ✅ `FakeService` and `FakeContext` for testing
- ✅ `EngineService` and `engineContext` for real browser contexts
- ✅ Comprehensive tests: service lifecycle, context contract, typed errors
- ✅ `go test -race ./internal/browsercontrol/...` clean

**Exit gate**

- ✅ `go test -race ./internal/browsercontrol/...` passes.
- ✅ Lifecycle and cancellation tests are deterministic; no sleeps.

**Green target**

- Introduce `internal/browsercontrol` interfaces and value types only.
- No MCP dependency and no Fyne import.
- One context owns one engine session, network service, DOM, JS session, renderer state, and bounded observability buffers.

**Exit gate**

- `go test -race ./internal/browsercontrol/...` passes.
- Lifecycle and cancellation tests are deterministic; no sleeps.

### Phase 2 — Extract the headless page pipeline ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ `engineContext.Navigate()` orchestrates: session → fetcher → parser → DOM
- ✅ State-transition tests: created → navigating → parsing → interactive → complete
- ✅ Error tests: HTTP error, invalid MIME, cancellation, timeout
- ✅ Navigation tests: superseded, file URL denied, redirect, oversized response
- ✅ Close-during-load tests
- ✅ Semantic snapshot from DOM with role inference
- ✅ Read-only operations: query, click, type, viewport, screenshot

**Exit gate**

- ✅ `go test -race ./internal/browsercontrol/...` passes.
- ✅ `go test ./internal/browsercontrol/... -count=50` passes.
- ✅ Existing GUI/headless tests remain green.

### Phase 3 — Read-only MCP stdio server ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ `internal/mcpserver` package with Server implementation
- ✅ `cmd/mcp-server` entry point
- ✅ 10 MCP tool schemas defined
- ✅ Tool handlers for all read-only operations
- ✅ Protocol tests for initialization, capability advertisement
- ✅ JSON Schema validation tests
- ✅ Integration tests with httptest server
- ✅ Cancellation and error mapping tests
- ✅ Performance benchmarks

**Green target**

- ✅ Add `internal/mcpserver` and `cmd/mcp-server`.
- ✅ Register read-only tools first: create/close/list contexts, navigate, snapshot, screenshot, page metadata, console/network/security reads.
- ✅ Use stdout only for MCP frames and stderr for logs.

**Exit gate**

- ✅ Protocol tests for JSON-RPC initialization, tools/list, ping
- ✅ Error handling tests for unknown methods, malformed JSON
- ✅ Integration tests for create → navigate → snapshot flow
- ✅ Multiple context management and limits
- ✅ No mutation tools enabled yet.

### Phase 4 — Semantic interaction tools ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ Click, type, press-key, scroll, set-viewport operations
- ✅ Stable ref generation and invalidation
- ✅ Reference validation (wrong context, stale revision)
- ✅ Ambiguous selector handling (returns all matches)
- ✅ Hidden/disabled element handling
- ✅ Unicode text support
- ✅ Key modifiers (Shift, Ctrl, etc.)
- ✅ CSS and text locators
- ✅ Action cancellation via context
- ✅ Sequential mutation ordering
- ✅ Concurrent read-only operations

**Exit gate**

- ✅ Local workflow E2E: navigate → snapshot → click → type → wait → screenshot

### Phase 5 — Guarded JavaScript evaluation and diagnostics ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ `Runtime.SerializeValue()` for safe result serialization
- ✅ `engineContext.Evaluate()` using real Goja runtime
- ✅ Source length limit enforcement (MaxSourceBytes)
- ✅ Result size limits (MaxResultBytes)
- ✅ Context ID and page revision in results
- ✅ Proper error handling for JS exceptions
- ✅ Support for all JS value types: string, number, boolean, null, undefined, object, array, function
- ✅ Console.log integration
- ✅ Comprehensive tests: basic, numbers, booleans, objects, arrays, errors, syntax errors, cancellation

**Exit gate**

- ✅ JS evaluation tests pass
- ✅ Source length limits enforced
- ✅ Error handling works correctly

### Phase 6 — Hardening and local release ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ `internal/mcpserver/audit.go` — Structured audit logger to stderr
  - Single-line JSON format for log aggregation
  - Sensitive data redaction (passwords, tokens, cookies)
  - Tool call audit trail with context ID, duration, error code
- ✅ `internal/mcpserver/health.go` — Health and metrics reporter
  - Server info: name, version, protocol version, Go runtime
  - Runtime metrics: requests, errors, timeouts, denials
  - Active context tracking with configurable max
  - Memory and goroutine monitoring
  - Health check with degradation detection
- ✅ `internal/mcpserver/quota.go` — Rate limiter and quota tracker
  - Token bucket rate limiter (configurable capacity + refill rate)
  - Per-context request, navigation, screenshot, memory quotas
  - Concurrent-safe operations
- ✅ `internal/mcpserver/shutdown.go` — Shutdown handler
  - SIGTERM/SIGINT/SIGHUP signal handling
  - Graceful shutdown with configurable timeout
  - Stdin closure detection
  - Resource cleanup coordination
- ✅ `internal/mcpserver/hardening_test.go` — Comprehensive tests
  - Audit logging, redaction, concurrent safety
  - Rate limiter, quotas, refill behavior
  - Health metrics, unhealthy state detection
  - Shutdown trigger, wait, execute

**Exit gate**

- ✅ All hardening tests pass
- ✅ Audit events emitted to stderr only
- ✅ Sensitive data automatically redacted
- ✅ Server health check functional
- ✅ Resource quotas prevent DoS

### Phase 7 — Optional Streamable HTTP ✅ COMPLETE

**Status:** Complete — 2026-07-28

**Deliverables**

- ✅ Go upgraded to 1.25 (`go.mod` updated)
- ✅ `internal/mcpserver/http.go` — Streamable HTTP transport
  - Loopback-only by default (127.0.0.1, ::1, localhost)
  - Cryptographic session IDs (32 bytes hex)
  - Session timeout + cleanup
  - Origin validation against DNS-rebinding/CSRF
  - Host header validation
  - Optional bearer token authentication
  - Request body size limits
  - Per-session rate limiting
- ✅ `cmd/mcp-server/main.go` — HTTP mode via `--http` flag
  - `--bind`, `--port`, `--path`, `--auth`, `--auth-token` flags
  - Env var support for tokens (`env:VAR_NAME`)
- ✅ `internal/mcpserver/http_test.go` — Comprehensive tests
  - Loopback enforcement
  - Health/version endpoints
  - Initialize, tools/list, session cleanup
  - Origin and Host validation
  - Authentication (missing, wrong, correct token)
  - DELETE session cleanup
  - Rate limiting per session
  - Body size limits
  - Invalid JSON handling

**Security requirements**

- ✅ Loopback binding enforced (rejects 0.0.0.0, public IPs)
- ✅ Origin allowlist (default: loopback only)
- ✅ Host header validation (defeats DNS-rebinding)
- ✅ Bearer token auth with constant-time comparison
- ✅ Secure session IDs via crypto/rand
- ✅ Session timeout + cleanup goroutine
- ✅ Request body size limits (MaxBytesReader)
- ✅ Rate limiting per session
- ✅ DELETE handler for session cleanup

**Exit gate**

- ✅ All HTTP transport tests pass
- ✅ Loopback binding verified at startup
- ✅ Security headers set
- ✅ Same tool handlers used (stdio = http behavior invariant)

## 6. Release slices

| Release | Outcome | Status |
|---|---|---|
| MCP alpha 0 | Browser-control interfaces and fake; no server | ✅ Phase 0-1 complete |
| MCP alpha 1 | Headless deterministic navigation API | ✅ Phase 2 complete |
| MCP alpha 2 | Read-only stdio MCP server | ✅ Phase 3 complete |
| MCP beta 1 | Semantic actions | ✅ Phase 4 complete |
| MCP beta 2 | Guarded evaluation and diagnostics | ✅ Phase 5 complete |
| MCP v1 | Hardened local stdio release | ✅ Phase 6 complete |
| MCP v1.x optional | Streamable HTTP | ✅ Phase 7 complete |

## 7. Definition of done for every tool

A tool is not done until all of the following exist:

- Typed input and output models with JSON Schema descriptions and bounds.
- Unit tests written before handler code.
- Happy path, boundary, malformed-input, cancellation, timeout, close, and authorization-policy tests.
- Stable machine-readable error code plus safe human message.
- Context ID and page revision in results where applicable.
- Structured content and a concise text fallback where client compatibility needs it.
- Audit event classification and sensitive-field redaction.
- Documentation example that matches a golden test.
- No direct import from `internal/ui`.

## 8. Decisions — Phase 0 Accepted

All decisions resolved per recommendations:

| Decision | Resolution | Rationale |
|----------|------------|----------|
| Go toolchain | Go 1.25 + SDK v1.4.0 | ✅ Upgraded for Phase 7 HTTP support |
| First release | Read-only alpha first | Ship minimal viable product, add mutations incrementally |
| Context persistence | Ephemeral private only | Simplifies security model for v1 |
| Downloads | Deny in v1 | No automatic file writes; return metadata/denial |
| JavaScript | After semantic actions | Phase 4 first, Phase 5 for eval |

**Decision log:** 2026-07-28 — All Phase 0 decisions accepted.

## 9. Success metrics

- A client can complete the canonical workflow without UI dependencies or sleeps.
- 100 repeated fixture workflows produce identical semantic snapshots after normalization.
- Cancellation returns within the configured deadline and leaves no active network/JS work.
- All mutation operations are attributable in audit output without exposing secrets.
- Stdio protocol output contains zero non-MCP bytes.
- MCP integration adds no Fyne import below the shell layer.
- The adapter is replaceable without changing browser-control tests.

## 10. Source ledger

- [MCP 2025-11-25 overview and JSON Schema rules](https://modelcontextprotocol.io/specification/2025-11-25/basic)
- [MCP lifecycle and negotiation](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle)
- [MCP transports and HTTP security requirements](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
- [MCP tools](https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
- [MCP resources](https://modelcontextprotocol.io/specification/2025-11-25/server/resources)
- [MCP authorization](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)
- [MCP security best practices](https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices)
- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)

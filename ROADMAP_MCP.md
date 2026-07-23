# Goosie MCP Integration Roadmap

**Status:** Proposed — documentation-first, no implementation started

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

### Phase 0 — Documentation and decision lock

**Deliverables**

- Accept this roadmap and the four supporting documents.
- Resolve open decisions listed in Section 8.
- Add a decision log entry for chosen SDK/toolchain versions at implementation time.
- Define the supported-platform matrix and CI jobs.

**Exit gate**

- Tool contracts have examples, limits, error codes, and risk classifications.
- Security requirements have owners and executable-test mappings.
- No supporting document contradicts package ownership rules.

### Phase 1 — Browser-control contract, fake first

**Red**

- Write compile-time and behavior tests for `Browser`, `Context`, navigation state, snapshot, action serialization, cancellation, and close idempotency.
- Write table tests for typed errors and capability reporting.
- Build a fake browser-control implementation for MCP adapter tests.

**Green target**

- Introduce `internal/browsercontrol` interfaces and value types only.
- No MCP dependency and no Fyne import.
- One context owns one engine session, network service, DOM, JS session, renderer state, and bounded observability buffers.

**Exit gate**

- `go test -race ./internal/browsercontrol/...` passes.
- Lifecycle and cancellation tests are deterministic; no sleeps.

### Phase 2 — Extract the headless page pipeline

**Red**

- Fixture-server tests for redirect, HTTP error, invalid MIME, cancellation, superseded navigation, timeout, CSP, file URL denial, oversized response, and close-during-load.
- State-transition tests: created → navigating → parsing → interactive → complete, plus failed/cancelled.

**Green target**

- Move orchestration currently embedded in `cmd/browser/main.go` behind browser-control.
- Make `cmd/browser` consume the same service where practical, preventing MCP/browser drift.
- Define readiness: `commit`, `interactive`, or `complete`; never use arbitrary sleep.

**Exit gate**

- Existing GUI/headless tests remain green.
- Browser-control fixture tests pass under `-race` and repeated execution (`-count=50`).

### Phase 3 — Read-only MCP stdio server

**Red**

- Protocol tests for initialization, capability advertisement, tool listing, JSON Schema validation, unknown tool, malformed arguments, cancellation, stdout purity, and shutdown.
- Golden JSON tests for structured outputs.

**Green target**

- Add `internal/mcpserver` and `cmd/mcp-server`.
- Register read-only tools first: create/close/list contexts, navigate, snapshot, screenshot, page metadata, console/network/security reads.
- Use stdout only for MCP frames and stderr for logs.

**Exit gate**

- Official SDK client integration passes over an in-memory transport and real stdio subprocess.
- MCP Inspector smoke test is documented and reproducible.
- No mutation tools enabled yet.

### Phase 4 — Semantic interaction tools

**Red**

- Tests for stable reference generation/invalidation, ambiguous selectors, detached nodes, hidden/disabled elements, navigation caused by actions, event ordering, Unicode typing, key modifiers, and action cancellation.
- Tests prove two mutations on one context execute in request order.

**Green target**

- Add semantic query, click, type, press-key, scroll, and set-viewport operations.
- Serialize mutations per context while permitting bounded read-only snapshots.
- Return the post-action page revision and whether navigation began.

**Exit gate**

- No action requires direct Fyne widget access.
- Local workflow E2E covers navigate → snapshot → click → type → submit → wait → screenshot.

### Phase 5 — Guarded JavaScript evaluation and diagnostics

**Red**

- Tests for timeout, interruption, queue saturation, cycles/non-serializable values, huge output, thrown exceptions, navigation invalidation, and closed contexts.
- Security tests for secrets/redaction and script policy bypass attempts.

**Green target**

- Add evaluation through the `internal/js.Session` owner goroutine.
- Bound source length, execution duration, result depth/bytes, and console/network retention.

**Exit gate**

- Race detector, fuzz tests for result serialization, and leak tests pass.

### Phase 6 — Hardening and local release

**Red**

- Process-level tests for SIGTERM, client disconnect, malformed frame storms, backpressure, memory budget, 100-context churn, and screenshot compression bombs.
- Full threat-model regression suite.

**Green target**

- Add audit events, quotas, health/version reporting, build metadata, and operator documentation.
- Produce client configuration examples without shell interpolation hazards.

**Exit gate**

- `go test ./... -short`, race suites, MCP contract tests, and security tests pass.
- Zero stdout contamination.
- No known high-severity threat remains open.

### Phase 7 — Optional Streamable HTTP

**Prerequisite gate**

- Upgrade Goosie to Go 1.25 or newer.
- Re-evaluate and pin the latest stable official SDK.
- Review SDK release security changes since v1.4.0.
- Accept a separate HTTP authorization/operations ADR.

**Required scope**

- Loopback-only by default, explicit Origin allowlist, Host validation/DNS-rebinding protection, authentication, protocol-version header, secure session IDs, session ownership binding, request/body limits, and DELETE cleanup.
- OAuth only if non-loopback/remote operation is explicitly approved.

**Exit gate**

- Cross-origin, DNS rebinding, session fixation/hijacking, token audience, logout/revocation, proxy header, and SSRF tests pass.
- Remote bind is impossible without an explicit unsafe/production configuration and authentication.

## 6. Release slices

| Release | Outcome | Dependencies |
|---|---|---|
| MCP alpha 0 | Browser-control interfaces and fake; no server | Phases 0–1 |
| MCP alpha 1 | Headless deterministic navigation API | Phase 2 |
| MCP alpha 2 | Read-only stdio MCP server | Phase 3 |
| MCP beta 1 | Semantic actions | Phase 4 |
| MCP beta 2 | Guarded evaluation and diagnostics | Phase 5 |
| MCP v1 | Hardened local stdio release | Phase 6 |
| MCP v1.x optional | Streamable HTTP | Phase 7 and separate approval |

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

## 8. Decisions required before implementation

1. **Go toolchain:** remain on Go 1.24.9 with SDK v1.4.0 for stdio alpha, or upgrade globally before Phase 3. Recommended: keep the compatible alpha path, plan the upgrade before HTTP.
2. **First release capability:** read-only navigation/snapshots or include semantic actions. Recommended: ship read-only alpha first.
3. **Context persistence:** ephemeral private contexts only versus named persistent profiles. Recommended: private ephemeral only for v1.
4. **Downloads:** deny, return metadata, or write to an operator-configured sandbox. Recommended: deny automatic file writes in v1.
5. **JavaScript:** include in v1 or defer. Recommended: include only after semantic actions and hard limits.

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

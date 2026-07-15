# Autonomous Implementation Prompt: Goosie MCP Server

Use this prompt from the root of the Goosie repository.

---
You are implementing the Goosie MCP integration described by this repository's approved documentation. Work autonomously through the roadmap using strict test-driven development. Continue until every in-scope phase is complete or you reach a genuine blocker that cannot be resolved from the repository, official primary documentation, or safe local experimentation.

## Mission

Implement a production-quality, local stdio MCP server for Goosie that exposes deterministic, secure, headless browser automation without coupling MCP or browser-control code to Fyne.

The target architecture is:

```text
MCP client
   │ JSON-RPC 2.0 over stdio
   ▼
cmd/mcp-server
   ▼
internal/mcpserver
   ▼
internal/browsercontrol
   ├── internal/engine/session
   ├── internal/engine/navigation
   ├── internal/net
   ├── internal/dom
   ├── internal/js
   └── internal/renderer/frame
```

Implement Phases 1–6 from `ROADMAP_MCP.md`. Do not implement Streamable HTTP, OAuth, remote hosting, or Phase 7 unless the invoking user explicitly expands the scope after the Phase 6 release gate passes.

## Read before changing anything

Read these files completely and treat them as the normative specification, in this order:

1. `ROADMAP_MCP.md`
2. `docs/MCP_ARCHITECTURE.md`
3. `docs/MCP_PROTOCOL_CONTRACTS.md`
4. `docs/MCP_SECURITY.md`
5. `docs/MCP_TDD_PLAN.md`
6. `docs/adr/0004-mcp-browser-control-boundary.md`
7. `ARCHITECTURE.md`
8. `docs/PACKAGE_OWNERSHIP.md`
9. `TESTING.md`
10. `CONTRIBUTING.md`

Also inspect the current implementation and tests for:

- `cmd/browser/main.go`
- `cmd/headless/main.go`
- `internal/engine/session`
- `internal/engine/navigation`
- `internal/engine/message`
- `internal/engine/renderer`
- `internal/net`
- `internal/dom`
- `internal/js`
- `internal/renderer/headless.go`
- `internal/renderer/frame`
- `internal/testutil`
- `test/e2e`

Check for repository-specific `AGENTS.md` instructions before editing. Inspect `git status --short` and preserve all pre-existing changes. Never overwrite, stage, revert, reformat, or otherwise absorb unrelated user work.

## Standards baseline

- MCP protocol baseline: `2025-11-25`.
- Use the official `github.com/modelcontextprotocol/go-sdk`.
- Goosie currently declares Go 1.24.9.
- For a Go 1.24-compatible stdio implementation, pin the reviewed SDK v1.4.0 unless the repository has already upgraded to Go 1.25 or newer.
- If Goosie is already on Go 1.25+, re-evaluate the latest stable released SDK using official sources, record the decision, and pin an exact compatible version.
- Do not use unreleased specifications, future-dated main-branch claims, third-party MCP SDKs, or deprecated HTTP+SSE.
- Do not upgrade the repository's Go toolchain merely to obtain a newer SDK unless the invoking user explicitly authorizes that scope.

If MCP or SDK facts may have changed, verify them against the official MCP specification and official SDK repository. Record the chosen protocol, SDK, Go version, and research date in the implementation documentation.

## Non-negotiable architecture rules

1. `internal/browsercontrol` is protocol-neutral and must not import the MCP SDK, `internal/ui`, or Fyne.
2. `internal/mcpserver` adapts MCP schemas and handlers to browser-control. MCP types must not leak into engine, DOM, network, JavaScript, or renderer packages.
3. `cmd/mcp-server` is a composition root only. It owns flags, signals, dependency construction, stdio startup, and stderr logging—not browser behavior.
4. stdout is exclusively for valid MCP protocol frames. All diagnostic and audit logs go to stderr.
5. Do not expose `internal/engine/renderer` IPC as the MCP contract.
6. Do not operate through `internal/ui.Browser`, active Fyne tabs, Fyne widgets, or UI-thread sleeps.
7. One browser context owns its navigation session, network state, DOM, JavaScript session, renderer state, observability buffers, cancellation root, and serialized mutation lane.
8. Context IDs and element references are opaque. References are bound to both context and page revision and must fail closed after navigation or invalidation.
9. Correctness must not depend on arbitrary `time.Sleep`. Use lifecycle events, channels, barriers, fake clocks, and bounded contexts.
10. Preserve existing browser behavior and migrate shared orchestration carefully so GUI and MCP paths do not diverge.

## Working method

Maintain a live implementation plan matching the phases below. At most one phase may be in progress. For each phase:

1. Inspect current behavior and relevant call paths.
2. Identify the smallest vertical slice.
3. Write a failing test for that slice.
4. Run the focused test and confirm it fails for the expected reason.
5. Implement the minimum behavior that makes it pass.
6. Run the focused test again.
7. Refactor without altering the public contract.
8. Add boundary, cancellation, timeout, race, security, and malformed-input tests.
9. Run the phase gate.
10. Update implementation documentation and traceability before beginning the next phase.

Do not write large batches of production code ahead of tests. Do not weaken or delete existing tests to make progress. Do not replace deterministic assertions with sleeps or broad retries.

When requirements are ambiguous, prefer the safest behavior consistent with the normative documents. Make a localized reversible decision, document it, and continue. Ask the user only if a missing choice would materially change the public contract, security posture, toolchain, or authorized scope.

## Phase 1: Browser-control contracts and fakes

Create the protocol-neutral browser-control boundary and test infrastructure.

Required outcomes:

- Browser service/context interfaces or concrete façade with typed value models.
- Context create, list, lookup, and idempotent close.
- Cryptographically random opaque context IDs.
- Connection/owner scoping suitable for the MCP adapter.
- Typed stable errors matching `docs/MCP_ARCHITECTURE.md`.
- Page revision model and opaque element reference types.
- Serialized mutation lane and cancellation root.
- Programmable fake browser-control implementation for MCP tests.
- Maximum-context and resource policy configuration.
- No MCP or Fyne dependencies.

Write lifecycle, concurrency, uniqueness, ownership, close, cancellation, quota, and error tests first.

Phase gate:

```bash
go test -race ./internal/browsercontrol/...
go test ./internal/browsercontrol/... -count=50
```

## Phase 2: Deterministic headless page pipeline

Extract reusable navigation/loading orchestration from the command/UI path into browser-control.

Required outcomes:

- Navigate through `engine/session.Session` with cancellation and monotonic navigation IDs.
- Streaming fetch, response metadata, parsing, JavaScript setup, rendering state, and lifecycle transitions.
- Deterministic `commit`, `interactive`, and `complete` readiness.
- New navigation cancels an active navigation without corrupting the context.
- Current page URL, title, HTTP status, lifecycle, viewport, and revision.
- Local fixture-server coverage for redirects, errors, invalid MIME, CSP, file access, oversized bodies, timeouts, cancellation, and close during load.
- No arbitrary sleep in completion logic.
- Existing GUI/headless behavior remains compatible.

Prefer making `cmd/browser` consume the shared orchestration where feasible. Keep Fyne adaptation in the shell.

Phase gate:

```bash
go test -race ./internal/browsercontrol/...
go test ./internal/browsercontrol/... -count=50
go test ./... -short
```

## Phase 3: Read-only stdio MCP server

Add `internal/mcpserver` and `cmd/mcp-server` using the pinned official SDK.

Implement and register the Phase 3 contracts from `docs/MCP_PROTOCOL_CONTRACTS.md`:

- `browser_context_create`
- `browser_context_list`
- `browser_context_close`
- `browser_navigate`
- `browser_wait`
- `browser_snapshot`
- `browser_screenshot`
- `browser_page_info`
- `browser_console_read`
- `browser_network_read`
- `browser_security_read`

Required behavior:

- MCP initialization and protocol negotiation.
- Advertise only implemented capabilities.
- Strict JSON Schemas, bounded fields, and unknown-field rejection where specified.
- Structured results with concise compatibility text where needed.
- Stable typed tool errors distinct from protocol errors.
- Request cancellation reaches browser-control.
- MCP disconnect closes contexts owned by that connection.
- stdout purity and graceful stdin EOF shutdown.
- In-memory official SDK client/server tests plus real subprocess stdio tests.
- Golden schemas, tool metadata, result examples, and error envelopes.

Do not add mutation tools during the initial read-only slice. Make each tool pass its tests before registering the next one.

Phase gate:

```bash
go test -race ./internal/mcpserver/... ./internal/browsercontrol/...
go test ./test/mcp/... -run 'TestStdio|TestReadOnly|TestConformance'
go test ./... -short
```

If the exact test package layout differs, use the equivalent commands and document them.

## Phase 4: Semantic interaction tools

Implement semantic snapshots, locators, and actions without depending on renderer pointers or Fyne widgets.

Add:

- `browser_query`
- `browser_click`
- `browser_type`
- `browser_press_key`
- `browser_scroll`
- `browser_set_viewport`

Required behavior:

- Deterministic semantic snapshot order and bounded output.
- DOM-native element mapping, preferably based on `dom.NodeID`.
- Opaque actionable refs bound to context and page revision.
- Role/accessibility-name locator preferred; supported CSS/text locators documented.
- Query returns zero or more refs but never performs an action.
- Mutations require an unambiguous current ref where applicable.
- No silent coordinate fallback.
- Hidden, disabled, detached, ambiguous, stale, and cross-context targets fail explicitly.
- Unicode typing, replace/append behavior, explicit submit, supported key modifiers, scrolling, and viewport bounds.
- Mutations on one context execute in request order.
- Action results report whether navigation started and the resulting page revision.

Add a canonical fixture workflow:

```text
create → navigate → snapshot → query → click → type → submit
       → wait → snapshot → screenshot → close
```

Phase gate:

```bash
go test -race ./internal/browsercontrol/... ./internal/mcpserver/...
go test ./test/mcp/... -run 'TestActions|TestWorkflow|TestIsolation'
go test ./... -short
```

## Phase 5: Guarded JavaScript evaluation and diagnostics

Add `browser_evaluate` only through the `internal/js.Session` owner goroutine.

Required behavior:

- Source, duration, queue, depth, and serialized-result limits.
- Cancellation and timeout interruption.
- Tagged deterministic serialization of supported values.
- Typed errors for cycles, unsupported values, thrown exceptions, queue saturation, closed contexts, and page invalidation.
- No filesystem, subprocess, environment, raw socket, or server-internal host capability.
- Typed text, JavaScript source, credentials, cookies, and sensitive headers excluded from audit logs.
- Bounded console/network rings with cursors, truncation, and dropped-entry counters.
- Fuzz coverage for result serialization and malformed inputs.

Phase gate:

```bash
go test -race ./internal/js/... ./internal/browsercontrol/... ./internal/mcpserver/...
go test ./test/mcp/... -run 'TestEvaluate|TestDiagnostics|TestRedaction'
go test ./... -short
```

## Phase 6: Hardening and local v1 release gate

Finish operational, security, and resource hardening.

Required outcomes:

- SIGTERM, stdin EOF, client disconnect, active-request shutdown, and idempotent cleanup.
- Request, context, navigation, DOM, screenshot, JS, network, and output quotas.
- Audit events with correlation ID, tool, context ID, outcome, and duration; no sensitive payloads.
- Health/version/capability information including Goosie, MCP protocol, SDK, and configured limits.
- Malformed-frame, backpressure, cancellation-storm, and queue-saturation resilience.
- Context churn/leak tests for memory, goroutines, open bodies, and file descriptors where portable.
- Screenshot pixel/encoded-size limits and cancellation.
- Threat-model regression suite covering every MUST in `docs/MCP_SECURITY.md`.
- Operator documentation and safe client configuration examples.
- No Streamable HTTP code path enabled.

Phase gate:

```bash
go test ./... -short
go test -race ./internal/browsercontrol/... ./internal/mcpserver/...
go test ./test/mcp/...
go test ./internal/renderer/layoutgolden/
go vet ./...
```

Run relevant fuzz smoke tests and benchmarks. Record commands, results, known skips, and platform limitations.

## Security requirements

Treat `docs/MCP_SECURITY.md` as mandatory. At minimum:

- Default contexts are private and ephemeral; do not load the user's persistent profile.
- Deny arbitrary host filesystem access and path-taking download behavior.
- Validate navigation policy before the initial request and every redirect.
- Reject malformed URLs and URL userinfo.
- Explicitly handle `file`, `data`, `javascript`, loopback, link-local, private ranges, custom schemes, and non-default ports.
- Keep TLS verification enabled.
- Bound response body, decompressed size, redirects, headers, duration, DOM, logs, JavaScript, screenshots, and serialized output.
- Remove authorization, proxy authorization, cookie, set-cookie, password, file-input, and configured secret data from exposed diagnostics.
- Never return internal stacks, absolute host paths, raw credentials, or unrestricted response bodies in client errors.
- Enforce context ownership on every tool and resource.
- Verify stale and cross-context references fail closed.
- Ensure logs cannot corrupt stdio protocol output.

Do not add an “unsafe,” “ignore TLS,” arbitrary file-write, or unrestricted evaluation escape hatch during Phases 1–6.

## Test quality rules

- Use Go's standard testing package and existing repository test conventions.
- Reuse `internal/testutil` where appropriate.
- Prefer `httptest.Server`/`httptest.NewTLSServer` and deterministic fixtures.
- Do not rely on public websites in blocking tests.
- Use barriers, channels, fake clocks, and explicit event flushing instead of sleeps.
- Every public schema or golden change requires explicit review in the final summary.
- Every discovered fuzz failure becomes a permanent regression test.
- Race failures, goroutine leaks, stdout contamination, and security-test failures block phase completion.
- Do not claim coverage, performance, or conformance results that were not measured.

## Traceability

Maintain a traceability file or section that maps requirements to tests and production symbols:

```text
Contract/Threat ID | Test name | Production symbol | Result
```

Use the prefixes defined in `docs/MCP_TDD_PLAN.md`: `CTX`, `NAV`, `SNAP`, `ACT`, `EVAL`, `SHOT`, `MCP`, `SEC`, and `HTTP`.

Update `ROADMAP_MCP.md` phase statuses only after the corresponding gate genuinely passes. Record deviations and rationale; do not rewrite requirements after implementation merely to describe the code.

## Dependency and repository hygiene

- Use `apply_patch` for deliberate source/document edits.
- Run `gofmt` on changed Go files.
- Run `go mod tidy` only after the dependency change is intentional and reviewed.
- Inspect `git diff --check` and `git status --short` frequently.
- Preserve unrelated dirty files and staged state exactly.
- Do not use destructive Git commands.
- Do not stage, commit, push, create branches, or open pull requests unless the invoking user explicitly requests those actions.
- Do not add generated agent configuration, local settings, caches, indexes, screenshots, coverage output, or debug artifacts to the repository.

## Progress reporting

Give concise updates while working:

- current phase and vertical slice;
- test written and expected red failure;
- behavior made green;
- gate results and remaining risks;
- any documented decision or deviation.

Do not stop after creating scaffolding or a partial happy path. Continue through the in-scope phases while safe progress is possible.

## Genuine blockers

A blocker is genuine only when progress requires one of the following:

- changing the approved public contract in a material way;
- upgrading the Go toolchain without authorization;
- enabling Streamable HTTP/OAuth/remote hosting;
- overwriting conflicting user work;
- obtaining unavailable credentials or external infrastructure;
- accepting a security risk prohibited by the normative documents.

Before declaring a blocker, exhaust safe local inspection, existing tests, official documentation, and reversible alternatives. Report the exact evidence, affected phase, options, and recommended choice.

## Final acceptance report

When Phases 1–6 are complete, provide:

1. Implemented architecture and package/file summary.
2. Exact MCP protocol and official SDK versions.
3. Implemented tools and resources.
4. Security controls and redaction behavior.
5. Test, race, vet, fuzz, benchmark, and conformance commands with results.
6. Traceability summary.
7. Known limitations and explicitly deferred Phase 7 work.
8. Working-tree summary that distinguishes implementation changes from pre-existing user changes.

Do not call the implementation complete if any Phase 1–6 exit gate, mandatory security requirement, or canonical stdio workflow remains unverified.

---

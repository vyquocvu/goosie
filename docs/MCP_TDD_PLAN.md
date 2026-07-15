# MCP Test-Driven Development Plan

**Status:** Proposed

**Rule:** production behavior is introduced only after a failing test demonstrates the contract.

## 1. TDD loop

For each vertical slice:

1. **Red:** add the smallest deterministic test at the lowest useful layer. Confirm it fails for the intended reason.
2. **Green:** implement only enough behavior to satisfy the contract.
3. **Refactor:** remove duplication, tighten names/boundaries, and keep all tests green.
4. **Contract check:** run JSON/schema golden tests and the next broader integration layer.
5. **Hardening:** add cancellation, boundary, race, leak, fuzz, and security cases before declaring the slice complete.

Tests may not depend on public internet, arbitrary sleeps, wall-clock ordering without a fake clock, or the GUI event loop for core behavior.

## 2. Test layers and planned locations

| Layer | Planned location | Purpose |
|---|---|---|
| Browser-control unit | `internal/browsercontrol/*_test.go` | State, refs, locators, limits, typed errors, serialization. |
| Browser-control integration | `internal/browsercontrol/integration_test.go` | Local fixture fetch → DOM/JS/render lifecycle. |
| MCP handler unit | `internal/mcpserver/*_test.go` | Fake browser-control, schemas, mapping, ownership, cancellation. |
| MCP in-memory integration | `internal/mcpserver/integration_test.go` | Official SDK client/server lifecycle without subprocess timing. |
| Stdio process integration | `test/mcp/stdio_test.go` | Real binary, stdout purity, shutdown, signals. |
| Security | `internal/test_suite/security/mcp_test.go` | Threat-model regression matrix. |
| E2E workflow | `test/mcp/workflow_test.go` | Full local-site workflows. |
| Conformance | `test/mcp/conformance_test.go` | Protocol initialization/capability/tool/resource behavior. |
| Fuzz | co-located `Fuzz*` tests | URLs, locators, schemas, outputs, frames. |
| Bench/leak | co-located benchmarks/churn tests | Latency, allocation, bounded growth. |

Exact directories may be adjusted to package visibility, but layer ownership must remain.

## 3. Test infrastructure required before feature code

### Fake browser-control

A programmable fake records calls, blocks until cancellation, emits typed errors, advances page revisions, and returns oversized/truncated fixtures. MCP handler tests must never need the real renderer.

### Local fixture server

Use `httptest.Server` with endpoints for:

- static HTML and CSS;
- redirects and redirect loops;
- delayed/chunked/cancelled responses;
- oversized and compressed bodies;
- attachments and invalid MIME;
- forms, click navigation, DOM mutation, timers, console messages;
- CSP, cookies, sensitive headers, TLS, HTTP errors;
- deterministic screenshot content.

### Fake clock and event recorder

Waits, idle expiry, and timeouts need injectable clock/timer behavior. Event assertions use a concurrency-safe recorder and barriers/channels, not sleeps.

### Golden data

Store reviewed golden files for:

- tool input JSON Schemas;
- tool list metadata/descriptions;
- structured result examples;
- semantic snapshots;
- redacted console/network/security results;
- error envelopes.

Golden updates require explicit review because they are public-contract changes.

## 4. First-test order by slice

### Slice A — service lifecycle

1. create returns unique private context;
2. list is connection-scoped;
3. close is idempotent;
4. operation after close returns `context_not_found`/closed contract;
5. parent cancellation closes child work;
6. maximum-context quota fails before allocating resources;
7. concurrent create/close is race-clean.

### Slice B — navigation

1. state sequence and page revision;
2. new navigation cancels the previous one;
3. waitUntil variants return only after their signal;
4. timeout/cancel leaves context reusable;
5. policy rejects scheme/IP/redirect before data exposure;
6. metadata and failures map to stable results;
7. close during navigation releases the body and goroutines.

### Slice C — snapshot and refs

1. deterministic semantic order;
2. actionable nodes receive refs;
3. secrets are redacted;
4. max depth/nodes/bytes set `truncated` accurately;
5. old refs fail after revision change;
6. refs cannot cross contexts;
7. raw DOM mutations cannot produce dangling unsafe pointers.

### Slice D — MCP read-only adapter

1. initialize/version/capabilities;
2. list tools/resources and exact schemas;
3. invalid input rejected without browser call;
4. fake result becomes structured MCP result;
5. typed browser error becomes `isError` result;
6. request cancellation reaches fake context;
7. logs never reach stdout;
8. disconnect closes owned contexts.

### Slice E — actions

1. query result cardinality and deterministic refs;
2. click current ref dispatches expected DOM event;
3. click old/hidden/disabled ref fails explicitly;
4. type replace/append/submit semantics;
5. Unicode and modifier key behavior;
6. action-triggered navigation reports new navigation ID/revision;
7. simultaneous mutations preserve request order.

### Slice F — evaluation

1. primitives and structured serialization;
2. exceptions and safe messages;
3. cycle/unsupported value handling;
4. timeout interrupts execution;
5. cancellation and close interrupt queued/running work;
6. result/source/depth limits;
7. no host capability escape;
8. queue saturation is bounded and typed.

### Slice G — screenshots and diagnostics

1. image dimensions/MIME and deterministic fixture hash/tolerance;
2. page revision captured atomically;
3. pixel and encoded-byte caps;
4. cancellation during raster/encode;
5. console/network cursors and dropped counts;
6. sensitive header/form/console redaction.

## 5. MCP protocol matrix

| Scenario | Required assertion |
|---|---|
| Initialization first | Pre-initialize operation is rejected. |
| Version negotiation | Baseline accepted; unsupported version handled per protocol/SDK. |
| Capabilities | Advertise only implemented tools/resources; no prompts/sampling/client features. |
| Tool schemas | Valid JSON Schema 2020-12-compatible shapes and server validation. |
| Cancellation | Cancelled request stops work and does not poison session. |
| Ping | Respond while no long mutation holds global locks. |
| Shutdown | Stdio EOF produces graceful cleanup and process exit. |
| Unknown method/tool/resource | Correct protocol/tool-level error, no panic. |
| Malformed/oversized message | Bounded rejection and continued service where safe. |
| Parallel clients/contexts | Ownership and isolation maintained. |

## 6. Non-functional gates

### Race and repeatability

```bash
go test -race ./internal/browsercontrol/... ./internal/mcpserver/...
go test ./internal/browsercontrol/... ./internal/mcpserver/... -count=50
```

### Core regression

```bash
go test ./... -short
go test ./internal/renderer/layoutgolden/
```

### Fuzz smoke

Run each MCP/browser-control fuzz target for a CI smoke budget and a longer scheduled budget. Any discovered corpus input becomes a permanent regression case.

### Resource growth

Churn 100+ contexts and navigations, then assert bounded retained memory/goroutine/file-descriptor deltas after cleanup and GC stabilization. Avoid brittle exact counts; define reviewed tolerances.

### Performance budgets

Initial budgets to validate during Phase 0/1:

- handler overhead with fake browser: p95 under 5 ms;
- semantic snapshot of 2,000 fixture nodes: p95 under 100 ms and bounded allocations;
- cancellation observed within 250 ms for cooperative network/wait operations;
- stdio server idle memory and per-context budget recorded as baselines before release.

Budgets are regression alarms, not permission to sacrifice correctness.

## 7. Traceability template

Each implementation PR adds a table:

| Contract/Threat ID | Test name | Production symbol | Result |
|---|---|---|---|
| `NAV-CANCEL-01` | `TestNavigateSupersedesActiveLoad` | `browsercontrol.Context.Navigate` | pass |

IDs should use `CTX`, `NAV`, `SNAP`, `ACT`, `EVAL`, `SHOT`, `MCP`, `SEC`, or `HTTP`. This keeps roadmap claims auditable.

## 8. CI staging

1. Fast PR: formatting/vet, unit, schema golden, short suite.
2. PR integration: local fixtures, stdio subprocess, race on affected packages.
3. Scheduled: full race, fuzz, churn/leak, benchmarks, platform matrix.
4. HTTP-only future job: raw transport/auth/origin/host/session security suite.

No test may skip because the public internet is unavailable; external compatibility smoke tests, if ever added, are non-blocking and separate.

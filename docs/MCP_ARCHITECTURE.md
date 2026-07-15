# MCP Architecture and Browser-Control Boundary

**Status:** Proposed

**Related ADR:** [ADR 0004](adr/0004-mcp-browser-control-boundary.md)

## 1. Target architecture

```text
MCP client
   │ JSON-RPC 2.0 (stdio first)
   ▼
cmd/mcp-server                 composition, flags, signals, stderr logging
   ▼
internal/mcpserver             schemas, MCP handlers, result/error mapping
   ▼
internal/browsercontrol        UI-independent browser service and policy
   ├── engine/session          navigation lifecycle and cancellation
   ├── engine/navigation       IDs, priority, origin/file policy
   ├── net                     fetch, cookies, CSP, metadata, security
   ├── dom                     document store and semantic snapshot
   ├── js                      owner-goroutine runtime and actions
   └── renderer/frame          loaded-page raster/screenshot
```

`internal/mcpserver` depends on `internal/browsercontrol`, never the reverse. Neither package imports `internal/ui`. MCP sessions and Goosie browser contexts are distinct: one MCP connection may own multiple browser contexts, while one browser context owns one active page lifecycle.

## 2. Why extraction is necessary

The reusable primitives are already well separated, but full-page loading is assembled in `cmd/browser/main.go`. That path calls session navigation, fetches a stream, decides downloads/content handling, reads HTML, initializes page state, and updates Fyne. It is unsuitable as an API because:

- it accepts `*ui.Browser` and crosses the UI thread;
- completion is callback/sleep-oriented in headless mode;
- it mixes navigation, download UI, fallback content, and rendering;
- it does not expose a stable page revision or semantic element identity;
- it cannot safely serve multiple independent MCP contexts.

The extraction must preserve GUI behavior while moving engine orchestration into browser-control. `cmd/browser` should become a consumer of that service rather than remain a second navigation implementation.

## 3. Ownership model

### Server

Owns protocol configuration, the context registry, global limits, shutdown, and audit sink. A server has a maximum number of contexts and maximum aggregate memory/network work.

### Browser context

Owns:

- a unique unguessable context ID;
- an `engine/session.Session`;
- network service, cookie jar, cache policy, and private storage;
- current DOM/document and monotonically increasing page revision;
- a JS session and its single owner goroutine;
- renderer/screenshot state;
- bounded console, network, security, and audit rings;
- a mutation queue and cancellation root.

Context close is idempotent and cancels navigation, JS, network, renderer work, and waiters before releasing resources.

### Page revision and element references

Every committed document increments `pageRevision`. A snapshot issues opaque references of the form `e_<revision>_<token>`; clients must treat them as opaque. A reference is valid only for its context and revision. Navigation, document replacement, and destructive DOM mutation invalidate old references. This prevents a delayed action from targeting a different page.

References map to DOM-native `NodeID` internally, not renderer pointers or Fyne widgets. The mapping is bounded and cleared on revision changes.

## 4. Concurrency model

- Registry operations are concurrency-safe.
- Each context has one serialized mutation lane: navigate, click, type, key, scroll, viewport, and evaluate.
- Read-only snapshots may execute concurrently only when backed by an immutable committed snapshot; otherwise they join the lane.
- MCP request cancellation propagates through the handler to browser-control and into navigation/network/JS contexts.
- Disconnect does not imply that already-committed browser side effects are rolled back. Explicit cancellation is honored at safe boundaries.
- Slow clients cannot block engine event dispatch; observability uses bounded rings with dropped-entry counters.

No correctness path may rely on `time.Sleep`. Waits subscribe to lifecycle/page-revision signals and always have a deadline.

## 5. Browser-control conceptual API

Names below specify responsibilities, not final Go syntax:

```text
Service.CreateContext(options) -> ContextInfo
Service.ListContexts() -> []ContextInfo
Service.CloseContext(id)

Context.Navigate(url, waitUntil, timeout) -> NavigationResult
Context.Wait(condition, timeout) -> WaitResult
Context.Snapshot(options) -> PageSnapshot
Context.Screenshot(options) -> Image
Context.Query(locator) -> []ElementRef
Context.Click(ref, options) -> ActionResult
Context.Type(ref, text, options) -> ActionResult
Context.PressKey(key, modifiers) -> ActionResult
Context.Scroll(target/delta) -> ActionResult
Context.SetViewport(width, height, scale) -> Viewport
Context.Evaluate(source, args, limits) -> EvaluationResult
Context.Console(since, limit) -> LogPage
Context.Network(since, limit) -> NetworkPage
Context.Security() -> SecuritySnapshot
```

All methods accept `context.Context`, return typed sentinel errors, and avoid protocol-specific types.

## 6. Navigation state and readiness

Reuse the engine session lifecycle:

```text
created → navigating → parsing → interactive → complete
                         └──────────────→ failed
any active state ───────────────────────→ cancelled
any state ──────────────────────────────→ closed
```

MCP exposes only deterministic waits:

- `commit`: response accepted and document identity established;
- `interactive`: DOM available and synchronous scripts processed to the supported extent;
- `complete`: browser-control has completed its supported document/subresource pipeline.

The result states exactly which condition was met. “Network idle” is deferred until Goosie can define and observe it without guessing.

## 7. Semantic snapshot

The default snapshot is compact text/structured data designed for model consumption, not raw HTML. Each included node has role/tag, accessible or visible name, selected state/attributes, and an optional opaque reference when actionable.

Requirements:

- deterministic traversal and attribute order;
- depth, node-count, text-length, and total-byte limits;
- explicit truncation metadata;
- no computed style dump by default;
- password values, tokens, cookies, hidden form values, and configured secret patterns redacted;
- raw HTML available only as a separately classified diagnostic resource with tighter limits.

## 8. Screenshot architecture

`RenderHTMLToImage` proves the pure-Go raster path, but it takes HTML rather than a live committed context. Browser-control needs a live-page snapshot boundary that captures an immutable display list/frame state, then encodes PNG outside mutation-critical sections.

Screenshot constraints:

- viewport by default; full-page is deferred until bounded tiling exists;
- maximum width, height, pixels, and encoded bytes;
- PNG only initially;
- cancellation during render/encode;
- return MCP image content, never an implicit filesystem path.

## 9. Error model

Browser-control defines stable codes that MCP maps into tool errors:

| Code | Meaning | Retry guidance |
|---|---|---|
| `context_not_found` | Unknown/closed context | Create or select a context. |
| `page_changed` | Reference belongs to an old revision | Take a new snapshot. |
| `element_not_found` | Locator/ref cannot resolve | Refresh snapshot/query. |
| `ambiguous_target` | Locator matched multiple nodes | Refine locator. |
| `invalid_state` | Operation cannot run in current lifecycle state | Wait or navigate. |
| `policy_denied` | Security/operator policy rejected action | Do not retry unchanged. |
| `deadline_exceeded` | Operation did not finish in time | Retry with bounded timeout. |
| `cancelled` | Client/request cancelled | Retry if still desired. |
| `limit_exceeded` | Input/output/resource quota exceeded | Reduce requested scope. |
| `unsupported` | Goosie lacks required web capability | Choose another action. |
| `internal` | Unexpected failure with correlation ID | Inspect server logs. |

Expected browser/action failures return a successful MCP tool response with `isError: true` and structured details. Protocol misuse uses JSON-RPC errors as appropriate. Internal details and stacks stay in stderr/audit logs.

## 10. Package and dependency rules

- `internal/browsercontrol` belongs to engine layer 3 and may import engine/lower packages, never shell/UI.
- `internal/mcpserver` is an adapter layer beside the shell and imports browser-control plus the official SDK.
- `cmd/mcp-server` contains no browser logic.
- MCP types never leak into DOM, renderer, JS, net, or engine session packages.
- Internal engine IPC remains versioned independently; do not tunnel it as MCP.
- The MCP SDK version is pinned, not floating.

## 11. Observability

- All stdio logs go to stderr.
- Audit events include timestamp, connection principal/local process identity when available, context ID, tool, outcome, duration, and correlation ID.
- Never log JavaScript source, typed text, page bodies, cookies, authorization headers, or screenshot bytes by default.
- Metrics: active contexts, requests by tool/outcome, latency, cancellations, limit denials, dropped event entries, navigation bytes, snapshot/screenshot bytes, and goroutines at churn-test boundaries.

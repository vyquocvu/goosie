# Rendering and Resource Pipeline Implementation Plan

## Goal

Implement the resource lifecycle described in
[`website/docs/webview-architecture.md`](website/docs/webview-architecture.md): load external CSS
and classic JavaScript from URLs, apply correct URL/CSP/navigation handling, and
move toward ordered script execution and targeted rendering invalidation.

## Current State

- `cmd/browser.loadPageAsync` streams the response from HTTP, but buffers the full
  document before rendering.
- `internal/dom` can discover external CSS, scripts, and images during streaming
  parsing through `ParseConfig.OnResource`; the browser does not consume it yet.
- `Renderer.RenderHTML` parses inline CSS and launches `loadExternalCSS`
  asynchronously after the first frame.
- `cmd/browser.updateUIWithContent` executes all inline scripts, then fetches and
  runs all external scripts. This loses mixed document order and ignores `defer`
  and `async`.
- JS DOM mutations currently cause a full HTML re-render.

## Design Constraints

- Preserve the existing `Renderer.RenderHTML` and `js.Runtime.RunScript` APIs.
  They are widely used by tests, demos, headless rendering, and the UI.
- Own document-level state in a new coordinator rather than splitting it between
  the browser command and renderer.
- All subresources use the active navigation context, final document URL, CSP
  policy, common fetcher, and navigation scheduler.
- Keep the first release to classic scripts. Module scripts remain on the existing
  unsupported-feature path.
- Preserve document order for stylesheets and classic scripts even when requests
  complete out of order.

## Target Ownership

```text
cmd/browser
  └── DocumentLoader / ResourceCoordinator
        ├── navigation.Session + Scheduler
        ├── internal/net.Fetcher + final URL + CSP
        ├── internal/dom streaming resource discovery
        ├── CSS fetch / parse / ordered stylesheet assembly
        ├── classic script fetch / execution queue
        └── renderer + JS runtime integration
```

The coordinator is the only component that decides whether a discovered resource
is valid, allowed, scheduled, fetched, or cancelled. The renderer receives a
stable document snapshot plus an ordered stylesheet set; the JS runtime receives
scripts in the order selected by the coordinator.

## Milestones

### 0. Characterization Tests

Add tests that lock in the intended behavior before replacing existing paths.

- Relative and absolute resource URL resolution after a redirect.
- CSP rejection prevents an HTTP request for CSS and scripts.
- New navigation cancels all in-flight subresources.
- Stylesheet source order wins over response completion order.
- Mixed inline/external classic scripts execute in document order.
- The document is not refetched or re-executes scripts after a JS mutation.

### 1. Resource Coordinator Foundation

Create an internal document-loading coordinator with explicit dependencies:

- active navigation `context.Context` and `navigation.Scheduler`
- final document URL and parsed CSP
- `net.Fetcher`
- callbacks or interfaces for CSS application, script execution, image loading,
  lifecycle metrics, and errors

For every resource, the coordinator will:

1. resolve the URL against the final document URL;
2. check the applicable CSP directive;
3. call `Scheduler.AddResource` with the correct priority;
4. fetch using the returned resource context;
5. call `RemoveResource` exactly once when the load completes, fails, or is
   cancelled;
6. ignore results belonging to an inactive navigation.

Initial priorities are `PriorityBlockingCSS`, `PriorityScript`, and the existing
visible/deferred image priorities.

### 2. Connect Streaming Discovery

Use `dom.ParseConfig.OnResource` in the main-document path to register resource
loads as tags arrive. Extend `dom.Resource` only as necessary to retain:

- document position;
- resource kind;
- source URL;
- script mode (`classic`, `async`, `defer`, `module`);
- integrity/cross-origin metadata when those policies are supported.

Do not make `internal/dom` perform networking. It remains a parser and reports
facts to the coordinator.

### 3. Blocking External CSS

Replace `Renderer.loadExternalCSS` as the browser navigation path with coordinator
owned CSS loading.

- Fetch and parse every `<link rel="stylesheet">` through the coordinator.
- Merge external rules with inline `<style>` rules in document order.
- Wait for required blocking stylesheets before the first styled frame.
- Permit a timeout/error fallback that renders with the stylesheets successfully
  loaded so far, while recording a failure metric.
- Keep `Renderer.loadExternalCSS` temporarily as a compatibility fallback for
  direct `RenderHTML` callers; do not use it from browser navigation.

Add a renderer entry point that accepts an already parsed document and assembled
stylesheet, or an equivalent immutable render snapshot. Keep `RenderHTML` as a
wrapper for legacy callers.

### 4. Ordered Classic Script Queue

Build an ordered list of both inline and external classic scripts during parsing.

- Parser-blocking scripts pause the document progression until fetched and run.
- Inline scripts run at their source position.
- External scripts are fetched through the coordinator, CSP checked, then run at
  their source position.
- Script failures are reported but do not stop subsequent classic scripts unless
  the engine's policy explicitly requires it.
- Script mutations update the document snapshot before the next rendering step.

Initially, execute this queue after document bytes are available but before the
final browser render. This delivers correct ordering without requiring a complete
incremental DOM/JS integration in the first implementation.

### 5. `defer`, `async`, and Document Lifecycle

Add separate queues and milestones:

- `defer`: fetch in parallel; execute in source order after parsing.
- `async`: execute once ready; no source-order guarantee.
- Dispatch `DOMContentLoaded` after parser completion and deferred scripts.
- Dispatch `load` after required document resources settle.

Keep module scripts unsupported and report them through the existing fallback
mechanism. Do not silently treat modules as classic scripts.

### 6. Mutation-Specific Rendering

Replace the current full `RenderHTML(mutatedHTML)` callback with a mutation record
that identifies the affected DOM/style scope.

- Style-only changes invalidate style, layout only when geometry changes, and
  paint only when geometry is unchanged.
- Keep immutable resource identities so a mutation cannot rediscover/re-fetch an
  unchanged stylesheet or script.
- Coalesce mutation bursts before rendering a frame.

This milestone depends on the renderer's existing incremental invalidation work;
it should not be bundled with the resource coordinator introduction.

### 7. CSS Secondary Resources

After link stylesheets are stable, discover and schedule:

- `@import` rules, preserving CSS order and cycle detection;
- `@font-face` URLs;
- CSS `url(...)` images.

Each resource remains subject to the active navigation context, origin limits,
CSP, and cancellation.

## Files Expected to Change

| Area | Likely files | Change |
|---|---|---|
| Browser orchestration | `cmd/browser/main.go` | Replace ad-hoc CSS/script loops with the coordinator |
| DOM discovery | `internal/dom/treebuilder.go` | Report resource ordering and script metadata |
| Navigation | `internal/engine/navigation/priority.go` | Integrate existing `AddResource`/`RemoveResource` lifecycle |
| New document loader | `internal/engine/...` | Coordinator, queues, result types, metrics |
| Rendering adapter | `internal/renderer/renderer.go` | Render an immutable document/style snapshot; retain legacy wrapper |
| JS integration | `internal/js/runtime.go` | Lifecycle/event hooks only; preserve `RunScript` behavior |

## Risk and Mitigation

GitNexus impact analysis reports high or critical risk for the current central
entry points:

| Symbol | Risk | Direct dependents | Mitigation |
|---|---:|---:|---|
| `loadPageAsync` | High | 3 | Migrate browser call sites together; retain signature initially |
| `Renderer.RenderHTML` | Critical | 7 | Add a new snapshot entry point and keep `RenderHTML` as a wrapper |
| `Renderer.loadExternalCSS` | Critical | 1 | Stop using it only in browser navigation; leave fallback behavior intact |
| `Runtime.RunScript` | Critical | 6 | Build ordering outside the runtime; do not change execution semantics |
| `Scheduler.AddResource` | Low | 0 | Exercise it through coordinator integration tests |

Every implementation milestone must run fresh GitNexus impact analysis before a
symbol is edited. Changes must be separately reviewable and independently tested.

## Acceptance Criteria

The initial browser pipeline is complete when a fixture with relative external
CSS, an inline script, and external classic scripts:

1. uses the final redirected page URL to resolve each resource;
2. rejects CSP-blocked resources before network fetch;
3. renders its first styled frame with its required stylesheet rules;
4. executes mixed inline/external classic scripts in source order;
5. cancels unfinished subresources on new navigation;
6. does not refetch completed CSS/scripts after a DOM mutation; and
7. leaves direct renderer, headless, and demo callers working unchanged.

## Verification

- Unit tests for URL resolution, CSP, queue ordering, priority assignment, and
  cancellation/cleanup.
- `httptest` integration fixtures with delayed CSS/script endpoints to prove
  document order is independent of response order.
- Browser integration tests for first styled paint and mutation behavior.
- Race tests for navigation during resource loads and mutation bursts.
- Existing renderer golden/layout, JS integration, and end-to-end suites.

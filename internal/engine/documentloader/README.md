# documentloader

The `documentloader` package owns document-level subresource lifecycle:
URL resolution, CSP gating, navigation-scoped scheduling, ordered fetch,
and result delivery to renderer/JS callbacks.

## Why it exists

Prior to this package the browser navigation path (`cmd/browser`) and the
renderer (`internal/renderer.Renderer.RenderHTML` / `loadExternalCSS`) split
subresource work between them. External CSS was discovered after the full
parse, classic scripts were executed in a single pass after first render,
and CSP was checked inconsistently.

This package introduces a single coordinator that, for every resource
discovered during a navigation:

1. resolves the URL against the final document URL (after redirects);
2. consults the document's parsed Content-Security-Policy;
3. registers the load with `internal/engine/navigation.Scheduler.AddResource`
   under the correct priority;
4. fetches through the active navigation context;
5. calls `Scheduler.RemoveResource` exactly once on completion, failure,
   or cancellation;
6. ignores results from an inactive navigation.

The renderer receives a stable document snapshot plus an ordered
stylesheet set. The JS runtime receives scripts in the order the
coordinator selected. `Renderer.RenderHTML`, `Runtime.RunScript`,
`cmd/browser.loadPageAsync`, and `Renderer.loadExternalCSS` are not
modified by M1; they are consumers of later milestones.

## Non-goals (M1)

- No actual style application. CSS bodies are accumulated and emitted via
  `Callbacks.OnStylesheet`. The renderer owns parsing and rule merge.
- No actual script execution. Script bodies are accumulated and emitted
  via `Callbacks.OnScript` in document order. Ordering across mixed
  inline/external scripts is enforced by `HandleResource` sequencing,
  not by the JS runtime.
- No `defer`/`async` semantics. M1 treats every `<script src>` as a
  classic parser-blocking script. M5 adds the deferred and async queues.
- No CSS secondary resources (`@import`, `@font-face`, `url(...)`).
  Scheduled by M7.
- No mutation invalidation. The coordinator is unaware of DOM mutations
  after `HandleDocumentEnd`. M6 wires that.

## Files

| File | Purpose |
|---|---|
| `resource.go` | `Resource`, `ResourceKind`, `ScriptMode`, result types |
| `url.go` | `ResolveURL(base, ref)` — absolute + relative URL resolution |
| `coordinator.go` | `Coordinator` struct, `Options`, `Callbacks`, lifecycle |
| `coordinator_test.go` | Characterization tests (M0) and coordinator tests |

## Dependencies

- `internal/engine/navigation` — scheduler, ID, priority
- `internal/engine/metrics` — phase recorder
- `internal/net` — `CSPPolicy`, `Fetcher`

The package does not import `fyne.io/fyne/v2`, `internal/renderer`,
`internal/dom`, `internal/js`, or `internal/css`. It is pure
orchestration; the renderer/JS runtime consume its outputs.
# Supported Web Platform

This document defines the Goosie v2 web-platform scope. It is a contributor
contract, not a complete statement of current implementation coverage.

Full modern web application compatibility is not a v2 goal. Goosie v2 targets
HTML/CSS documents, JavaScript-light applications, internal tools, dashboards,
feeds, blogs, documentation, forms, tables, images, and kiosk-style content.

## Status Categories

### Supported

Supported features are in the v2 target surface and should have predictable
behavior, tests, and documented limits before they are marked complete.

### Partial

Partial features are useful today but intentionally incomplete. New work may
extend them only when the cost fits the lightweight engine target.

### Planned

Planned features belong in v2 but are not part of the currently supported
contract.

### Fallback

Fallback features are accepted inputs that should degrade predictably without
crashing, hanging, starting unbounded work, or corrupting retained state.

### Out of Scope

Out-of-scope features should not be added to the pure-Go core unless the
roadmap explicitly moves them into a planned compatibility path.

## Supported HTML Elements

| Element or group | Status | Fallback behavior |
| --- | --- | --- |
| `html`, `head`, `body`, `title`, `meta`, `link`, `style`, `script` | Partial | Unknown metadata is preserved for later phases when practical. |
| `h1`-`h6`, `p`, `br`, `hr`, `pre`, `code`, `blockquote` | Supported | Unknown text-level semantics render as normal flow text. |
| `div`, `span`, `section`, `article`, `header`, `footer`, `main`, `nav`, `aside` | Partial | Unsupported landmarks render as block or inline containers. |
| `ul`, `ol`, `li` | Supported | Unsupported marker styling uses default markers. |
| `a` | Partial | Unsupported protocols are not navigated by the core engine. |
| `img` | Partial | Decode failures render as missing or empty replaced content. |
| `form`, `label`, `input`, `button`, `textarea`, `select`, `option` | Partial | Unsupported controls render as simple controls or inert boxes. |
| `table`, `thead`, `tbody`, `tfoot`, `tr`, `th`, `td`, `caption` | Partial | Unsupported table features use normal flow fallback. Cell spans (colspan/rowspan) are clamped to a max of 100 to prevent DoS. Visual row order follows thead -> tbody -> tfoot. |
| `svg` | Partial | Unsupported SVG content renders as omitted or fallback content. |
| `canvas`, `video`, `audio`, `iframe`, `object`, `embed` | Out of Scope | Render as inert fallback content. |
| Unknown HTML elements | Fallback | Preserve children and render as generic inline or block containers. |

### Runtime detection of unsupported elements

The engine detects the same set of out-of-scope elements (`canvas`,
`video`, `audio`, `iframe`, `object`, `embed`) both during streaming
HTML parse (`dom.ParseConfig.OnUnsupportedFeature`) and at JavaScript
runtime when pages construct them dynamically via
`document.createElement(...)` (`js.Runtime.SetRuntimeUnsupportedFeatureCallback`).

The runtime path is deduplicated per kind and per `Runtime` instance —
each kind is reported at most once. The hook is nil-safe and never
allocates when no callback is installed. Callers wire the hook to the
fallback decision layer (`internal/engine/fallback.Policy.Record`) so
that scripts which dynamically build out-of-scope elements still trigger
the same fallback logic as markup that already contains them.

### Runtime detection of dynamic import()

Static ES module declarations (`<script type="module">`) are detected
during streaming HTML parse and reported as `dom.FeatureESModule`.
Dynamic `import(...)` expressions that appear only inside script
bodies are detected at runtime by `Runtime.ScanAndReportUnsupportedJSFeatures`,
which is auto-invoked from `Runtime.RunScript`. The scanner respects
JS lexical structure (line/block comments, single/double/template-quoted
strings, identifier boundaries) so that mere textual occurrences of
`import` in comments or strings do not produce false positives.

The scanner short-circuits when the script does not contain the
substring `import` (0 B/op, 0 allocs/op, ~2 ns/call when no callback
is installed) and reports at most once per Runtime via the same
deduplicated detection callback as the runtime element creation path.

### Runtime detection of WebSocket, Web Worker, ServiceWorker

The engine installs stub constructors on the JS global object so that
pages calling these unsupported APIs surface their intent to the
fallback layer without crashing the page:

- `new WebSocket(url)` reports `dom.FeatureWebSocket` and returns a
  stub object with no-op `close`, `send`, `addEventListener`, etc.
- `new Worker(url)` reports `dom.FeatureWebWorker` and returns a stub
  object with no-op `postMessage`, `terminate`, `addEventListener`.
- `navigator.serviceWorker.register(url)` reports
  `dom.FeatureServiceWorker` and returns a rejected promise;
  `getRegistration` returns null and `getRegistrations` returns an
  empty array.

Each kind is reported at most once per Runtime via the existing
deduplicated detection callback. The stubs do NOT enforce capability
policy — the detection is the signal we surface, not the access denial.

## Supported CSS

| Feature | Status | Fallback behavior |
| --- | --- | --- |
| Type, class, ID, descendant, child, adjacent sibling, and general sibling selectors | Partial | Invalid selectors are ignored. |
| Attribute selectors | Partial | Unsupported operators or malformed selectors are ignored. |
| Pseudo-classes such as `:first-child`, `:last-child`, and `:nth-child` | Partial | Unsupported pseudo-classes do not match. |
| Pseudo-elements such as `::before` and `::after` | Partial | Unsupported generated content is omitted. |
| Cascade, specificity, and `!important` | Partial | Malformed declarations are ignored. |
| Colors and backgrounds | Partial | Invalid colors fall back to inherited or initial values. |
| Font family, size, weight, and style | Partial | Unavailable fonts use platform defaults. |
| Margin, border, padding, width, height, and box sizing | Partial | Invalid values fall back to initial values. |
| Inline and block layout | Partial | Unsupported display values use block or inline fallback. |
| Flexbox | Partial | Unsupported flex properties use normal flow fallback. |
| Tables | Partial | Unsupported table layout details use normal flow fallback. |
| Media queries | Partial | Level 4 range syntax (`(width <= 600px)`, `(600px >= width)`, chained) is supported; unsupported media features evaluate as non-matching. |
| CSS custom properties and `calc()` | Partial | Invalid substitutions or calculations invalidate the declaration. |
| Animations, transitions, transforms, filters, grid, and advanced typography | Planned | Declarations are ignored until implemented. |

## Supported DOM and Browser APIs

| API | Status | Fallback behavior |
| --- | --- | --- |
| HTML parsing through `golang.org/x/net/html` | Supported | Malformed HTML follows tokenizer tree-construction recovery. |
| Text extraction | Supported | Unknown nodes contribute child text when applicable. |
| `document.getElementById` | Supported | Missing IDs return no element. |
| `getElementsByClassName`, `getElementsByTagName` | Partial | Unsupported node types are skipped. |
| `querySelector`, `querySelectorAll` | Partial | Unsupported selectors return no match or an error. |
| `createElement` and basic element mutation | Partial | Unsupported mutations are rejected or become inert. |
| `appendChild`, `removeChild`, `replaceChild`, `insertBefore` | Partial | Invalid ownership or hierarchy changes fail predictably. |
| `addEventListener`, `removeEventListener` | Partial | Unsupported event types are ignored. |
| `console.log`, `console.error`, `console.warn`, `console.info`, `console.table` | Supported | Values that cannot be formatted use a safe string form. |
| `window.location` and query helpers | Partial | Invalid URLs are rejected. |
| `window.history` | Partial | Unsupported state behavior is ignored. |
| `setTimeout`, `setInterval` | Partial | Timers must be cancelled on document or session cleanup. |
| `fetch` | Partial | Requests must use cancellable contexts and bounded response limits. |
| `localStorage`, `sessionStorage` | Partial | Quota or validation failures return predictable errors. |
| Web Components, Shadow DOM, Service Workers, WebRTC, WebGL, WebGPU, WebAudio, IndexedDB | Out of Scope | APIs are absent unless a future compatibility path adds them. |

## Supported Table Layout Algorithm Subset

Goosie v2 implements a simplified, lightweight table layout algorithm that maps HTML tables onto a CSS Grid structure:

1. **Grid-Based Column Allocation**: Tables are treated as grid containers where each column is assigned an `auto` track sizing.
2. **Cell Spans**: Both `colspan` and `rowspan` are supported up to a maximum clamp limit of 100 to prevent Denial of Service (DoS) and out-of-memory errors.
3. **Visual Ordering**: Section visual ordering is strictly normalized to `thead` (first) -> `tbody` (middle) -> `tfoot` (last), regardless of source code sequence.
4. **Column Measurement Caching**: Column widths are cached per table node ID and available container width using a bounded cache. If the table is not invalidated (e.g. by mutations), cached column sizes are reused on subsequent layout passes.
5. **Flow Fallback**: Unsupported or malformed table structures fall back gracefully to normal inline or block layout behavior.

## Maximum Resource Limits

These v2 limits define the target contract for new engine work. Existing code
may need follow-up changes before every limit is enforced at runtime.

| Resource | Limit | Required behavior when exceeded |
| --- | --- | --- |
| Maximum document size | 16 MiB encoded HTML per main document | Cancel parsing and surface a navigation error. |
| Maximum stylesheet size | 2 MiB encoded CSS per stylesheet, 8 MiB total per document | Ignore the oversized stylesheet and report a recoverable error. |
| Maximum decoded image size | 64 MiB per decoded image, 256 MiB total per document | Treat the image as failed and keep layout stable. |
| Maximum script execution budget | 50 ms per task before cooperative cancellation points | Stop the task and report a script error. |
| Maximum DOM nodes | 250,000 nodes per document | Stop parsing and surface a document-too-large error. |
| Maximum CSS rules | 50,000 rules per document | Ignore additional rules and report a recoverable style error. |
| Maximum concurrent resource requests | 6 per origin, 24 globally | Queue within bounded scheduler capacity or reject lower-priority work. |
| Maximum retained navigation history entries | 500 entries per profile | Evict oldest entries. |
| Maximum local storage per origin | 5 MiB | Reject writes that exceed quota. |

## Content Security Policy (CSP) Subset

Goosie v2 enforces a subset of the Content Security Policy (CSP) Level 3
specification. The CSP header is parsed from HTTP responses and applied to
script loading, stylesheet loading, and fetch requests.

### Supported Directives

| Directive | Description |
| --- | --- |
| `default-src` | Fallback for directives not explicitly set. |
| `script-src` | Controls which scripts may execute. |
| `style-src` | Controls which stylesheets may be loaded. |
| `connect-src` | Controls which URLs may be fetched via XHR/fetch. |
| `base-uri` | Controls which URLs may be used as the document base. |

### Supported Source Expressions

| Source | Example | Behavior |
| --- | --- | --- |
| `'none'` | `script-src 'none'` | Blocks all resources of that type. |
| `'self'` | `script-src 'self'` | Allows resources from the same origin (scheme + host + port). |
| Scheme | `https:` | Allows any resource using that scheme. |
| Host | `example.com` | Allows resources from that exact host (any path). |
| Host with path | `example.com/assets` | Allows resources whose path starts with `/assets`. |
| Wildcard host | `*.example.com` | Allows the base domain and any subdomain. |
| Full URL | `https://cdn.example.com/lib.js` | Allows only that exact URL. |

### Directive Fallback

When a directive is absent, CSP falls back to `default-src`. When both the
directive and `default-src` are absent, no restriction applies for that
resource type.

### Limitations

- `unsafe-inline`, `unsafe-eval`, nonces, hashes, and strict-dynamic are
  not supported and are silently ignored.
- `report-uri` and `report-to` directives are parsed but not acted upon.
- Multiple `Content-Security-Policy` headers are merged in order.

## Unsupported Feature Policy

Unsupported features should fail closed and visibly:

- Parsing errors recover where the HTML or CSS grammar defines recovery.
- Unsupported CSS declarations are ignored without affecting other valid
  declarations.
- Unsupported elements preserve children when that is safe.
- Unsupported browser APIs are absent or return explicit errors.
- Unsupported network, media, or script work must not create unbounded
  goroutines, queues, timers, caches, or buffers.

When a proposed feature is not listed here, contributors should treat it as
out of scope until the roadmap and this support matrix are updated.

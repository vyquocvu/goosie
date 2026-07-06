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
| `table`, `thead`, `tbody`, `tfoot`, `tr`, `th`, `td`, `caption` | Partial | Unsupported table features use normal flow fallback. |
| `svg` | Partial | Unsupported SVG content renders as omitted or fallback content. |
| `canvas`, `video`, `audio`, `iframe`, `object`, `embed` | Out of Scope | Render as inert fallback content. |
| Unknown HTML elements | Fallback | Preserve children and render as generic inline or block containers. |

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
| Media queries | Partial | Unsupported media features evaluate as non-matching. |
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

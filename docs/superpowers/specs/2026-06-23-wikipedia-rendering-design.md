# Goosie: Full HTML/JS/CSS Rendering (Wikipedia Target)

**Date:** 2026-06-23
**Goal:** Make Goosie render https://en.wikipedia.org/wiki/Main_Page like a regular browser — full HTML structure, all linked CSS applied, JavaScript interactive features working, high visual fidelity.

## Non-Goals

- Pixel-perfect Chromium parity
- CSS animations and transitions (deferred)
- WebAssembly, Canvas API, WebGL
- Service Workers, Web Workers
- Browser extensions
- Multi-process isolation
- Replacing Goja with a different JS engine (use polyfills instead)

---

## Section 1: External Resource Pipeline

### Problem

Goosie currently fetches HTML then renders immediately. External stylesheets (`<link rel="stylesheet">`), external scripts (`<script src>`), and linked images are not fetched or applied. Wikipedia's entire visual style is delivered via `load.php?modules=...` CSS bundles referenced in `<head>` — without these, no CSS applies at all.

### Design

New package: `internal/loader`

**`PageLoader`**
- Orchestrates a full page load: HTML fetch → resource discovery → parallel asset fetch → render-ready signal
- Implements render-blocking semantics: CSS in `<head>` blocks first render; `async`/`defer` scripts do not
- Fires a render-ready callback once all render-blocking resources are fetched and applied
- After first render, continues fetching non-blocking resources (images, async scripts) and triggers targeted re-renders

**`ResourceQueue`**
- Tracks pending / loaded / failed assets
- Categorizes: render-blocking CSS, render-blocking scripts, async scripts, images, fonts
- Notifies `PageLoader` when render-blocking set is complete

**`AssetCache`**
- In-memory cache keyed by normalized URL
- Respects `Cache-Control: max-age` and `Expires` headers within a session
- Shared across tabs in the same browser instance
- Flushed on browser restart (no disk persistence in this milestone)

**Resource discovery pass**
After HTML parse, walk the node tree collecting:
- `<link rel="stylesheet" href="...">` — render-blocking CSS
- `<script src="...">` without `async`/`defer` — render-blocking JS
- `<script src="..." async>` / `<script src="..." defer>` — non-blocking JS
- `<img src="...">` / `<img srcset="...">` — lazy-loaded images
- `@import` rules inside already-fetched CSS — recursive CSS fetch

**CSS application order**
Fetched stylesheets are applied to the CSS engine in document order (same as a browser). Each sheet is parsed and its rules merged into the active stylesheet cascade.

**JS execution order**
Render-blocking scripts execute synchronously (in Goja) before first render. Deferred scripts execute after DOM is ready. Async scripts execute when loaded.

---

## Section 2: CSS Completeness

### CSS Custom Properties (Variables)

Wikipedia uses `var(--color-base)`, `var(--font-size-medium, 14px)` etc. extensively.

- Variables cascade and inherit like regular properties
- Resolution happens during computed style calculation, after the cascade
- Syntax: `--name: value` declared on any element; `var(--name, fallback)` in any value position
- Circular references → treat as invalid (use fallback or initial value)
- Variables declared on `:root` are globally accessible

Implementation: add a `CustomProperties` map to the computed style struct; resolve `var()` tokens during value computation before layout.

### Positioning System

`position: relative | absolute | fixed | sticky`

- **relative**: element stays in normal flow; `top/right/bottom/left` offsets it visually from its natural position
- **absolute**: removed from normal flow; positioned relative to nearest ancestor with `position != static`; requires a "containing block" tracking pass in the layout tree
- **fixed**: like absolute but containing block is the viewport; stays anchored during scroll
- **sticky**: in-flow until scroll threshold, then fixed-like within its scroll container

Implementation:
1. After normal flow layout, collect all positioned elements
2. For absolute/fixed: resolve containing block, compute offsets, place element
3. For fixed: store separately from scroll-transformed layer; paint after scroll transform

### Float Layout

`float: left | right | clear: left | right | both`

- Floated elements are removed from normal flow; text and inline content wraps around them
- Inline layout engine must track "exclusion zones" per line box — horizontal bands where floats occupy space
- `clear` property skips past all preceding floats of the specified direction

Implementation: extend `InlineLayoutEngine` with a float exclusion list. For each line box, compute available width by subtracting active float extents at the current Y position. Floats accumulate until their bottom edge is passed.

### CSS `calc()`, `min()`, `max()`, `clamp()`

Wikipedia uses `calc(100% - 160px)` for column widths.

- Implement a simple expression evaluator in the CSS value parser
- Supported units: `px`, `%`, `em`, `rem`, `vw`, `vh`
- `%` resolves against the containing block dimension at layout time (deferred evaluation)
- `min(a, b)`, `max(a, b)`, `clamp(min, val, max)` — evaluate after resolving units

### z-index and Stacking Contexts

- A stacking context is created by: `position != static` with `z-index != auto`, `opacity < 1`, `transform`, `filter`
- Within a stacking context, paint order: background → negative z-index children → block flow → floats → inline → non-negative z-index positioned children
- The display list must be sorted by stacking context and z-index before painting

Implementation: build a layered display list during the paint phase. Each stacking context is a node in a tree; children are sorted by z-index then DOM order.

### `overflow: hidden | scroll | auto`

- `hidden`: clip content to the element's border box; establishes a block formatting context (clears floats)
- `scroll` / `auto`: add a scroll container; content overflows into a scrollable region
- `overflow: hidden` is the most critical for Wikipedia (used on floated info-boxes and columns)

### Pseudo-elements `::before` / `::after`

- Generate anonymous inline/block boxes from CSS `content:` property
- Content values: `""` (empty), string literals, `counter()`, `attr()`
- Insert as first/last child in the layout tree of the originating element
- Already partially implemented — complete the layout tree insertion and content resolution

### Text Properties

- `line-height`: unitless multiplier, px, or `%` of font-size
- `letter-spacing`, `word-spacing`: applied during inline text measurement
- `text-overflow: ellipsis`: clips text and appends `…` when inline content overflows a `nowrap` container
- `word-break: break-all | break-word`: controls line-break points in inline layout
- `text-decoration`: underline, overline, line-through (Wikipedia uses underline for links)
- `text-transform`: uppercase, lowercase, capitalize

---

## Section 3: JavaScript Polyfills & DOM API Completeness

### Polyfill Injection (`internal/js/polyfills.go`)

Polyfills are injected as a single JS string evaluated before any page script. They target Goja's gaps:

**Promise**
- Full `Promise` constructor with executor
- `.then()`, `.catch()`, `.finally()`
- `Promise.resolve()`, `Promise.reject()`, `Promise.all()`, `Promise.allSettled()`, `Promise.race()`, `Promise.any()`
- Microtask queue via `queueMicrotask` (implemented as a Go channel-drained loop after each JS call)

**Collections**
- `Map`, `Set` — with iteration protocol (`Symbol.iterator`, `for...of` via transpilation)
- `WeakMap`, `WeakSet` — basic implementation (GC semantics approximated)

**Symbol**
- `Symbol(description)`, `Symbol.iterator`, `Symbol.toPrimitive`
- Enough for iterator protocol support

**Object methods**
- `Object.assign`, `Object.entries`, `Object.fromEntries`, `Object.values`, `Object.is`, `Object.create` (if missing)

**Array methods**
- `Array.from`, `Array.of`
- `Array.prototype.find`, `findIndex`, `includes`, `flat`, `flatMap`, `at`

**String methods**
- `startsWith`, `endsWith`, `includes`, `padStart`, `padEnd`, `trimStart`, `trimEnd`, `repeat`, `matchAll`

**Miscellaneous**
- `queueMicrotask`
- `structuredClone` (shallow fallback via JSON round-trip)
- `globalThis`

### DOM API Extensions (`internal/js/runtime.go`)

**Element.classList**
- `add(...classes)`, `remove(...classes)`, `toggle(class, force?)`, `contains(class)`, `replace(old, new)`, `item(n)`, `length`
- Backed by the element's `class` attribute; mutations sync back to the DOM node

**Element.dataset**
- Read/write access to `data-*` attributes via camelCase keys
- `element.dataset.fooBar` → `data-foo-bar` attribute

**Element.style**
- `setProperty(name, value, priority?)`, `getPropertyValue(name)`, `removeProperty(name)`
- `element.style.color = "red"` inline style mutation
- CSS variable read/write via `setProperty("--name", value)`

**window.getComputedStyle(element)**
- Returns resolved style values after cascade + inheritance + layout
- Read-only; re-computed on each call

**Element geometry**
- `getBoundingClientRect()` → `{top, right, bottom, left, width, height, x, y}`
- `offsetWidth`, `offsetHeight`, `offsetTop`, `offsetLeft`
- `scrollWidth`, `scrollHeight`, `scrollTop`, `scrollLeft`
- `clientWidth`, `clientHeight`

**Window geometry**
- `window.innerWidth`, `window.innerHeight`
- `window.scrollX`, `window.scrollY` (aliases: `pageXOffset`, `pageYOffset`)
- `window.scrollTo(x, y)`, `window.scrollBy(dx, dy)`

**Document state**
- `document.readyState` — `"loading"` → `"interactive"` → `"complete"`
- `document.cookie` — read/write (session-only jar)
- `document.title` — read/write
- `document.head`, `document.body`
- `document.createTextNode(text)`
- `document.createDocumentFragment()`
- `document.createEvent(type)` + `element.dispatchEvent(event)`

**Element traversal**
- `element.closest(selector)` — walks ancestors, returns first match
- `element.matches(selector)` — tests element against CSS selector
- `element.children` (element-only child list, as opposed to `childNodes`)
- `element.previousElementSibling`, `nextElementSibling`
- `element.innerHTML` (read/write — parse HTML fragment on write)
- `element.outerHTML` (read)
- `element.insertAdjacentHTML(position, html)`
- `element.insertAdjacentElement(position, element)`

**Events**
- `CustomEvent(type, {detail})` constructor
- `element.dispatchEvent(event)`
- `window` as event target (resize, scroll events)
- `document` DOMContentLoaded, load events
- Mouse events: `click`, `mouseenter`, `mouseleave`, `mouseover`, `mouseout`, `mousedown`, `mouseup`
- Keyboard events: `keydown`, `keyup`, `keypress`
- Focus events: `focus`, `blur`
- `event.preventDefault()`, `event.stopPropagation()`, `event.stopImmediatePropagation()`
- `event.target`, `event.currentTarget`, `event.bubbles`, `event.cancelable`

**Observers**
- `MutationObserver` — watch for DOM mutations (node insertion/removal, attribute changes, text content changes)
- `IntersectionObserver` — stub that fires callback immediately with `isIntersecting: true` for all targets (good enough for lazy-load triggers)
- `ResizeObserver` — stub that fires once with current dimensions

**Animation**
- `requestAnimationFrame(callback)` — schedules callback before next Fyne frame; returns integer ID
- `cancelAnimationFrame(id)`

**History**
- `history.pushState(state, title, url)`, `history.replaceState(state, title, url)`
- `window.onpopstate` / `popstate` event fires on back/forward navigation

---

## Section 4: Rendering Fidelity

### SVG Rendering

Wikipedia uses SVG for icons (nav icons, logo), mathematical notation, and some diagrams.

- Add dependency: `github.com/srwiley/oksvg` + `github.com/srwiley/rasterx` (pure Go, no CGo)
- `SvgRenderer` in `internal/renderer/svg.go`: accepts SVG bytes, rasterizes to `image.RGBA` at target dimensions
- `<img src="*.svg">` and inline `<svg>` elements both route through `SvgRenderer`
- Inline SVG: serialize the SVG node subtree to bytes, pass to rasterizer
- Cache rasterized images by URL + dimensions to avoid re-rasterizing on scroll

### Stacking Context Paint Order

After layout, the display list is rebuilt as a stacking context tree:

1. Walk layout tree; create a new stacking context node for each element that establishes one
2. Within each context, sort children: negative z-index → block/float → inline → non-negative positioned
3. Flatten the tree into a paint-ordered list
4. Fyne canvas objects are added in paint order

### Font & Text Rendering

- `line-height` multiplier applied to font size during line box height calculation
- `letter-spacing` added to each glyph advance in text measurement
- `text-decoration: underline` rendered as a rectangle below the text baseline
- `text-transform` applied to text content before measurement and display
- `font-family` preference list: attempt to match against system fonts; fall back to Fyne default
- `text-overflow: ellipsis`: during inline layout, if a word-unwrapped line overflows, truncate and append `…`

### Image Pipeline

- Asynchronous: images fetched after first render; placeholder shown while loading
- Supported formats: PNG, JPEG, GIF (first frame), WebP, SVG
- `srcset` parsing: pick highest-resolution source that fits the display pixel density
- Lazy loading: images below the viewport are deferred until scroll brings them near
- Re-render scope: only the element's bounding box is invalidated when an image loads, not the full page

### Fixed-Position Elements

- During layout, `position: fixed` elements are collected into a separate fixed-element list
- The scroll canvas applies a translation transform to scrollable content
- Fixed elements are painted on top of the scrolled canvas, outside the transform, using viewport coordinates
- On scroll, only the scrollable layer is re-translated; fixed elements repaint in place

---

## Implementation Order

1. **Resource pipeline** (`internal/loader`) — unblocks all CSS and JS from loading; nothing else matters without this
2. **CSS variables** — cheap, high impact; Wikipedia's color scheme depends on them
3. **Float layout** — Wikipedia info-boxes and images use floats
4. **Positioning** (relative → absolute → fixed) — header, nav menus, overlays
5. **calc() / min() / max()** — column width math
6. **z-index / stacking contexts** — menus rendering above content
7. **overflow: hidden** — float clearing, clipping
8. **JS polyfills** — Promise, Map/Set, Array/String/Object methods
9. **DOM API extensions** — classList, dataset, style mutation, geometry APIs
10. **Observers** — MutationObserver, IntersectionObserver stubs
11. **SVG rendering** — icons and logo
12. **Text rendering completeness** — line-height, letter-spacing, text-decoration
13. **Fixed-position scroll** — sticky header stays in place
14. **Stacking context paint order** — menus visible above content
15. **Image pipeline** — srcset, lazy loading, async re-render

---

## Testing Strategy

- **Unit tests per feature**: each CSS property, each polyfill, each DOM API method
- **Integration test**: load a saved snapshot of Wikipedia's HTML+CSS (no network required) and compare rendered output against a reference screenshot
- **Live test**: `test/e2e/online_pages_test.go` — load `https://en.wikipedia.org/wiki/Main_Page`, assert no panics, key elements present in display list
- **Regression baseline**: screenshot before/after each phase to catch visual regressions

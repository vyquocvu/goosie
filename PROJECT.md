# Project: Goosie Browser Engine Live URL Hardening & Subsystem Fixes

## Architecture
Goosie is a modular browser engine in Go:
- `cmd/browser`: Desktop GUI browser utilizing Fyne UI, `documentloader.Coordinator`, `renderer.Renderer`, and `js.Session`/`js.Runtime`.
- `cmd/headless`: CLI headless renderer rendering HTML to PNG via `renderer.RenderHTMLToImage`.
- `cmd/perf-review`: Benchmarking CLI driver backed by `browsercontrol.NewEngineService()`.
- `internal/net`: Network fetching, HTTP streaming, cookie management, CSP security.
- `internal/dom`: HTML5 tokenization, tree construction, DOM node hierarchy, query selector matching.
- `internal/css`: CSS3 lexer, parser, property atom interning, specificity calculation, bucketed rule compilation, media query evaluation, custom property (`var()`) resolution.
- `internal/renderer`: Render tree construction, style resolution & cascade, layout engine (block formatting, inline formatting, table grid, flexbox), display list generation, raster backend.
- `internal/renderer/layoutgolden`: Golden layout test harness and testdata.
- `internal/js`: Goja-based ECMAScript runtime, DOM bridge (`Node`, `Element`, `Document`, `Event`, `CustomEvent`), polyfills, frame scheduler, session concurrency.
- `internal/browsercontrol`: In-process headless browser automation service implementing navigation, screenshot, DOM snapshot, script evaluation.

## Feature Inventory
| # | Feature | Description | Milestone | Status |
|---|---------|-------------|-----------|--------|
| 1 | Window EventTarget | Implement `addEventListener`, `removeEventListener`, `dispatchEvent` on `window` and `globalThis` | M1 (DOM/JS) | DONE |
| 2 | Window Global Scope Sync | Synchronize properties assigned to `window` with global scope for third-party libraries | M1 (DOM/JS) | DONE |
| 3 | Element Attributes & Iterator | Implement `Element.hasAttributes()`, iterable `Element.attributes` (`NamedNodeMap`) with `{name, value}` | M1 (DOM/JS) | DONE |
| 4 | Event Context Properties | Populate `event.target = this` and `event.currentTarget = this` in event dispatch | M1 (DOM/JS) | DONE |
| 5 | Media & Document API Stubs | Add `HTMLMediaElement` stubs (`play()`, `pause()`, `load()`), `document.location` alias, timer string eval | M1 (DOM/JS) | DONE |
| 6 | Script MIME Type Filter | Ignore non-executable script types (e.g. `application/ld+json`, `text/template`) | M1 (DOM/JS) | DONE |
| 7 | DOM Selector Expansion | Enhance `Element.matches`, `closest`, and `matchesSelector` with combinators and attribute operators | M1 (DOM/JS) | DONE |
| 8 | Browsercontrol Navigation Sync | Synchronize parsed HTML into `jsRuntime.SetHTMLContent`/`LoadHTML` during `Navigate` | M1 (DOM/JS) | DONE |
| 9 | Text Overlap Coalescing Fix | Fix inline text coalescing in `internal/renderer/display_list.go` to use contiguous run-length grouping | M2 (CSS/Renderer) | DONE |
| 10 | CSS Specificity & `!important` | Sort matched rules by specificity/source order and respect `!important` in `style.go` | M2 (CSS/Renderer) | DONE |
| 11 | CSS Shorthands & UA Styles | Add `font:` and `list-style:` shorthand handlers in `style.go`, update default UA link styles | M2 (CSS/Renderer) | DONE |
| 12 | CSS Attribute Selector Bucketing | Dynamically support all attribute names in `collectCandidates` in `internal/css/selector.go` | M2 (CSS/Renderer) | DONE |
| 13 | Table Layout Column Sizing | Implement proportional content-based column sizing in table grid layout | M2 (CSS/Renderer) | DONE |
| 14 | Headless External CSS Resolution | Resolve and pre-fetch external `<link rel="stylesheet">` during headless screenshot capture | M2 (CSS/Renderer) | DONE |
| 15 | Live URL E2E Test Suite | Automated test harness testing all 10 live URLs with navigation, title, screenshot, and evaluation | E2E Track / M3 | DONE |
| 16 | Zero-Regression Conformance | 100% pass across `go vet ./...`, `go test ./...`, `layoutgolden`, and `perf-review` | M3 (Verification) | DONE |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| E2E | E2E Testing Track | Live URL automated test runner & test suites for 10 websites (Tiers 1-4) | None | DONE |
| M1 | DOM & JavaScript Engine Fixes | `internal/dom`, `internal/js`, `internal/browsercontrol` (Features 1-8) | None | DONE |
| M2 | CSS & Layout Engine Fixes | `internal/css`, `internal/renderer`, `internal/renderer/layoutgolden` (Features 9-14) | None | DONE |
| M3 | Full Integration & Verification | All 10 live URLs pass, 0 regressions, golden tests pass, perf-review passes | E2E, M1, M2 | DONE |

## Code Layout
- `cmd/browser/`: Desktop browser application
- `cmd/headless/`: CLI headless renderer
- `cmd/perf-review/`: Performance benchmarking CLI
- `internal/browsercontrol/`: Browser automation service
- `internal/dom/`: DOM parser, node tree, query selector matching
- `internal/css/`: CSS parser, selector compiler, cascade, properties
- `internal/renderer/`: Render tree, style application, layout, display list, raster
- `internal/renderer/layoutgolden/`: Golden layout test harness and testdata
- `internal/js/`: ECMAScript runtime, DOM bindings, polyfills, frame scheduler
- `test/e2e/`: E2E tests, Playwright visual comparison, live website tests

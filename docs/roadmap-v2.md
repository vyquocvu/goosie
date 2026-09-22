# Goosie v2 — Production Readiness Review and Delivery Roadmap

- Updated: 2026-09-23
- Reviewed baseline: `feat/v2` at `ba66721`
- Status: proposed; this review does not authorize implementation or release

## 1. Recommendation and product scope

Keep Goosie's custom Go engine, CPU tile renderer, and native macOS integration. It is an experimental static-document browser, not a production-ready general-purpose browser. Recent HTML/CSS/resource and tab work is real progress, but existing package tests do not establish safe untrusted-content handling or working browser journeys.

The first implementation deliverable should close the verified viewport, document-geometry, tile-cache, and decoded-raster-image allocation gaps. Fix independent tab navigation next. Do not add JavaScript or market a public beta before designing the associated security boundaries.

### Architecture choices

| Approach | Benefit | Trade-off | Recommendation |
| --- | --- | --- | --- |
| Continue the custom Go engine | Preserves engine ownership, current rendering architecture, and headless determinism | Browser compatibility and security remain substantial ongoing engineering work | Recommended for this project |
| Go application around a system WebView | Reuses a mature platform engine | Not a Go-implemented engine; platform behavior and distribution differ | Consider only if shipping a browser product matters more than owning its engine |
| Go application embedding an established cross-platform engine | Much broader existing web compatibility | Native integration, binary size, engine update obligations, and a different architecture | Separate strategic decision, not an incremental feature |

No engine substitution, new dependency, or architecture-rule change is authorized by this roadmap.

### Production means user workflows, not a feature count

Proposed first supported product: a native macOS ARM64 browser plus a separately described headless renderer. Other architectures/platforms must earn their own artifact and native acceptance. Linux/Windows binaries currently do not imply Linux/Windows browser windows.

| User need | Required production behavior |
| --- | --- |
| Read and navigate | Enter a URL, follow links/anchors, use Back/Forward/reload/stop, resize, zoom, and find/copy text without losing state |
| Multitask | Independent tabs, usable tab labels/overflow, background completion, reopening/restore, predictable close behavior |
| Use supported websites | A versioned HTML/CSS/JS/Web-API matrix and named, executable static and dynamic workflows; forms and cookie-based login where advertised |
| Keep browsing state | Versioned local profiles, bookmarks/history/session recovery, clear-site-data controls, and explicit retention behavior |
| Handle files safely | Controlled local-file access, explicit download destination/overwrite behavior, cancellation, and no automatic execution |
| Understand security | Correct URL/origin display, certificate failures, mixed-content policy, permission prompts where needed, and safe rejection of unsupported content |
| Control privacy | Defined normal/private session behavior, no unconsented telemetry, diagnostic redaction, and reliable data deletion |
| Access the application | Keyboard-only use, Unicode/IME input, native accessibility, and a tested screen-reader journey |
| Install and stay protected | Signed/notarized application, tested upgrades/profile migration, authenticated security updates, rollback/recovery, and a support policy |

Accounts, cloud sync, billing, and a hosted service are not requirements for this local-first product. Adding them later requires a separate data-handling and business analysis. Password management, extensions, advanced media, WebRTC, and GPU APIs are deferred unless a selected target workflow requires them.

The exact website/workflow matrix and support ownership must be approved before a public release claim. Existing fixture/corpus results are useful evidence, not a substitute for that product decision.

## 2. Fresh baseline and review limitations

The current tree has a real DOM/style/layout/paint pipeline, not just the synthetic renderer described in old documentation. `Session.NewSession` performs bounded HTML parsing, stylesheet collection, style resolution, image/font loading and layout (`internal/engine/engine.go:143-175`). Cascade precedence exists (`internal/style/style.go:679-683`), along with flex, Grid and table implementations (`internal/layout/block.go`, `internal/layout/grid.go:63`, `internal/layout/table.go:35`). These are implemented subsets, not evidence of complete CSS conformance; they should not be planned again as entirely missing features.

Environment: Go `1.26.5`, Darwin ARM64, native/cgo-enabled default build. Checks were run against the reviewed source before documentation edits.

| Check | Result |
| --- | --- |
| `go test -count=1 ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |
| `go build ./...` | PASS |
| `go run ./cmd/goosie -backend headless -gate -frames 3 -out -` | PASS; synthetic startup/render/shutdown smoke only |
| `go vet -all ./...` | FAIL: five unkeyed `css.Color` literals in tests |
| `CGO_ENABLED=0 go build ./...` on Darwin ARM64 | FAIL: missing Darwin stub symbols/interface methods |

The vet locations are `test/style/gradient_test.go:113`, `test/style/gradient_test.go:158`, `test/style/presentational_hint_test.go:14`, and `test/style/presentational_hint_test.go:23`; line 113 contains two literals. The no-cgo compiler errors reference `internal/platform/platform_darwin.go:21`, `:31`, and `:48`.

Both test commands report **no test files for `cmd/goosie` and `cmd/goosie-headless`**. Therefore passing race detection does not contradict the application ownership defects below: those interleavings are not exercised by the existing tests.

This assessment did not run native interactive journeys, a new pixel-parity/live-site sweep, sanitizers, a penetration test, or dependency advisory scanning. `govulncheck` was not installed. No cached visual score is presented as a fresh result. The synthetic smoke timings are not a performance benchmark.

CodeGraph was consulted first but returned obsolete v1 paths such as `cmd/browser` and `internal/js`; findings below use the current tree instead. The index was not changed. Existing README/PROJECT architecture claims and older roadmap checkboxes are not current acceptance evidence.

## 3. Evidence-backed review findings

Severity here sets release priority. Source-proven defects are distinguished from reproduced command failures and unverified exploitability; no remote exploit was attempted.

### R1 — Critical: real-document entrypoints bypass checked geometry/cache limits

Both `cmd/goosie/main.go:582-589` and `cmd/goosie-headless/main.go:118-130` call `Session.Paint` and derive pool/cache size from the entire document tile count. `internal/engine/engine.go:445-465` provides `PaintChecked` and explicitly requires it for untrusted input. `internal/engine/limits.go:74-115` already provides viewport/extent validation, with `MaxDocumentTiles = 65536` and `MaxTileCacheBytes = 64 MiB` at `:54-55`.

The existing extent check also undercounts negative nonaligned tile spans: `(-1,-1,65535,65535)` covers 257×257 signed tile coordinates, not 256×256. Use `frame.CoordFor`'s floor division (`internal/frame/tile.go:23-30`) when counting. The headless executable additionally performs an unrestricted file read and preliminary parse before entering the bounded engine parser (`cmd/goosie-headless/main.go:97-103`).

Correct both application entrypoints rather than merely adding another isolated engine test. Preserve a bounded working-set cache for long documents; do not make the cache proportional to document area. Admit the headless input before parsing it.

### R2 — Critical: compressed response size does not bound decoded images

`internal/image/decode.go:16-32` fully decodes PNG/JPEG/GIF without dimension/pixel preflight. `internal/engine/engine.go:330-367` allows concurrent image work and retains decoded images. A small encoded image can have a very large decoded footprint; a per-response network cap does not prevent this.

Add overflow-safe metadata validation before decoding, a per-image pixel limit, and a per-document reservation budget covering image/background-image work already in flight and retained results. Limits must not depend on successful completion counts. These bounds do not, by themselves, prove a total-process RSS or CPU-time bound.

### R3 — High: tabs are not independently navigable

`cmd/goosie/main.go:199-201` stores cancellation/generation at window scope. Navigation in one tab can cancel another (`:322-330`), and switching a populated tab invalidates outstanding navigation (`:465-470`). `newTab` displays a new layer without selecting its tab (`:479-503`), while `internal/tabs/manager.go:21-31` only activates the first tab.

A new tab can display one document while the active ID, toolbar, and next navigation still belong to another. Use per-tab navigation identity/cancellation, a separate presentation serial, and explicit successful-state installation.

### R4 — High: mutable UI state has multiple owners

The event pump mutates chrome state while `Present` reads it (`cmd/goosie/toolbar_window.go:34-43,63-69`). Navigation goroutines mutate tab/history/toolbar state (`cmd/goosie/main.go:345-365,419-439`). Scheduler viewport operations are not cross-thread APIs (`internal/raster/scheduler.go:23-27,195-206`). Collection locking in `TabManager` does not protect mutable fields on returned tabs.

`syncTabToToolbar` also replaces `Input` without resetting cursor/selection (`cmd/goosie/main.go:307-311`); `internal/toolbar/input.go:20-35` slices using the stale cursor, creating a panic path when switching to a shorter URL and deleting. These are source-proven unsafe paths, not reproduced GUI crashes in this assessment.

### R5 — High: chrome and document presentation are not separated

`cmd/goosie/toolbar_window.go:38-43` paints chrome directly over the composed document buffer. The scroll fast path shifts retained backing pixels (`internal/raster/scheduler.go:439-448`), so chrome can contaminate subsequent document output. There is no reserved content viewport corresponding to the 76-device-pixel overlay.

Reserve content coordinates, separate chrome from reusable page pixels, and verify small bidirectional scrolls against full redraw. Resize currently adjusts toolbar bounds only (`cmd/goosie/toolbar_window.go:108-110`); the document must retain a session for responsive reflow. Tab labels are currently rectangles rather than readable glyphs (`internal/tabs/draw.go:74-87`).

### R6 — High: navigation commitment and teardown are incomplete

Back/Forward changes the history cursor before success (`cmd/goosie/main.go:371-386`); a failed traversal leaves it inconsistent with the displayed document. New navigation does not explicitly reset scroll, and closing a background tab restores potentially stale active-tab scroll (`:355-366,506-524`).

Requests use background-rooted contexts (`:326`), tab close does not cancel/join their work, and the chrome pump can block sending after consumption stops (`cmd/goosie/toolbar_window.go:63-70`). `net.HTTP.Close` closes idle connections, not application tasks (`internal/net/http.go:358-360`). Define transaction-like URL/history/document commitment and cancel/join ordering through shutdown.

### R7 — High: browser security requires more than standard TLS

Existing protections should be retained: standard certificate verification, validated HTTP(S) redirects, an 8 MiB decoded-response cap, and context-aware requests (`internal/net/http.go:268-389`); subresource scheme rejection exists (`internal/engine/limits.go:571-584`).

Missing or incomplete boundaries include mixed-content/HSTS/CSP/origin policy, font CORS and private-network request policy. Redirect validation permits HTTPS-to-HTTP transitions (`internal/net/http.go:273-281`); subresource checks primarily authorize schemes, not origin/network destination. This allows remote markup to trigger local/intranet GETs, but does not establish arbitrary local-file reading or script-based exfiltration.

CSS import expansion precedes aggregate validation (`internal/engine/imports.go:18-63`, `internal/engine/limits.go:459-484`); fonts are loaded sequentially without an aggregate request budget (`internal/engine/engine.go:187-215`). A launch-time image deadline does not cancel all in-flight work (`:333-367`). Network, parser, font, image, and script budgets need a coherent document/process lifecycle.

### R8 — High: no untrusted-content containment; native contracts need hardening

The application processes document content and native presentation in the same process. `internal/platform/darwin/window.go:230-255` does not validate backing length against stride/height before passing a pointer to C. Same-sized native surfaces are reused without checking a changed stride/capacity (`internal/platform/darwin/shim_darwin.m:721-727,774-777`). Native exploitability through ordinary document rendering was **not** established.

The native clipboard synchronously dispatches to the main queue even when called there (`internal/platform/darwin/shim_darwin.m:825-851`), and native shutdown synchronization/lifetime also requires review. Fix these contracts, test them with native tooling, and establish a constrained content-process boundary with brokered host access. Go memory safety does not supply browser isolation.

### R9 — Product gap: rendering is not interactive web compatibility

The main loader creates an engine session and retains only a painted layer (`cmd/goosie/main.go:573-591`). The scheduler handles scroll/geometry rather than page event dispatch (`internal/raster/scheduler.go:246-317`). There is no current JavaScript runtime package in the v2 tree.

Links, forms, selection, a DOM event loop, persistent document mutation/reflow, accessibility, and dynamic application APIs need end-to-end integration. Cookies/profile storage are not implemented by the current HTTP client (`internal/net/http.go:270-288`), and headers are flattened (`:342-345`), which must change before correct multiple-`Set-Cookie` handling. Do not label visual fixtures or static corpus percentages as web-application compatibility.

### R10 — Release blockers: current automation can report misleading success

The release smoke step sets executable permissions and prints PASS without running the binary (`.github/workflows/release.yml:49-56`). Releases package archives, not an accepted signed/notarized/updateable macOS application (`:31-59`). Standard PR checks run on Ubuntu (`.github/workflows/ci.yml:13-59`); nightly native coverage exercises a synthetic scene (`.github/workflows/nightly-bench.yml:39-57`). The "Security Audit" runs vet/race checks but no advisory scanner (`.github/workflows/security.yml:24-27`).

The reproduced vet/no-cgo failures must be fixed without weakening checks. Validate actual extracted artifacts and native workflows before advertising a target. Keep publishing and credential/configuration changes subject to explicit authorization.

### R11 — High: visual scoring is not currently a fail-closed acceptance gate

`testdata/parity.py:30-35` ignores renderer exit failures, `:43-44` skips missing image pairs, `:47-49` resizes mismatched reference dimensions, and `:99-109` returns zero even when fixture scores fail. Its published scoring rule is 90% matching pixels with a per-channel tolerance of 30 (`:18-19`), not exact equivalence or standards compliance.

Before using it as a release gate, require complete expected outputs, successful current renders, exact dimensions, artifact freshness and nonzero failure exits. Preserve fixtures/reference pixels and existing thresholds; stronger evidence must not be created by dropping failures. CI currently runs the Go suites but does not invoke this Python harness (`.github/workflows/ci.yml:34-47`).

The separate headless executable stops after two presentations (`cmd/goosie-headless/main.go:161-174`), not verified resource/tile settlement. Its output readiness also needs a specific regression gate; a valid PNG is not necessarily a complete render.

## 4. One next implementation deliverable

**Outcome:** Both real-document entrypoints enforce declared viewport/document/tile-cache limits, and raster image decoding cannot exceed declared per-image/per-document pixel reservations before publication.

Implementation plan: [Guarded document rendering](superpowers/plans/2026-09-23-guarded-document-rendering.md).

Acceptance must include the actual `cmd/goosie` URL loader and the headless executable, not only direct engine calls. Use deterministic local inputs, cheap invalid metadata, small injectable accounting limits, and bounded subprocesses rather than deliberately allocating enormous images in the test runner. Supported small pages must still render correctly.

This delivers specific allocation safeguards, **not** a general secure-browser claim, a total-process memory guarantee, safe JavaScript execution, correct tab interaction, or production-release approval. Aggregate font/CSS traffic, decoder CPU containment, accumulated tab memory, native boundaries, and sandboxing remain explicit work below.

## 5. Ordered production gates

These are dependency gates for separate future specs/plans, not tasks to implement together. Only the first deliverable above is scoped for execution planning now.

| Order | Deliverable boundary | Acceptance evidence |
| --- | --- | --- |
| 1 | Guarded real-document rendering | Actual entrypoint allocation/rejection tests; image reservations; bounded cache; unchanged valid render output |
| 2 | Independent, race-free tab/document lifecycle | One state owner; per-tab cancellation/generation; successful URL/history commitment; redirects; scroll; close/join; deterministic out-of-order integration tests under `-race` |
| 3 | Usable, responsive browser surface | Separate chrome/content pixels; retained sessions and resize/DPR reflow; readable titles/overflow; URL/security/error display; links/anchors; native and headless visual/input evidence |
| 4 | Safe resource access and content containment | Aggregate request/byte/time limits, local-file and network-origin policy, validated native/IPC boundaries, restricted content processes, cancellation/crash recovery, adversarial tests |
| 5 | Interactive and persistent browsing | Focus/forms/selection/find/zoom, Unicode/IME/accessibility; downloads; profiles/bookmarks/history/session restore; cookie/storage policy, clearing/private-mode tests |
| 6 | Supported dynamic applications | Selected JS runtime plus DOM bindings, event/tasks/microtasks, script ordering, mutation/invalidation, Fetch and required APIs; cross-origin/CSP/storage tests and interruptibility |
| 7 | Qualified public beta/release candidate | Required feature/WPT subsets, pinned visual fixtures, supported-site workflows, fuzzing, dependency scans, native sanitizer results, repeated lifecycle/load/crash tests and privacy review |
| 8 | Supported production distribution | Signed/notarized artifacts, extracted-artifact execution, clean-machine install/upgrade/migration/recovery, authenticated updates, provenance/licenses, security ownership and release sign-off |

Security design begins immediately; gate 4 is not permission to add unsafe capabilities in gates 1–3. Cookie/origin/storage policy must accompany any feature using it. Process containment must precede exposing untrusted dynamic execution. Packaging/native CI can be developed in parallel under their own approved scope, but cannot bypass runtime release gates.

A static-document alpha is an intermediate engineering release. A production claim must name supported workflows and must remain safe when unsupported pages are opened.

## 6. Deferred from the first implementation plan

| Deferred item | Current evidence | Why separate |
| --- | --- | --- |
| Tab ownership, navigation and transactional history | `cmd/goosie/main.go:199-201,315-525`; `internal/tabs/manager.go:21-31` | Independent application state-machine deliverable; next after allocation safeguards |
| Chrome/content separation, real labels, overflow and reflow | `cmd/goosie/toolbar_window.go:38-43,108-110`; `internal/tabs/draw.go:74-87` | Presentation/input lifecycle, not allocation preflight |
| Aggregate fetch/font budgets and cancellation | `internal/engine/imports.go:18-63`; `internal/engine/engine.go:187-215,333-367` | Requires a shared resource-loader lifecycle and policy |
| Native buffer/thread/lifetime hardening and sandbox | `internal/platform/darwin/window.go:230-255`; `internal/platform/darwin/shim_darwin.m:721-727,825-851` | Distinct native and process-security boundary; public-release blocker |
| Origin, mixed-content, CSP and private-network rules | `internal/net/http.go:273-281`; `internal/engine/limits.go:571-584` | Policy work must accompany the relevant web capabilities |
| Links/forms/selection/accessibility/IME | `internal/raster/scheduler.go:246-317`; `internal/platform/darwin/shim_darwin.m:273-391` | Requires document event routing and native input contracts |
| JS and application Web APIs | `cmd/goosie/main.go:573-591`; `internal/engine/engine.go:445-475` | Requires retained sessions, runtime/event loop, APIs and containment |
| Cookies, downloads, profiles and privacy UX | `internal/net/http.go:270-288,342-345`; `internal/tabs/tab.go:9-18` | New user-data contracts and lifecycle, not current rendering safeguards |
| Further rendering compatibility and responsive style recalculation | `internal/engine/engine.go:401-443`; `internal/layout/grid.go:63`; `internal/layout/table.go:35` | Extend and test implemented subsets; layout-only `Reflow` is not a full style/media-query refresh |
| Fail-closed parity and headless readiness | `testdata/parity.py:30-52,99-109`; `cmd/goosie-headless/main.go:161-174` | Complete/fresh images and render settlement are separate acceptance work |
| Vet/no-cgo failures, artifact smoke and trusted updates | `test/style/gradient_test.go:113`; `internal/platform/platform_darwin.go:21`; `.github/workflows/release.yml:49-56` | Known release prerequisites; do not bury unrelated fixes in the renderer change |
| README/PROJECT and historical spec reconciliation | `README.md:3`; `PROJECT.md:5`; tab spec `:169-179` | Documentation includes obsolete architecture and contradictory shortcut behavior |

Deferred means outside this one implementation change, not optional for production. In particular, native isolation/policy issues remain release blockers even after tests for allocation safeguards pass.

## 7. Evidence and operating rules

- Preserve existing tests and architecture/cgo rules. Write regressions for the real failing path before code changes; use channels/local servers for ordering, not sleeps.
- Every supported behavior gets a named test; unsupported expectations remain visible and cannot silently count as passes. Select relevant Web Platform Tests instead of claiming full WPT coverage.
- Preserve parity fixtures and reference images. Pin browser/font/viewport/DPR versions and document readiness; inspect differences. Do not change fixtures, omit content, or relax tolerances merely to raise a score.
- Headless checks do not prove native behavior. For UI work, exercise the native feature and compare chrome-enabled headless output; verify keyboard/IME/accessibility separately.
- Resource policies use explicit units and cover in-flight and retained data. A per-response byte limit is not a page-memory limit; pixel bounds are not decoder CPU isolation; per-tab limits are not process limits.
- Keep current synthetic pacing thresholds on their declared hardware: mean frame ≤16 ms, p99 ≤33 ms, warm scroll ≤5 ms/op, cold scroll ≤8 ms/op (`.github/workflows/nightly-bench.yml:53-57`). Establish separate real-document first-paint/input/memory budgets before beta; do not infer them from the three-frame smoke.
- Before beta, record at least 1,000 representative navigation/interaction cycles with no crash/deadlock and resource growth within predeclared budgets. Before production, complete a 14-day release-candidate observation period, restarting after release-blocking fixes. These are proposed release policies, not results achieved here.
- Diagnostics must omit page content, URL query secrets, credentials and browsing history by default. Any telemetry requires explicit product/privacy approval.
- Assign release approval, vulnerability triage, update delivery and supported-version ownership before production. Test update authenticity and failure recovery; checksums alone are not an authenticated update channel.
- Estimate dates only after scope, team capacity and acceptance budgets are agreed. Report completed gates and remaining blockers, not speculative completion percentages.

Historical context remains in the existing architecture and feature specs; their approval labels do not override current code or acceptance evidence. No implementation, CI configuration, repository permission, dependency, fixture, commit or release was changed by this review.

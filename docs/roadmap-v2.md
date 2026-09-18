# Goosie Roadmap v2 — From Engine Prototype to Production

Updated: 2026-09-18  
Status: proposed delivery roadmap; milestones below are not yet accepted  
Baseline: `feat/v2` at `e3177f9`

“v2” identifies this roadmap revision and the current engine generation; it does not declare a production 2.0 release. The destination is a scoped production 1.0, not immediate Chrome/Firefox parity.

## 1. Direction and release scope

Keep the custom Go engine and its CPU tile-rendering architecture. Prioritize correctness, safe handling of untrusted content, and measurable compatibility before broader UI features or further rendering optimization.

Recommended initial production scope:

- Native macOS browser, with ARM64 as the primary validation target; advertise other architectures only after artifact and native testing.
- Headless renderer for deterministic testing and supported automation.
- A published HTML/CSS/JavaScript compatibility matrix and a named set of supported website workflows.
- Reliable navigation, forms, text input, accessibility, persistent browsing state, and recovery within that declared scope.
- Security boundaries suitable for visiting untrusted pages, including unsupported pages.
- Signed distribution, a supported security-update mechanism, and a release/support process.

A static-document alpha is an intermediate release, not a general-purpose production browser. Unsupported features must fail safely and be documented; a compatibility disclaimer is not a substitute for security.

Native Windows/Linux UI, extensions, account sync, advanced media, WebRTC, WebGL/WebGPU, and full standards parity are separate follow-on programs unless the Phase 0 target workflows make a specific capability necessary. Tabs and advanced developer tools are not prerequisites for the first single-window release.

Embedding an established browser engine would be a different strategy with faster compatibility but less ownership of the engine. This roadmap assumes continued investment in Goosie's own implementation.

## 2. Verified starting point

Current runtime:

```text
HTTP/file bytes → HTML DOM → CSS/style → block/inline layout → display list
               → 256px CPU tiles → damage compositor → macOS/headless window
```

The toolbar is outside the document pipeline. `website/` is a separate Docusaurus documentation site, not the browser application.

| Area | Evidence-backed baseline | Main gap |
| --- | --- | --- |
| Frame path | Worker pool, tile cache, damage composition, vsync scheduling, deterministic tests | Acceptance still relies heavily on synthetic scenes |
| Document pipeline | HTML, styles, layout, and painting connected in `internal/engine/engine.go:49` | Limited compatibility; existing stages are not complete standards implementations |
| CSS | Selector matching and property parsing | Specificity and importance ignored by `internal/style/style.go:481`; at-rules skipped in `internal/css/parser.go:43` |
| Layout | Basic block/inline layout and single-row flex | Text-dependent height propagation, complete flex, tables, Grid, positioning, and clipping |
| Text and images | Embedded Go Regular font; PNG/JPEG/GIF decoder | Font selection/shaping and document image sizing/painting not connected end to end |
| Navigation | Main-document fetch, address bar, history primitives | Cancellation, stale-result rejection, correct traversal semantics, and ownership of mutable UI state |
| Security | Standard Go HTTP/TLS behavior and local static analysis | Unbounded response reads, no browser origin-policy layer, no renderer sandbox, no dedicated fuzz/security gates |
| Distribution | Tagged archive builds for several OS/architecture combinations | Release smoke step does not run the binary; no trusted app distribution/update acceptance |

Fresh checks passed during the assessment on Go 1.26.5, macOS ARM64:

```bash
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```

These results cover the existing tests, not untested navigation paths, browser compatibility, native user journeys, or release installation. They do not establish results on every supported Go version or platform.

The README and architecture notes still describe a renderer-only engine. Historical milestone labels also need reconciliation with present acceptance evidence: `test/gate/scroll_gate_test.go:10` uses a synthetic scene rather than the HTML document pipeline.

## 3. Delivery sequence

| Phase | Outcome | Release boundary |
| --- | --- | --- |
| 0 | Trustworthy scope, tests, and acceptance evidence | Development baseline |
| 1 | Reliable and bounded document lifecycle | Internal preview |
| 2 | Correct static-document pipeline | Static-document alpha |
| 3 | Broader CSS/layout compatibility | Expanded alpha |
| 4 | Usable document interaction | Interactive alpha |
| 5 | Dynamic applications with security boundaries | Controlled application preview |
| 6 | Security, reliability, and performance hardening | Public beta, then release candidate |
| 7 | Trusted distribution and ongoing maintenance | Production 1.0 |

The main dependency chain is Phase 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7. Security design starts in Phase 0 and constrains every later phase; it is not deferred to beta. Packaging and native CI can progress alongside engine work after Phase 0, but cannot bypass the release gates.

### Phase 0 — Establish trustworthy acceptance

**Deliverables**

- [ ] Publish the supported-feature matrix, target-page corpus, and required user workflows, including explicit exclusions.
- [ ] Separate supported-feature pass rates from exploratory tests for unsupported features; retain expected failures with reasons rather than counting them as passes.
- [ ] Correct stale runtime documentation and reconcile historical milestone acceptance against executable evidence.
- [ ] Introduce automated Chromium comparison, selected Web Platform Tests, and deterministic local fixtures with pinned fonts, viewport, DPR, and reference versions.
- [ ] Define render completion by settled resources and visible tile readiness, not an arbitrary number of presents.
- [ ] Exercise the real HTML pipeline in performance acceptance alongside the existing synthetic microbenchmarks.
- [ ] Add macOS build/native smoke coverage before merge, declare supported Go versions, and identify required CI statuses.
- [ ] Define a threat model covering remote documents, redirects, local files, images, fonts, future scripts, persistence, and updates.

**Exit gate**

A clean checkout can reproduce the baseline; all currently supported expectations pass; unsupported expectations remain visible; real-document comparison and packaged-binary execution produce reviewed artifacts. Target hardware, numeric performance/resource budgets, and the scope of the first release are recorded before subsequent milestones are accepted.

### Phase 1 — Stabilize document lifecycle

**Deliverables**

- [ ] Establish one owner for navigation, toolbar, and document state; publish background results through an explicit handoff.
- [ ] Add navigation IDs, cancellation, and stale-result rejection; cancel outstanding work on close.
- [ ] Distinguish new navigation, Back/Forward traversal, reload, and replacement so history is not duplicated or truncated incorrectly.
- [ ] Preserve committed URL/history on failure; handle final redirect URLs, loading, failure, and retry states.
- [ ] Retain document session state and relayout on viewport/DPR changes; define scroll reset and history restoration behavior.
- [ ] Add context-aware requests, supported-scheme handling, content-type/encoding policy, and limits for response bytes, document complexity, geometry, tiles, and memory.
- [ ] Establish local-file access rules before resource loading expands.

**Exit gate**

Integration tests cover overlapping navigations finishing out of order, cancellation, redirects, failure/retry, Back/Forward/reload, resize, and close-during-load. These pass under race detection; oversized or malformed input is rejected without an application hang or uncontrolled allocation.

### Phase 2 — Make static pages correct

**Deliverables**

- [ ] Fix HTML tokenization/tree-building behavior required by the corpus, including entities, raw text, malformed markup, and whitespace.
- [ ] Implement cascade precedence by origin, importance, specificity, and source order; correct inheritance and computed values.
- [ ] Integrate block and inline sizing so line wrapping updates parent heights and following sibling positions.
- [ ] Preserve text boundaries, spaces, explicit line breaks, and whitespace modes through layout and painting.
- [ ] Introduce a document resource loader: base/relative URL resolution, linked stylesheets in document order, and image fetch/decode/sizing/paint.
- [ ] Support font-family fallback, weight/style selection, shaping, and bidirectional text for the declared language scope.
- [ ] Keep refresh behavior consistent with initial layout: preserve metrics, stylesheets, DPR, and correct damage.

**Exit gate**

Every required static-page fixture passes geometry and visual expectations at declared viewport/DPR combinations. External resources and failures are covered by deterministic local-server tests. No supported case depends on a synthetic replacement for document loading or layout.

### Phase 3 — Expand CSS and layout coverage

**Deliverables**

- [ ] Complete flex sizing, shrink/basis, alignment, wrapping, gaps, and anonymous items.
- [ ] Implement intrinsic sizing, min/max constraints, tables, and replaced-element sizing.
- [ ] Implement relative/absolute/fixed/sticky positioning, overflow, clipping, stacking contexts, visibility, and opacity.
- [ ] Add media queries, custom properties, relevant CSS functions, and Grid according to target-page requirements.
- [ ] Extend painting for required decorations, backgrounds, borders, and image fitting.
- [ ] Add correct invalidation first; optimize subtree work only after full-rebuild comparisons prove equivalent output.

**Exit gate**

Each supported feature has conformance and end-to-end coverage, including nested combinations and resize behavior. The target corpus passes without per-site rendering hacks, and existing frame-path budgets remain satisfied.

### Phase 4 — Enable real interaction

**Deliverables**

- [ ] Add hit testing and document event routing with press/release, modifiers, keyboard identity, and coordinate conversion.
- [ ] Preserve discrete input ordering; coalesce only events whose semantics permit it.
- [ ] Implement link activation, anchors, focus traversal, hover/active states, and native cursor feedback.
- [ ] Implement form controls, validation, submission, and associated navigation behavior.
- [ ] Add text selection/copy, clipboard integration, keyboard shortcuts, Unicode input, and IME composition.
- [ ] Expose a native accessibility tree and support keyboard-only and screen-reader workflows.

**Exit gate**

Required user journeys pass through both automated input tests and native macOS verification. Form input, navigation, selection, IME, and accessibility remain usable across resize, scroll, and failed loads.

### Phase 5 — Support dynamic applications safely

**Deliverables**

- [ ] Evaluate and integrate an established JavaScript runtime against compatibility, interruptibility, memory limits, licensing, and maintenance needs.
- [ ] Implement DOM bindings, script loading/execution order, events, task/microtask queues, timers, and rendering scheduling.
- [ ] Connect DOM/style mutations to correct invalidation, layout, and paint.
- [ ] Add Fetch and the Web APIs required by the target applications; record unsupported APIs explicitly.
- [ ] Implement origin and navigation policy, CORS, CSP, cookie attributes, mixed-content rules, and origin-scoped storage before exposing corresponding APIs.
- [ ] Place untrusted script/document execution behind a constrained process boundary with validated IPC and brokered network/file access.
- [ ] Add storage clearing, persistence/restart behavior, execution interruption, and renderer-crash recovery.

**Exit gate**

The declared dynamic-application workflows pass together with cross-origin, local-file, cookie/storage isolation, runaway-script, and crash-recovery tests. A JavaScript runtime alone does not satisfy this milestone; the surrounding browser APIs and security model are required.

### Phase 6 — Harden for public beta

**Deliverables**

- [ ] Add continuous parser, selector, URL, decoder, and IPC fuzzing; retain discovered failures as regression cases.
- [ ] Add dependency vulnerability scanning, a security-reporting process, and tracked remediation.
- [ ] Review the renderer sandbox and privilege boundaries; test file/network denial and process recovery.
- [ ] Run adversarial-content and endurance tests across navigation, redirects, resource loading, resize, input, and shutdown.
- [ ] Measure memory plateau, resource cleanup, actual style/layout work, input latency, first useful paint, and scroll pacing on real documents.
- [ ] Collect privacy-conscious diagnostics with a documented consent policy; exclude page content and credentials by default.

**Exit gate**

Security review has no unresolved release-blocking findings. A recorded soak of at least 1,000 navigation/interaction cycles has no crash, deadlock, leaked renderer process, or sustained resource growth beyond Phase 0 budgets. Public beta begins only after sandbox and origin-policy acceptance; the release candidate requires the full compatibility and native workflow suite.

### Phase 7 — Ship and maintain production 1.0

**Deliverables**

- [ ] Produce a signed/notarized macOS app with verified identity and version metadata; package the headless tool if advertised.
- [ ] Test installation, first launch, permissions, upgrades, profile migration, data clearing, uninstall, and recovery on clean supported systems.
- [ ] Execute extracted artifacts in release smoke tests; publish only after every supported target has passed validation.
- [ ] Provide checksums, dependency/license inventory, SBOM, and build provenance.
- [ ] Establish authenticated security updates and tested failure recovery; document the supported update channel and support lifetime.
- [ ] Publish compatibility limits, user documentation, release notes, issue triage, and a vulnerability-response policy.
- [ ] Complete a 14-day release-candidate soak; restart the observation period after release-blocking fixes.

**Exit gate**

All required tests pass for the release candidate and shipped artifacts, installation/update/recovery is verified, and no release-blocking correctness or security issue remains. Someone is explicitly responsible for release approval, security response, and maintenance. Passing CI alone is not release authorization.

## 4. Acceptance rules across every phase

| Dimension | Required evidence |
| --- | --- |
| Correctness | All required in-scope tests passing; known unsupported cases listed separately; no silently weakened expectations |
| Visual output | Pinned reference environment, recorded geometry/pixel tolerances, reviewed diffs, and retained images |
| Navigation/concurrency | Actual application integration scenarios under race detection, not only isolated state-object tests |
| Performance | Existing synthetic gates retained; real-document budgets frozen in Phase 0 and enforced independently |
| Resource safety | Declared limits for bytes, nodes, decoded pixels, geometry, execution, and memory; rejection/recovery tests |
| Security | Policy and boundary tests accompanying each new capability; fuzz/security regressions retained |
| Native behavior | macOS interaction and accessibility verification; headless success is not proof of native usability |
| Release | Tested packaged artifacts and recovery paths; reproducible evidence attached to the release candidate |

Preserve the current nightly reference budgets on their declared workload and hardware: mean frame time ≤16 ms, p99 ≤33 ms, warm scroll ≤5 ms/op, and cold scroll ≤8 ms/op. These are existing synthetic-workload thresholds, not evidence that arbitrary pages meet them. Real-page first-paint, input, and memory budgets must be established separately in Phase 0.

Do not improve a metric by dropping content, replacing HTML with a synthetic scene, increasing all memory limits, prewarming away the intended cold path, or relaxing comparison tolerances without reviewed justification.

## 5. First execution batches

Start with small test-led batches rather than implementing the whole roadmap at once:

1. **Phase 0 baseline:** refresh current documentation, declare release scope and threat boundaries, establish deterministic comparison fixtures and render readiness, record budgets, and make native/artifact checks executable.
2. **After Phase 0 acceptance:** establish application integration tests for navigation ordering, history traversal, shutdown, and resize; repair ownership/cancellation and history semantics against those tests.
3. **Complete Phase 1:** add response/document/resource limits, resize reflow, and visible failure behavior; pass the full lifecycle acceptance suite.
4. **Begin Phase 2:** add failing fixtures for specificity, `!important`, and text-dependent block height; fix these before extending CSS coverage.

Each batch gets its own reviewed implementation plan. This document does not authorize publishing releases, changing repository permissions, or committing unrelated work.

## 6. Scheduling and maintenance

Use exit gates, not feature-count percentages, to report progress. Assign an owner, acceptance evidence, and current blockers to each active phase. Calendar estimates should follow Phase 0 scope selection, team capacity, and measured delivery from the first batch; this roadmap makes no fixed-date promise.

Broad Chrome/Firefox-level compatibility remains a long-term program beyond a scoped 1.0. Revisit target workflows and release scope at each milestone without retroactively declaring unsupported behavior complete.

Historical context, retained rather than overwritten:

- [v2 architecture design](superpowers/specs/2026-09-11-goosie-v2-architecture-design.md)
- [M1–M3 frame-path design](superpowers/specs/2026-09-11-goosie-v2-m1-m3-frame-path-design.md)
- [Browser toolbar design](superpowers/specs/2026-09-17-browser-toolbar-design.md)

# SP-1 — Remove debug subsystem, harden hover fix

- Date: 2026-11-11
- Status: approved design, pending implementation plan
- Program position: first sub-project of the agreed chain SP-1 → SP-2 (V0) → SP-3 (P2 journeys) → SP-4 (L1+L2) → SP-5 (L3+L4+L5) → SP-6 (P3) → SP-7 (evidence-driven engine features)
- Base commit: `1bb471a` on `main` (working tree clean at design time)

## Context

Commit `1bb471a` landed two things together: a hover-flash fix (tracked-file changes) and a debug/phase-tracking subsystem (new packages, binaries, docs). A three-way review (reuse, quality, efficiency) found:

- `go vet -all ./...` fails on HEAD: `internal/debug/render_debug.go:219,231` copy a `sync.Mutex` embedded in `Stats` (copylocks). The repo gate requires vet clean.
- The debug subsystem is largely parallel reinvention: `PhaseTracker` duplicates `Session.WorkCounts` and hooks only the initial style pass, `Reflow`, and `Paint` — missing every incremental restyle path (`RestyleForHover`, `RestyleAndReflow`, PumpJS), so its phase-offset numbers are wrong. 10 of 11 wrapper methods in `internal/engine/phase_tracking.go` have zero callers. `internal/debug/overlay.go` is entirely dead (`GetObjectRect` is implemented by nothing). `Renderer` duplicates `frame.FrameRecorder`/`surface.LoopStats` and its only consumer feeds hardcoded zeros. `cmd/debug-render` and `cmd/test-render` are ~80% identical and duplicate `goosie -bench/-gate`; both read files unbounded, skip viewport validation, ignore `PaintChecked` errors (nil-panic path), and `test-render` shares one tracker across sessions (NodeID collisions). `docs/debug-tools.md` and `docs/phase-tracking.md` document flags that do not exist.
- The `phaseTracker interface{}` field claims "avoid import cycle"; no cycle exists (archtest already allows engine→debug).
- The hover fix itself is sound (empty-rect repaint diffs display lists in device pixels; cursor updates survive the early return; `LineHeight` comparison is px-vs-px; new tests fail without the fix) but has hardening gaps listed below.

Decision (user-approved): delete the debug subsystem entirely; keep `WorkCounts` + `FrameRecorder` + `-bench/-gate` + `dump-*` as the single measurement infrastructure. If per-node tracking is needed later, SP-2 (V0) designs it on the existing infrastructure.

## Scope

### Commit 1 — remove debug subsystem

Delete:

- `internal/debug/` (overlay.go, phase_tracker.go, profiler.go, render_debug.go)
- `cmd/debug-render/`, `cmd/test-render/`
- `internal/engine/phase_tracking.go`
- `docs/debug-tools.md`, `docs/phase-tracking.md`

Revert (remove only the debug-related hunks, keep everything else):

- `internal/engine/engine.go`: `phaseTracker` field, `WithPhaseTracker` option, the tracking hook in `NewSession` (node-ID batch + DOM walk registration), the tracking hook in `Reflow`
- `internal/engine/paint.go`: the tracking hook in `Paint`
- `internal/archtest/rules.go`: the `internal/debug` entry and the `internal/debug` element of the `internal/engine` allow-list

Keep:

- `testdata/render/debug-test.html` — committed render fixture, part of the parity corpus; removing it would change the corpus denominator
- `docs/README.md`, `docs/spec/*` — planning documents, out of SP-1 scope (see Flagged conflict)
- The hover-fix changes in `cmd/goosie/interaction.go`, `internal/engine/selection.go`, `internal/platform/darwin/shim_darwin.m`, and both new tests

After removal, grep the repo for dangling references (`internal/debug`, `PhaseTracker`, `WithPhaseTracker`, `debug-render`, `test-render`, `debug-tools`, `phase-tracking`) and fix any remaining mentions in kept docs.

### Commit 2 — harden the hover fix

1. `internal/engine/selection.go` `HasTextAt`: make band merging order-independent. Sort word line-rects by y0, merge two bands only when they overlap vertically AND their x-ranges overlap or touch. This fixes both review findings: false negatives when `selWords()` document order is not vertical order (abs-pos/float/flex-order content), and false positives in the gutter between two columns at the same y. Point-in-band test semantics stay half-open.
2. `textLineRect`: keep the half-leading expansion for explicit `line-height` (verified dimensionally correct against `layout/inline.go` centering). `line-height:normal` (`LineHeight <= 0`) keeps the border rect — a known limitation (normal can include font leading beyond ascent+descent); deferred to SP-7 text-shaping work. State this in a short comment.
3. Delete `Session.HoverDamageRect` (`internal/engine/hover_optimization.go`) — zero callers confirmed repo-wide. Keep `FocusDamageRect`/`SelectionDamageRect` (still used).
4. Trim the long comments to 2–3 lines each, keeping the non-obvious WHY: `cmd/goosie/interaction.go` hoverContent (flash root cause, CSS-vs-device pixels, deferred mutation), `internal/engine/selection.go` (line-box vs glyph-box hit), `internal/platform/darwin/shim_darwin.m` (AppKit redraw policy clears the layer).
5. `cmd/goosie/render_work_test.go` `TestHoverWithoutPseudoClassDoesNotRepaint`: add an assertion that hover state actually changed during the moves (e.g. a `mouseenter` listener counted via the JS runtime, or the session's hover-state accessor — whichever the existing test helpers support), so the zero-work-delta assertion cannot pass vacuously if hit-testing regresses to always-false.

## Out of scope

- Consolidating `selWords` caching by `layoutBuilds` (efficiency review item) — pre-existing hot-path cost, belongs to SP-4/SP-5 incremental work.
- `line-height:normal` leading, text shaping/bidi — SP-7.
- Any change to `docs/spec/*` or the SSOT conflict — SP-2.
- New features of any kind.

## Acceptance criteria

1. `go build ./...`, `go vet -all ./...`, `go test ./...`, `go test ./internal/archtest/` all pass.
2. `go test -race` passes for the affected packages: `./cmd/goosie/ ./internal/engine/ ./test/engine/`.
3. No dangling references to the removed packages/binaries/docs anywhere in the repo.
4. `TestHoverWithoutPseudoClassDoesNotRepaint` still asserts zero work delta AND asserts hover changed; `TestHasTextAtIncludesLineLeading` and the existing hover/selection suites pass.
5. A new regression test covers the order-independence fix: a fixture where document order is not vertical order (e.g. absolutely-positioned text above a later-in-document paragraph) reports `HasTextAt` correctly at points the old early-break loop missed.
6. Render parity corpus runs clean if a Chromium oracle is available locally; if not, record that it was not run. The macOS shim change cannot be verified natively in a display-less session — record that explicitly.
7. Two commits on `main`: (1) debug subsystem removal, (2) hover hardening. No other files touched.

## Flagged conflict (not resolved here)

`docs/README.md` declares `docs/spec/` the mandatory single source of truth, contradicting AGENTS.md, which names `docs/roadmap-v2.md` as the current production-readiness review; the spec documents describe a greenfield v2.0 architecture (flat 64-byte structs, process pool, hybrid rasterizer) that differs from the built engine. Resolve in SP-2 before planning feature work against those specs.
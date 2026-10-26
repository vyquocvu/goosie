# TASK-054 — an IntersectionObserver deadlocks the load path (GOOSIE-IOEXPORT-001)

**Priority:** P1. **Method:** UNIT (a watchdog oracle in `test/js/`) + INT (the real `NewSession` load path) + LIVE (the two pages sweep #9 lost, then a 100-site re-run by the integrator).
**Owner:** agent tasked with TASK-054. **Defect:** `GOOSIE-IOEXPORT-001` (P1, open, `docs/qa-open-defects.json`).
**Base:** `main` at the commit that registers this doc; read `git log -1 --format=%H` and write it in §8.
**Language:** English — every number here has to be quotable verbatim into the defect registry.

## 0. Rules for this branch

- **Do not touch** `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
  `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`, or
  `testdata/parity.py`. The integrator owns those. Propose BR ids and exact test names in §8; adding a
  `// qc:` marker for an id that is not in the catalog yet would red `go test ./test/qc/`.
- **Do not touch** `internal/js/js.go`, `internal/js/shadow_dom.go` or any other file TASK-051 is editing
  (`docs/tasks/TASK-051-DOM-SELECTOR-MATCHING.md`). This task is confined to
  `internal/js/intersection.go`, its tests, and (optionally) one `testdata/render/js-dom/` fixture.
  If you believe the fix cannot be contained there, stop and say so in §8 instead of widening the diff.
- No `git push`, no merge, no PR. One worktree, commits only on your branch.
- Every claim needs the command, the real number and the exit code measured on your host. If a prediction
  turns out wrong, correct it in place and say what was wrong — do not quietly restate it.

## 1. Reproduction, measured by the integrator on 2026-10-26

Minimal page `/tmp/t50-iohang.html` (a 100×100 blue `div#x` plus
`new IntersectionObserver(function(es){ document.title = "io " + es.length; }).observe(document.getElementById("x"))`)
and its control `/tmp/t50-control.html` (same div, no script), rendered with
`-backend headless -width 200 -height 150 -dpr 1 -screenshot`:

| page | merged build `b46d4d5` | `a5c54245` (sweep-8 source) | `4ea1430` (TASK-047 merged) |
|------|------------------------|-----------------------------|------------------------------|
| control | rc 0, 0.18 s, PNG | rc 0, 0.18 s, PNG | rc 0, 0.24 s, PNG |
| observer | **no exit in 20 s** | **no exit in 15 s** | **no exit in 15 s** |

The deadlock is latent and predates TASK-047. What TASK-047 changed is that real pages now **reach** the
observer: sweep #8's diagnostic for getbootstrap was
`TypeError: Object has no member 'addEventListener'`, so its scripts died before registering one. Sweep #9
(`docs/qa-live-parity/2026-10-26T0427Z-b46d4d51.{json,md}`) shows the two live casualties:

| URL | sweep #8 | sweep #9 |
|-----|----------|----------|
| `https://getbootstrap.com/docs/5.3/getting-started/introduction/` | `LOW_COVERAGE`, 0.54 s, rc 0 | `GOOSIE_TIMEOUT`, 120.01 s, rc 124 |
| `https://vietnamnet.vn/` | `LOW_COVERAGE`, 2.21 s, rc 0 | `GOOSIE_TIMEOUT`, 120.01 s, rc 124 |

Both pages still render on a binary built from `a5c54245` on this host today: 1.2 s and 2.3 s, rc 0 each.
A binary built from `4ea1430` times out on both (90 s cap in the bisect). TASK-050 is exonerated; the defect
is `internal/js/intersection.go`.

The hung process is **idle**, not slow: 0.36 s of CPU after 60 s, its sockets CLOSED.

## 2. Cause, from the goroutine dump

A SIGQUIT dump of the merged build on getbootstrap (`/tmp/t50-hang.err`, 49 KB) puts goroutine 1 at:

```
[sync.Mutex.Lock]
js.(*Runtime).jsGetShadowRoot            internal/js/shadow_dom.go:120
js.(*Runtime).setupDOM.func56            internal/js/js.go:1168
goja.(*valueProperty).get / Object.Export
js.(*Runtime).CheckIntersections         internal/js/intersection.go:311
engine.(*Session).Reflow                 internal/engine/engine.go:1414
engine.NewSession                        internal/engine/engine.go:516
cmd/goosie/document_loader.go:437
```

`CheckIntersections` takes `r.mu` at `internal/js/intersection.go:308` and, still holding it, calls
`entry.target.Export()` at `:311`. Exporting a node wrapper walks its enumerable properties, which include
`parentNode` (`internal/js/js.go:1806-1820`) and `shadowRoot` (`internal/js/shadow_dom.go:22-33`) — both
`enumerable: true` getters that call back into the runtime, and `jsGetShadowRoot` then takes `r.mu` again.
Go mutexes are not reentrant, so the load path waits forever. The object export is only used to recover a
`__nid__` the observer already had at `observe()` time.

Sibling observers do not have the bug and are the reference: `MutationObserver`
(`internal/js/mutation_observer.go:109`) and `ResizeObserver` (`internal/js/resize_observer.go:98`) read
`__nid__` when `observe()` is called and keep a `*dom.Node`.

## 3. The fix

In `internal/js/intersection.go` only:

1. Add `targetID int` to `IntersectionEntry`; populate it in `intersectionObserve` from the argument's
   `__nid__` (reject/warn exactly as the two sibling observers do if it is absent).
2. In `CheckIntersections`, replace the `entry.target.Export()` scan with `entry.targetID`, and do not call
   any goja property accessor or `Export` while `r.mu` is held. Take the lock only to read the runtime maps
   (`nodeRegistry`, `elementRects`) by ID.
3. Keep `entry.target` for the callback payload (`fireIntersectionCallback` sets `target` on the entry
   object); do not change the observable shape of `IntersectionObserverEntry`.
4. While you are there, prove with a grep and a sentence in §8 whether any other `Export()` or `.Get(...)` on
   a page-controlled object happens under `r.mu` in `internal/js/`; if one does, it is out of scope but must
   be stated, not silently left.

## 4. Oracles

- `test/js/` watchdog test: calling `NewSession` (or the js-level equivalent the file uses) on a page with
  an observer must return. Run the load in a goroutine and fail if it has not returned after a generous
  wall-clock cap (10 s). This is a liveness assertion, not a performance budget: a deadlock fails, a correct
  build returns in milliseconds, and the cap never grades speed. Say in the test comment that the cap is a
  watchdog and why it is not tuned.
- The callback still fires for a visible element and the entry's `target` is the same object the page
  observed (identity, not just `__nid__`).
- An element that is not visible does not fire, so the oracle cannot pass by firing everything.
- The two live URLs above are integration evidence, not part of `go test` (network); record their timings.
- Optional but preferred: one `testdata/render/js-dom/` fixture where an observer reveals content, so the
  visual corpus pins the load path. If you add one, do **not** change `MIN_SCORED_FIXTURES`; report the
  corpus you measured and the integrator raises the floor.

## 5. Mutations (behaviour-off, each must die and be restored)

- M01: restore the `Export()`-under-`r.mu` scan → the watchdog test must fail by timeout (record rc and the
  message; if the test binary's own timeout is hit instead, tighten the watchdog).
- M02: use `entry.targetID` but read it after re-locking `r.mu` (simulating re-entry through a getter) → the
  deadlock must reappear and the watchdog must catch it.
- M03: drop the `targetID` capture and default it to 0 → the visible-element test must fail (no callback),
  proving the ID is read, not guessed.
- Any mutant that survives is an equivalent mutant: say so instead of claiming a kill.

## 6. Gates

`go test -count=1 ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `gofmt -l` empty on every
touched file. Then, after the merge, the integrator re-runs the two live URLs and sweep #10; do not run a
100-site sweep yourself.

## 7. What to write in §8

Base commit; the diff inventory; each oracle with its exact test name and the measured Chromium/Goosie values
the expectation came from; the mutation table with dying test names and rc; the gates; the two live URLs with
timings measured on your branch's binary; the grep result for other `Export`-under-`r.mu` sites; and every
thing you did **not** measure, stated plainly.

## 8. Report — filled by the implementer

### 8.0 Base, branch, commits

- Worktree: `/Users/vyquocvu/Develop/Browser/goosie/.qoder/worktrees/agent-general-purpose-e0a73aae`,
  branch `worktree-agent-general-purpose-e0a73aae`.
- Base: `59cbca7` (`docs(qa): record the ninth 100-site sweep and file GOOSIE-IOEXPORT-001`), which is the
  local `main` head that contains this document and the sweep-#9 evidence. The worktree was created at
  `c8cd80b`, which predates this document; it was fast-forwarded with `git merge --ff-only main` before any
  work started (no local commits existed, so nothing was rewritten, and nothing was merged into `main`).
- Fix commit: `00032a3` — `fix(js): capture an IntersectionObserver target's node ID at observe time`.
- Report + fixture: the follow-up commit on the same branch (this section and
  `testdata/render/js-dom/03-intersection-observer-first-reflow.html`). Nothing was pushed, merged or opened
  as a PR.

### 8.1 Diff inventory

`00032a3` touches exactly two files, 363 insertions / 22 deletions:

| file | change |
|------|--------|
| `internal/js/intersection.go` | `IntersectionEntry.targetID int` added (+ comment stating why no property may be read off `target` later); `intersectionObserve` captures `target.Get("__nid__")` once, defaulting to `-1`; `CheckIntersections` replaces the `Export()`-under-`r.mu` scan with `r.intersectionRectOf(entry.targetID, elementRects)`; new helper `intersectionRectOf` takes `r.mu` for one map read, translates wrapper ID → `*dom.Node` → `node.ID`, and returns the rect **after** unlocking. |
| `test/js/intersection_observer_test.go` | new file, 296 lines, four oracles plus the `runUnderWatchdog` helper. |

The follow-up commit adds one fixture (99 lines, most of it the header comment that says what it does and does
not gate) and this section. No other production file was touched:
`internal/js/js.go`, `internal/js/shadow_dom.go`, and every file TASK-051 is editing are byte-identical to
`59cbca7`, and no integrator-owned file (`docs/qa-test-catalog.md`, `docs/qa-open-defects.json`,
`docs/qa-ratchet.json`, `test/qc/catalog.go`, `test/qc/traceability.golden.json`,
`testdata/render-known-gaps.json`, `testdata/parity.py`) was edited.

### 8.2 Oracles, with the values the expectations came from

All four live in `test/js/intersection_observer_test.go`. The shared helper `runUnderWatchdog` runs the whole
runtime lifetime — construction, script, checks, and `rt.Close()` — inside one goroutine and returns only a
plain string, because `Runtime.Close` takes the same mutex: a `defer rt.Close()` in the test goroutine turns a
deadlock into a hung *test binary*, so the in-test cap never fires. `intersectionWatchdog` is 10 s. It
is a liveness cap, not a budget: the correct build finishes each test in under 10 ms (`go test -v` prints
`0.00s`), so the cap has three orders of magnitude of headroom and was not tuned to any machine.

| test | asserts | measured value |
|------|---------|----------------|
| `TestIntersectionObserverCheckReturns` | Two `CheckIntersections()` calls on a 200×150 viewport with `#seen` at (0,0,100,100) and `#off` at (0,400,100,100) deliver exactly one callback, one entry, `target ===` the object the page passed to `observe()`, `isIntersecting` true, and **no second event** on the re-check. | log `[{"count":1,"seen":true,"off":false,"intersecting":true}]`; PASS in `0.00s` |
| `TestIntersectionObserverDoesNotFireForOffscreenTarget` | An observed element entirely below the fold fires nothing, so an implementation that fires every entry on every check cannot pass the positive test. | log `[]`; PASS in `0.00s` |
| `TestIntersectionObserverObserveOfPlainObjectNeverFires` | `observe({})` and `observe({width,height})` neither throw nor ever fire (see 8.7 for why this is not the sibling observers' TypeError). | log `[]`, `Run` returns nil; PASS in `0.00s` |
| `TestNewSessionWithIntersectionObserverReturns` | The real load path: `js.New` + `engine.NewSession(ioPage, …, WithViewportH(150), WithJS(rt))` returns under the cap, and the callback has run **during the reflow `NewSession` performs** (its `document.title` write is visible afterwards). | title `io 1`, log `[{"count":1,"seen":true,"off":false,"intersecting":true}]`; PASS in `0.00s` |

The expectations are not invented numbers: the entry object's fields (`target`, `isIntersecting`) are the ones
`fireIntersectionCallback` builds, the page's own log records `entries.length` and identity, and the geometry
is in the engine's own units — `SetElementRect` takes `dom.Node.ID`, which is why the wrapper→node
translation in `intersectionRectOf` is load-bearing and not cosmetic. Before the fix, the first and fourth
tests hung at `CheckIntersections` (SIGQUIT stack
`jsGetShadowRoot ← Object.Export ← CheckIntersections ← Reflow`); after the fix they return immediately. The
same page with the observer stripped is the control that proved the page itself is not slow: 0.18–0.25 s,
rc 0, PNG, on the buggy binary.

### 8.3 Mutations (behaviour-off, each applied to the committed fix, each restored)

Every mutant was built with `go build ./...` (plus `gofmt -l` empty) before the run, and the suite was run
with `go test -count=1 ./test/js/ -run 'TestIntersectionObserver|TestNewSessionWithIntersectionObserver' -v
-timeout 300s`. Restore was `git checkout -- internal/js/intersection.go`; after each restore the same command
returns **rc 0** in `0.69s` (whole package).

| id | mutant | rc | dying tests (all at the 10 s watchdog) |
|----|--------|----|----------------------------------------|
| M01 | the pre-fix code: re-lock `r.mu`, then `entry.target.Export()` and read `__nid__` off the result, keyed by `nodeID` — the `-` side of the `CheckIntersections` hunk in `00032a3` | **1** | `TestIntersectionObserverCheckReturns` (10.00s), `TestIntersectionObserverDoesNotFireForOffscreenTarget` (10.00s), `TestNewSessionWithIntersectionObserverReturns` (10.00s). Message: `… did not return within 10s: it is deadlocked, not slow (GOOSIE-IOEXPORT-001). The check must not call Export() or any property getter on a node wrapper while holding the runtime mutex.` Package FAIL in `30.646s`; the other five observer tests and the plain-object oracle passed. |
| M02 | keep `entry.targetID`, but add `r.mu.Lock(); _ = entry.target.Get("shadowRoot"); r.mu.Unlock()` immediately before the `intersectionRectOf` call — an explicit re-entry through the same enumerable getter | **1** | same three tests, same three messages, each exactly 10.00s; package FAIL in `30.798s` |
| M03 | drop the `targetID` capture and leave the field at its zero value (`targetID: 0`), so the check looks up a registry key that is never assigned (`nextNID` starts at 1, `nodeRegistry[0]` is nil) | **1** | `TestIntersectionObserverCheckReturns` (0.01s, `observer log = [], want [{"count":1,…}]`) and `TestNewSessionWithIntersectionObserverReturns` (0.00s, `document title after load = "io", want "io 1"`). Package FAIL in `0.686s`. |

Notes, stated rather than glossed over:

- M03 kills the two tests that require a callback and **not** `TestIntersectionObserverDoesNotFireForOffscreenTarget`
  or `TestIntersectionObserverObserveOfPlainObjectNeverFires`: both of those expect *silence*, and a build
  that never fires satisfies silence trivially. That is the negative-test pair working as designed (they exist
  to stop M01/M02-class bugs from passing on a "fires everything" implementation), but it is worth being
  explicit that a no-callback mutant is caught by the positive tests only.
- No mutant survived, so there is no equivalent mutant to declare.
- M02 is stronger than the defect needed: the original bug read a getter *inside* `Export`; the mutant calls
  `Get("shadowRoot")` directly while holding the lock and dies identically, which shows the oracle is
  sensitive to "property access under `r.mu`" and not to `Export` as a particular API.
- M01 also reproduced on the artifacts: a binary built from the M01 source renders the minimal observer page
  and the new parity fixture not at all (20 s cap, no PNG) while the fixed binary finishes both, and the
  SIGQUIT dump matches §2 line for line.

### 8.4 Gates (exact exit codes, on the tree that contains the fix **and** the fixture)

| gate | command | exit |
|------|---------|------|
| full suite | `go test -count=1 ./...` | **0** (`test/gatecheck` 32.860s, `test/js` 5.186s, `internal/archtest` 3.982s) |
| vet | `go vet -all ./...` | **0** |
| architecture | `go test ./internal/archtest/` | **0** |
| formatting | `gofmt -l internal/js/intersection.go test/js/intersection_observer_test.go` | **0**, no output |

The oracles themselves: `go test -count=1 ./test/js/ -run 'TestIntersectionObserver|TestNewSessionWithIntersectionObserver'`
→ **rc 0** in `0.691s`, all nine tests (four new, five pre-existing) PASS.

### 8.5 The two live URLs, measured on this branch's binary

Binary: `go build -o /tmp/goosie-t054-fixed ./cmd/goosie` from `00032a3`, invoked exactly as `parity.py` does:
`-backend headless -width 1200 -height 900 -dpr 1 -screenshot -out … -url …`.

| URL | fixed binary | same build with M01 applied |
|-----|--------------|-----------------------------|
| `https://getbootstrap.com/docs/5.3/getting-started/introduction/` | **rc 0, 3.48 s**, PNG 117,309 bytes (page logs one uncaught script error, pre-existing) | **no exit in 30 s**, no PNG |
| `https://vietnamnet.vn/` | **rc 0, 2.10 s**, PNG 336,420 bytes | **no exit in 30 s**, no PNG |

Both PNG byte counts are identical to a second run, and identical to the run made before the fixture was
added, so the render is deterministic for these pages. These are wall-clock network measurements on this
host on 2026-10-26 and vary with the network; the meaningful fact is rc 0 versus no exit, not the seconds.
The buggy build was capped at 30 s rather than 120 s because the deadlock does not resolve; the integrator's
§1 numbers (120.01 s, rc 124) remain the sweep-of-record.

### 8.6 Grep audit: other `Export()` / property access under `r.mu` in `internal/js/`

Method: a script over every non-test `.go` file in `internal/js/` that tracks `r.mu.Lock`/`RLock` nesting
(including `defer r.mu.Unlock()`, treated conservatively as "locked to the end of the function") and flags any
line inside the locked region that calls `.Export()`, `.Get(`, `.Set(`, `ExportTo`, `AssertFunction` or
`.Call(`. It found **44 lock statements in 41 functions (19 of them deferred) and zero flagged lines.** The
only two function-level hits from a coarser pass were false positives, checked by hand:

- `internal/js/custom_elements.go:196 invokeLifecycleCallback` — `wrapper.Get(callback)` at line 222 is
  **after** `r.mu.Unlock()` at 215. The locks at 198 and 213 cover a map scan and a map read only.
- `internal/js/shadow_dom.go:45 jsAttachShadow` — `call.Arguments[0].Export()` at line 59 runs **before** the
  lock at 97, which then covers one `nodeRegistry` scan and one map write.

`js.go:Run` and `js.go:Evaluate` also hold `r.mu` only for the `closed` flag read and release it before
entering goja, so the whole-script path is not a lock-under-getter path either. `Runtime.mu` is unexported,
so no package outside `internal/js` can hold it; the engine side (`Reflow` → `SetViewportRect` /
`SetElementRect` / `CheckIntersections`) never touches a goja value under it. This is the answer to §3.4:
**no other `Export`-or-property-under-`r.mu` site exists in `internal/js/` today.** The audit is
structural, not a proof about future code: nothing in the package stops someone re-introducing the pattern,
and the archtest does not police it.

### 8.7 Corrections to this document

- **§3.1 is wrong about rejecting a non-node argument.** It says to "reject/warn exactly as the two sibling
  observers do if `__nid__` is absent". `MutationObserver` and `ResizeObserver` panic a `TypeError` there;
  this observer must not, because `observe()` with a plain object has always been a silent no-op and
  `test/js/observer_raf_test.go:50 TestIntersectionObserverObserve` asserts exactly that (`observer.observe(obj);
  // Should not throw`). The fix therefore records `targetID = -1` ("no node id") instead of throwing, keeps
  the argument accepted, and `TestIntersectionObserverObserveOfPlainObjectNeverFires` pins that such an entry
  can never fire — there is no geometry to intersect. Copying the siblings would have broken a green test and
  a documented behaviour of this observer.
- **The `-1` sentinel is not cosmetic either.** `0` is a valid wrapper ID in principle, so "default to 0" is
  the kind of sentinel that hides a bug rather than announcing one; `-1` makes "this entry has no node" an
  explicit state. M03 shows that defaulting to `0` yields no callback at all, which is why it is a mutation and
  not a design.
- **§4's optional fixture suggested "an observer reveals content".** The fixture that landed does not make
  pixels depend on *when* the callback lands, and that is deliberate, measured, not a shortcut: Chromium
  delivers `IntersectionObserver` callbacks in the rendering steps that follow the layout the entries
  describe, so a reveal written from the callback can land one frame after `page.screenshot`. A fixture built
  that way would score a race between two engines rather than a fact about either. The fixture's gated fact is
  the one this task is about — the document must finish loading — and the callback's only recorded effect is
  `document.title`, which is not painted. Before the fix this fixture produces **no PNG at all**, which is how
  parity reports a hang (`MISSING`), so the corpus does gate the defect; it does not replace the JS oracles
  for callback behaviour.
- **§1's reproducer is accurate as written** but worth restating for anyone re-running it: `-screenshot`
  requires `-url`; passing the URL as a positional argument exits rc 2 with a usage line, not with a hang.

### 8.8 Parity fixture

`testdata/render/js-dom/03-intersection-observer-first-reflow.html` (new, 99 lines). It is the live shape:
`new IntersectionObserver(...)` + `observe()` from inline parse-time script, two targets (one in view, one
far below the fold), a `load` handler that disconnects, and surrounding `createElement` / `setAttribute` /
`appendChild` / `document.title` work so the page paints content whether or not the callback is early.

Measured, not assumed. Because `parity.py` walks `testdata/render` relative to the current directory and
refuses to score fewer than `MIN_SCORED_FIXTURES`, the fixture was scored in a throwaway tree
(`/tmp/t054-parity/testdata/render/`) that held this one file plus 90 copies of it, so the floor was met
without touching the repository's corpus:

```
97.56%  PASS  js-dom/03-intersection-observer-first-reflow.png
        worst_region_iou=0.984375  coverage_ratio=0.9966953658407048  blank_regions=0/0  shifted=0
91/91 passing (100.0%), average match 97.56% (threshold=30, pass_at=90.0%)
```

with Goosie 0.20 s / 32,254-byte PNG and Chromium 131.0.6778.33 as the reference. The same fixture rendered
by the M01 binary produces no PNG (cap 20 s), which is the pre-fix behaviour this fixture exists to keep out
of the corpus.

**Integrator note (2026-10-26):** on the merged tree this fixture sits beside TASK-053's, so the corpus is
**93** and `MIN_SCORED_FIXTURES` went **92 → 93** in the registration commit; the fixture itself scores
**97.56% PASS** (`worst_region_iou=0.984375`, `coverage_ratio=0.9966953658407048`) in the full run, corpus
93/93 average 96.23%. Registered as **BR-234** (liveness/geometry, four oracles) and **BR-235** (integration +
this fixture).

### 8.9 What I did not measure

- **Sweep #10 / the 100-site sweep.** §6 says not to run one; I did not. The two live URLs in 8.5 are the
  only network evidence, and they exercise two of the sites sweep #9 lost, not the corpus.
- **Parity over the real corpus.** The 8.8 score comes from a padded probe tree containing this fixture (×91);
  I did not run `parity.py --render --render-chromium` over the repository's 92 fixtures, so I cannot state a
  corpus-wide average or claim nothing else drifted. `test/gatecheck` does not run full parity either.
- **Linux and Windows.** Everything was measured on darwin/arm64 (macOS, Chromium 131.0.6778.33). The bug is
  a non-reentrant mutex and is platform-independent, but no cross-platform run was made.
- **`go test -race`.** Not part of §6 and not run. `targetID` is written before the entry is published under
  `obs.mu` and read without `r.mu`, which is safe because it is immutable after `observe()`, but that
  reasoning is a reading of the code, not a race-detector result.
- **`targetID` validity over time.** If a page `unobserve()`s and the wrapper is collected, or a document is
  re-set with `SetDOM` (which resets `nodeRegistry` and `nextNID`), a stale `targetID` could resolve to a
  different node. `intersectionRectOf` returns "no rect" for a missing key and the entry is skipped, so the
  failure mode is a missed callback rather than a crash, and no test covers wrapper reuse across `SetDOM`.
  This is pre-existing (the old scan had the same exposure) and was not measured.
- **Scroll-driven intersection changes.** Only the post-layout check path is covered; `checkIntersections` on
  scroll with a moving viewport, thresholds other than the default, and `rootMargin` were not exercised
  (the observer's threshold handling is untouched by this fix).
- **The one uncaught script error on getbootstrap** (8.5) is reported by the binary and was not investigated;
  it predates this branch and is not part of this defect.


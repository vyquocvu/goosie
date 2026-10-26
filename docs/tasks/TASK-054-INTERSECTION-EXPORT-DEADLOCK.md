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

TODO

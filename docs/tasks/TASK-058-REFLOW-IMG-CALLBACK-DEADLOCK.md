# TASK-058 — a Reflow deadlocks when an observer callback loads an image (GOOSIE-IMGLOAD-001)

**Priority:** P1. **Method:** INT (the real `NewSession` load path under a watchdog) + LIVE (vneconomy.vn, the sweep #13 casualty).
**Owner:** integrator. **Defect:** `GOOSIE-IMGLOAD-001` (P1, open in `docs/qa-open-defects.json`).
**Base:** `main` at `174a14a`; branch `fix/imgmu-js-deadlock`, fix commit `8654797`.
**Language:** English — every number here is quotable verbatim into the defect registry.

## 1. Reproduction

Sweep #13 (`7f71648`) recorded `https://vneconomy.vn/` as `GOOSIE_TIMEOUT`. Reproduced on main
`174a14a` with:

```
goosie -backend headless -width 1280 -height 1024 -dpr 1 -screenshot \
       -url https://vneconomy.vn/ -out /tmp/vneconomy.png
```

No exit in 60 s (killed); a SIGQUIT dump of the hung process shows goroutine 1 at:

```
sync.Mutex.Lock
engine.(*Session).beginScriptImage        internal/engine/script_image.go:71
engine.(*Session).LoadScriptImage         internal/engine/script_image.go:55
js.(*Runtime).startImageLoad              internal/js/element_interfaces.go:1575
  (HTMLImageElement.src setter)
js.(*Runtime).fireIntersectionCallback    internal/js/intersection.go:465
js.(*Runtime).CheckIntersections          internal/js/intersection.go:302
engine.(*Session).Reflow                  internal/engine/engine.go:1457
engine.(*Session).RestyleAndReflow        internal/engine/engine.go:2382
engine.NewSession                         internal/engine/engine.go:567
```

No other goroutine is running engine code: the process is idle, not slow.

The minimal offline oracle is `test/engine/reflow_callback_image_test.go` — an
`IntersectionObserver` whose callback assigns a `data:` `img.src`, registered inline so the load path
runs exactly as on a live page. Pre-fix it fails at the 10 s watchdog; every run of
`TestIntersectionCallbackMayStartAnImageLoad` on `main` reproduces the hang with no network.

## 2. Cause

`Reflow` took `imgMu` with `defer Unlock` for its whole body (`engine.go:1319-1320`). Its last step is
`CheckIntersections()`, which invokes page callbacks **synchronously on the Reflow goroutine**. A
callback that assigns `img.src` calls the injected `ImageLoader`, which is the session itself, and
`beginScriptImage` takes `imgMu` again. Go mutexes are not reentrant, so the load path waits on a lock
it already holds. The same applies to the synchronous `data:` decode path (`decodeScriptImage` also
takes `imgMu`), so any script-assigned source deadlocks, not only network URLs.

This is the second deadlock of the same shape on the Reflow call path. TASK-054 removed a
JS-into-runtime re-entry (`GOOSIE-IOEXPORT-001`); this one re-enters the engine's loader. The rule the
two share: **a Reflow must not dispatch page JS while holding a lock that code reachable from that JS
can take**.

## 3. Fix

`internal/engine/engine.go`, `Reflow` only:

- the unconditional `defer s.imgMu.Unlock()` is replaced by a guard (`imgLocked`), keeping error
  returns before the dispatch covered;
- `imgMu` is released immediately before `CheckIntersections` (and before the observer geometry
  setters), then the callbacks run unlocked.

No other `imgMu` section dispatches page code: `applyImages` (engine.go:1151) and `fetchImages`
(engine.go:1067) are pure Go; `LoadDeferredImages` calls its `onReady` after unlocking
(engine.go:896-899). `drainScriptImages` on the TASK-057 integration branch also runs no JS.

## 4. Verification measured on this host

| check | result |
|-------|--------|
| oracle pre-fix (`main` `174a14a`) | FAIL at 10.00 s, watchdog line |
| oracle post-fix (`8654797`) | PASS in 0.01 s; `naturalWidth x naturalHeight` = `4x4` after one `PumpJS` |
| mutation: remove the early unlock (restore old scope) | FAIL at 10.00 s; restore byte-identical, PASS |
| `go test ./test/engine ./internal/engine ./test/js -count=1` | rc 0 |
| `go test -race ./test/engine -run 'TestIntersectionCallback...|TestIntersectionObserver|TestScriptImage'` | rc 0 |
| `gofmt -l` on changed files; `go vet` on engine + test/engine | no output / rc 0 |
| vneconomy.vn through a binary built from `8654797` | rc 0, PNG 90,027 B, 62,272 sampled non-white pixels of 327,680 (every second row/column) |

The pre-fix vneconomy run never produced a PNG; the post-fix one paints the page (19% sampled ink), so
the fix does not merely return early from the load.

## 5. Registry proposal

- `BR-250 | P1 | INT | PASS` — a Reflow must not dispatch page JS while holding a lock reachable from
  that JS: an `IntersectionObserver` callback that assigns `img.src` during the load-path Reflow
  returns, and the image decodes. Oracle `TestIntersectionCallbackMayStartAnImageLoad`; mutation:
  restoring the old lock scope fails it at the 10 s watchdog.
- Defect `GOOSIE-IMGLOAD-001`, owner this doc, closed by the fix commit once merged.

## 6. Still owed

- Full gate (`go test ./...`, race, vet, archtest, gofmt) and landing on `main`; this doc records the
  branch measurement, and the registry is updated only after the merge.
- Re-run the sites that sweep #13 lost to this deadlock after landing (sweep #14), and compare their
  labels against sweep #13.

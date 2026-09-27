# Testing strategy: tiers, baseline, and the next backlog

Written 2026-09-26 against `feat/v2` at `0a0fedc`. Every number below is from a command run
on that commit, not from a remembered state. Re-run them before acting on the baseline.

## 1. Where the suite actually stands

```
go test ./...                                  # 28 packages with tests, 22 without; all green
go test -coverpkg=./internal/... -coverprofile=c.out ./test/... ./cmd/goosie ./internal/...
go tool cover -func=c.out | tail -1            # total: 75.1% of internal statements
```

The suite is mature, and the strategy has to be read against that fact rather than against
a greenfield plan. Two things the brief asked for are already done and were **not**
repeated here:

- Unit depth on `internal/net/cache.go` and `internal/tabs/manager.go` — 8 and 10 functions,
  all reachable from `test/net` and `test/tabs`, at 100% function coverage. Writing a second
  batch of unit tests for these two modules would add maintenance, not defects found. What
  was missing on the tabs side is the *integration* with the frame path, which is §3, not
  this §1.
- E2E navigation, rendering, and scroll workflows — `test/gate` (three files) plus
  `testdata/parity.py` scoring every fixture against cached Chromium PNGs.

What the audit did find is two tiers with no real coverage: the pacing gate, and the host's
navigation-staleness contract.

| Tier | Where | State at audit |
| --- | --- | --- |
| Unit, pure logic | `test/{css,style,layout,dom,frame,net,tabs,toolbar,history,bookmarks,download,session}` | covered, 100% of functions in the named modules |
| Integration, components wired | `test/{engine,surface,raster,paint}` | covered; `test/engine` alone reaches 32.3% of all internal statements |
| Integration, host ownership | `cmd/goosie/tab_lifecycle_test.go` | **serial guard asserted by a tautology** |
| E2E, real frame path | `test/gate`, `cmd/goosie/*_test.go` | covered |
| Architecture | `internal/archtest` | covered; import-down and single-cgo-directory |
| Entrypoint limits | `test/entrypoints` | covered |
| **Perf gate tooling** | `scripts/v2-gate-check.sh` | **zero assertions, anywhere** |

`internal/ax` reports no coverage because it declares no behaviour — 35 lines of `Role`
constants and one `Node` struct. It is not a gap.

## 2. Gap one: a CI gate with no gate

`.github/workflows/nightly-bench.yml` fails or passes the M1 milestone on the exit status of
`scripts/v2-gate-check.sh`. Nothing tested that script. The script is float comparison,
median selection, and awk column arithmetic — the same shape of code that fails silently
when its input format shifts. It also documents its own fear of exactly that:

> `# an empty window would pass every budget below by having nothing in it`

The author guarded the frame count and left the millisecond fields unguarded. Round 2 showed
the first guard was weaker than it read (defect 5 below).

`test/gatecheck/gatecheck_test.go` (new, 25 cases across 21 top-level tests) is the contract
for that script. It is deterministic — it only reads files and compares numbers — so it
belongs in PR CI, and because CI runs `go test ./...` over `./test/...` it is already in
there. It skips cleanly where `bash` or `jq` is missing, so it stays green on the ubuntu
runners' terms.

### Red → Green

Nine defects failed against the script as found, in two rounds. Every one is the same
failure class: **the gate opens when it cannot read its input, or fails with the wrong
status.** Round 1 was found by writing the cases; round 2 was found by an adversarial QA
pass over round 1's own fix.

Round 1, four defects:

1. **`TestMissingGatedMetricFailsClosed`** — an artifact with no `report.mean_ms` produced
   `mean_ms 0.000 ok, budget 16 ms` and `PASS`. Confirmed by hand before encoding it:
   a report from any build that stopped emitting the timing fields passes the nightly
   timings by having nothing to measure, which is the empty-window case one level down.
2. **`TestNonNumericMetricFailsClosed`** — `"mean_ms":"n/a"` behaved identically.
3. **`TestBenchMissingBenchmarkFailsClosed`** — when the named benchmark is absent,
   `bench_median` ran its even branch over zero records, averaging two unset awk elements
   to `0`. The `-n "$got" || die "the benchmark did not run"` guard below it could not
   fire, because `0` is not empty. A benchmark deleted from `test/gate` would have been
   reported as `BenchmarkWarmScroll 0.000 ok`.
4. **`TestHelpExitsZero`** — `-h`/`--help` is handled in the argument loop, which only runs
   after `$1` has been consumed as the artifact path, so `v2-gate-check.sh -h` reported
   `cannot read -h`.

Round 2, five more, found by attacking round 1's fix rather than the original script:

5. **`TestNonPositiveFrameCountExitsTwo`** — the frame-count guard the header brags about
   compared against the *string* `"0"`. JSON writes a counted zero as `0.0` as happily as
   `0`, and `-1` and `0.5` are not zero at all, so the empty-window case the author guarded
   was still open. Now a `^[1-9][0-9]*$` match.
6. **`TestNonNumericBudgetExitsTwo`** — `-present-p99 ''` is indistinguishable, downstream,
   from "the flag was never passed", so passing an empty budget silently switched the gate
   off. A dangling `-mean` with no value fell out of the argument loop as exit 1 (budget
   missed) rather than exit 2 (bad invocation). Both now go through `assert_budget`.
7. **`TestNegativeMetricExitsTwo`** — the `gated` regex *I wrote in round 1* allowed a
   leading `-`, so `"style_passes":-5` satisfied the spec's "zero style passes" check.
   The fix for a fail-open bug was itself a fail-open bug.
8. **`TestWrongJSONShapeExitsTwo`** — `jq -e .` accepts `[]` and `{"report":"gone"}`; every
   read then failed inside `jq` and leaked `jq`'s exit status (5) as the gate's verdict. A
   `.report | type == "object"` check keeps a structurally wrong artifact an exit 2.
9. **`TestNonNumericTileCountersExitsTwo`** — `"tiles_rasterized":"a b"` reached bash
   arithmetic and died with exit 1. The tile counters are now read through `gated` too.

Fixes, all minimal, in `scripts/v2-gate-check.sh`: a `gated` helper that refuses an absent,
negative, or non-numeric value on any field a budget or a counter check measures against
(`mean_ms`, `p99_ms`, `style_passes`, `layout_passes`, the tile counters, and `present_p99_ms`
when it is gated); `assert_budget` on every millisecond flag; `bench_median` exits without
printing on an empty transcript so the existing "did not run" guard sees it; help recognised
before the artifact is consumed; the `.report` shape check. Header updated to state that "bad
artifact" includes a missing metric, and to point at `test/gatecheck` as the contract.

### Verified against real output, not fixtures

Fixing a parser against fixtures written by the same author who got the parser wrong proves
nothing, so the fix was re-run on live output after each round:

```
go run ./cmd/goosie -gate -backend headless -frames 120 -out /tmp/real-gate.json
go test -run='^$' -bench='BenchmarkWarmScroll|BenchmarkColdScrollBurst' \
  -benchmem -benchtime=20x -count=3 ./test/gate > /tmp/real-bench.txt
scripts/v2-gate-check.sh /tmp/real-gate.json -mean 16 -p99 33 -present-p99 12 \
  -bench /tmp/real-bench.txt -warm-ms 5 -cold-ms 8
```

The live artifact carries full-precision floats (`"mean_ms":7.140157575`), not the three-digit
shape the fixtures use, and the guards accept it. The final run reported `mean_ms 7.140`,
`p99_ms 16.608`, zero style and layout passes, `present_p99_ms 4.101` against the gate's own
budget, and true medians of 1.753 and 2.592 ms/op out of six real bench lines — PASS, exit 0.
`warm allocs/op` came back `0`, which is what the real line says. The ten-field layout the
script counts back from is the layout `go test -bench` emits; the first draft of the fixtures
did not match it, and the resulting failures were fixture bugs, not script bugs, and were
corrected before the script was touched.

The important part of that run is the line that round 1 would have printed nothing for:
`-present-p99 12` now gates. Before round 2, supplying it or not was indistinguishable.

Green, verified: `go test -count=1 ./...` (28 packages with tests, 0 failures),
`go test -race ./test/gatecheck ./cmd/goosie`, `go test -count=5 ./test/gatecheck`,
`go test ./internal/archtest/`, `gofmt -l` on both touched Go files,
`go vet ./test/gatecheck ./cmd/goosie` (scoped — repo-wide vet is not clean, see §4),
`bash -n scripts/v2-gate-check.sh`.
Total coverage of `internal` statements is unchanged at 75.1%, which is the point: this round
bought nine defects found in code CI depended on, not a number in a column.

## 3. Gap two: TabManager against the frame path

The brief named this integration specifically, and it was the one place a real contract was
guarded by a fake test. `TestTabSerialPreventsStaleResults` asserted that assigning
`tab.Nav.Serial = 2` made `tab.Nav.Serial` equal `2`. That is a property of the Go
assignment operator, not of the browser; it could not have failed for any product bug.

`internal/tabs/manager.go` itself is at 100% function coverage, so the missing link is not
the manager's unit behaviour but what the host does with it. `applyNavResult`
(`cmd/goosie/main.go:504`) is where a navigation result meets the tab it may or may not
belong to, and the staleness rule — a superseded result must not paint, must not clear a
loading flag, and must not steal the document — lived only in a comment.

Four tests in `cmd/goosie/tab_lifecycle_test.go` now hold that rule, driving the real
`tabs.TabManager`, a real `paint.BuildLayer`, and the real scheduler through the same path
the binary uses:

- `TestStaleNavResultCannotPaintOverTheCurrentOne` — serial 5 against tab serial 9 leaves the
  URL, the layer, `PlanStats().Published`, and both loading flags untouched.
- `TestStaleNavErrorDoesNotSurface` — a superseded failure must not become the active tab's
  error banner.
- `TestCurrentNavResultIsApplied` — the positive control. Without it the two tests above pass
  on a host that ignores every result.
- `TestBackgroundTabResultLeavesTheForegroundAlone` — a background tab finishing must not
  republish the foreground's frame.

All four pass against the product as found, so they are a regression net rather than a Red
detection — but they replaced a test that was also always green and proved nothing. Their
worth was checked by mutation rather than claimed: disabling the guard at
`cmd/goosie/main.go:538` fails `TestStaleNavResultCannotPaintOverTheCurrentOne` on all five of
its assertions (URL taken over, layer replaced, `Published = 1`, and both loading flags
cleared) and `TestStaleNavErrorDoesNotSurface`, while the other 22 tests in the package stay
green. Deleting the guard is therefore a PR-blocking change instead of a page that
occasionally paints the previous document over the new one. The guard and the source were
restored after the check; `git diff cmd/goosie/main.go` is empty.

## 4. Backlog, by role

Ordered by what a defect there would cost, not by what is easiest to write.

**QA lead**
- Define acceptance criteria for a `p95` budget. The artifact already carries `p50_ms` and
  `p99_ms`; the brief's "p50/p95 thresholds" do not exist as such — CI gates mean and p99.
  Decide whether p95 is a real requirement or the brief was approximate, then write it down
  in the spec so the next reader does not re-litigate it.
- Own `test/gatecheck` as the reference for what "a deterministic test of CI tooling" means:
  fixtures generated to the tool's real output shape, verified against live output.

**Implementation**
- `test/gatecheck`: gate the remaining reported-only fields (`max_ms`, `present_mean_ms`) if
  a missing one is ever used for anything but display. Deliberately not gated now.
- Wire `v2-gate-check.sh` into a macOS PR job against a *cached* artifact, so a change that
  breaks the script's parsing is caught on the PR rather than on the next nightly.
- Roadmap gate 6 (JS runtime, Goja per the chosen ADR) needs its test cases written before
  the runtime lands. That is the next module where Red-Green has real leverage, and it is
  large enough to be its own plan.

**Refactoring**
- ~~`internal/style/style.go` has the largest uncovered surface: 16 functions at 0%.~~ **Closed and
  re-measured in §21.** `go test -coverpkg=./internal/style ./...` now reports **83.4 %** of
  statements with **not one of the 104 functions at 0.0 %**. Two of the original sixteen were the
  font-registry setters, which §21 deleted rather than tested; the rest were reached by the
  fixture-per-property table batch. What is left is branch-level, ranked by
  `go tool cover -func`: `parseGridTemplateShorthand` 35.7 %, `resolveCurrentColor` 36.2 %,
  `gradientKeywordAngle` 37.0 %, `parseFlexDirection` 40.0 %, `parseBackgroundColor` 47.8 %. Those
  are gap-hunting jobs, not the "CI has never executed `border: 1px solid red`" job this bullet
  described, and they are worth less now than the write-up they came from.
- No restructuring was warranted in the gate script: the fixes are nine small guards, and the
  argument loop and check/note helpers already separate the concerns a rewrite would.
- `go vet ./...` is not clean repo-wide and was already not clean: `test/style/gradient_test.go`
  has unkeyed `css.Color` struct literals. Pre-existing, outside this work, and a one-line fix
  per site — worth doing alongside the `style.go` parser batch above rather than as its own
  change.

**Documentation**
- This file records the tier map, the fail-open class, and the now-tested staleness contract.
- The staleness tests align to the design rather than to the code: `2026-09-20-tabbed-browser.md`
  lines 986-1019 specify that a result whose serial no longer matches is dropped. The tests
  assert that specification, so a future refactor that "simplifies" `applyNavResult` by
  deleting the check fails against the plan's intent, not against an accident of
  implementation.
- No design doc or ADR needed changing: nothing here adds a capability. The gate-6 runtime ADR
  is unaffected.

## 5. Deferred, with reasons

- **Timing gates in PR CI.** The script's header argues a flaky gate is ignored within a
  month, and that stands. Only the deterministic parsing tests were added to the PR path.
- **A second batch of `TabManager` and network-cache *unit* tests.** Both modules are at 100%
  function coverage; new cases there would restate existing ones. The integration tier that
  was missing is now covered (§3).
- **Coverage for `internal/ax`.** Data declarations only. It earns tests when it grows
  behaviour, and the accessibility tree's behaviour is tested where it lives, in
  `internal/engine/accessibility.go` (5 functions, 100%).
- **`p50`/`p95` budget flags.** Blocked on the QA-lead decision in §4 about whether p95 is a
  requirement at all.
- **Gating `max_ms` and `present_mean_ms`.** Reported for diagnosis, and a missing one is now
  caught by the shape check, so making them budgets is a policy call rather than a gap.

## 6. How to re-verify

```bash
go test ./test/gatecheck/ ./cmd/goosie/        # the two touched packages
go test -count=5 ./test/gatecheck/             # determinism, not caching
go test -race ./...                            # the new tests share a scheduler
go test ./internal/archtest/                   # import rules, cgo still in one directory
scripts/v2-gate-check.sh /tmp/gate.json -mean 16 -p99 33   # the nightly path
```

To see the staleness tests earn their keep, change the guard at `cmd/goosie/main.go:538` to
`if false && tab.Nav.Serial != result.serial {`, run `go test ./cmd/goosie/`, and restore it.
Two tests fail; nothing else in the package does. That mutation was run once, on 2026-09-26,
and the result recorded in §3.

## 7. Second round: the `internal/style` parser batch (§4 named it; this is that work)

Written the same day, against the state §1-§3 left behind. The batch is
`test/style/property_longhands_test.go`: 15 tests driving the longhands that had never run
in CI through `style.Resolve`, plus a 1013-case hostile-value sweep. `test/style`'s coverage
of `internal/style` statements went 52.3% → 72.4%, and the functions at 0% in that package
went 20 → 4.

The claim in §4 that these parsers were "untested, not dead" was right for most of them and
**wrong for two**: `applyInlineStyle` and `applyDeclarations` have no caller anywhere outside
each other. Inline `style="…"` attributes are parsed by `parseInlineDeclarations` and applied
by `applyCascadeEntry` (`internal/style/style.go:746`), which is the path the new inline tests
exercise. The two orphans are a second, unreachable implementation of that. Deleting them is
not part of this round because engine code should go in a change that says so; they are
flagged here instead. Coverage now proves they are never reached.

### Two product defects, both Red first

1. **`background-repeat` crashed the style pass on an empty value.**
   `parseBackgroundRepeat` read `strings.Fields(v)[0]`. A declaration with nothing after the
   colon - `.x { background-repeat: }` or `style="background-repeat:"`, both of which a
   hand-written or truncated sheet can carry - gives `Fields` a zero-length slice, so
   `computeStyle` panicked with `index out of range [0] with length 0`. That is a page-triggered
   renderer crash, exactly the "safe handling of malformed input" the brief asks the security
   review to hold. Now an empty value keeps the initial `repeat`.
2. **`border: 1px solid rgb(0, 136, 238)` lost the colour.** `parseBorderShorthand` split the
   value with `strings.Fields`, so a function the author spaced after its commas arrived as
   `rgb(0,` `136,` `238)` - none of which parses as a colour, and the last of which was read as
   a width. Borders on such pages drew in the initial colour instead of the declared one. It
   now splits with `splitColorTokens`, the parenthesis-aware splitter `border-color` already
   used for exactly this reason; the two properties no longer disagree about what a token is.

Both were confirmed red against the unmodified script before being fixed, and green after -
not inferred from reading the diff.

### What the sweep covers, and how it reports

`TestMalformedPropertyValuesNeverPanic` feeds 45 properties x 21 hostile values through both
routes (a matched rule and a `style` attribute). Each combination recovers its own panic
rather than letting the testing package repanic it, because a repanic kills the binary and the
first crash would hide every property after it - which is how defect 1 was found the hard way.

### Verified against renders, not only assertions

Defect 2 changes what reaches the screen, so the parity harness ran for both binaries
(`HEAD` in a detached worktree, then the fix) over every `testdata/render` fixture against the
cached Chromium references. The first A/B was invalid: a second run was launched into the same
output directory while the first was still writing it, so its "fixed" leg scored 52 fixtures
rather than 56 and the two averages were not comparable. The clean re-run is:

```
baseline (HEAD)  55/56 passing (98.2%), average 95.02%
fixed            55/56 passing (98.2%), average 95.02%   # identical, per-fixture
```

No fixture regressed. The fix is also not expected to change any score: no fixture declares a
border colour with spaces after the commas, which is the only input whose reading changed. The
one failure, `semantic/03-aside-figure` at 59.07%, is pre-existing and unchanged.

```bash
go test -count=1 ./test/style/ ./internal/style/   # the batch
go test -count=1 ./...                             # whole suite
go test -race -count=1 ./test/style/ ./test/engine/
go vet ./...                                       # clean as of this round, see below
```

§4's `go vet` refactor item is done: the five unkeyed `css.Color` literals in
`test/style/gradient_test.go` and `test/style/presentational_hint_test.go` are keyed, and
`go vet ./...` is now clean repo-wide. Still open and pre-existing: `gofmt -l` flags
`internal/style/clip_test.go`, `internal/style/style.go`, and `test/style/fontfamily_test.go`
for struct-field alignment in regions no test touches.


## 8. Third round: the memory and cold-start SLA, measured before it was gated

The brief's Performance & Profiling rule is "flag any regression where heap allocation exceeds
50MB or paint pipeline drops below 60fps". The fps half has had a home since M1 - `test/gate`
for the counters, `scripts/v2-gate-check.sh` for the wall-clock budgets. The memory half had no
measurement at all, and this is not an oversight the earlier rounds missed: `docs/roadmap-v2.md`
and the 2026-09-23 plan each say outright that the work done so far "does not establish a
total-process RSS/CPU bound". `test/gate` proves the frame path allocates *nothing per frame*,
which is a stronger claim about the frame path and a weaker one about the process: it says nothing
about what the caches hold while they are not allocating.

So the first action was to measure rather than to assert a number. All of it below is this
machine (Apple silicon, headless backend, 2026-09-26), and the runs that produced timings overlapped
with the parity sweep, so treat the millisecond figures as an upper bound.

| metric | value |
| --- | --- |
| retained Go heap, warm gate scene at rest | 210.55 MiB |
| - of which configured caches account for | 207.10 MiB (128 tile budget + 4 device surfaces x 19.78) |
| - residual (glyph atlas, scene, layer records) | 3.45 MiB |
| growth across 600 warm scroll frames | 0.00 MiB (0.01 under `-race`) |
| process peak RSS, 800x600 @ dpr 1 | 84.6 MiB |
| process peak RSS, 1440x900 @ dpr 2 | 245.2 MiB |
| spawn to first present, 800x600 @ dpr 1 | median 45 ms (min 38, max 54) |
| spawn to first present, 1440x900 @ dpr 2 | median 56 ms (min 49, max 64) |

**Cold start passes with ~18x headroom, and is now a measured quantity.** The artifact already
carried absolute `presented_at` stamps per frame, so the number needs no product code: spawn the
binary, subtract the spawn epoch from `frames[0].presented_at`. That is what the figures above are,
and it is the shape a nightly step would take.

**"<50MB idle" is not met at the gate's own geometry, and cannot be.** The tile cache is configured
at 128 MiB there (1440x900 CSS at DPR 2, a document-wide cache), so a 50 MB assertion would fail on
a pipeline that is behaving exactly as designed. Whether the target is met is therefore a question
about configuration - cache budget x viewport x ring depth - not about a defect. That makes it a
Product & Scope call, recorded here rather than resolved by picking a threshold that would pass.

### What the gate now holds

`test/gate/footprint_gate_test.go`, three tests, in CI rather than the nightly because they assert
no wall-clock quantity:

1. `TestFootprintAtRestIsBoundedByConfiguredCaches` - retained heap <= the caches the harness asked
   for + 12 MiB slack (measured residual is 3.45; one extra device surface is 19.78, so the slack is
   room for the atlas to grow and not room for a bitmap to appear). It also fails if the warm scene
   is not actually holding all 504 tiles, because a path that retained nothing would satisfy the
   upper bound trivially.
2. `TestFootprintDoesNotGrowAcrossWarmSweep` - 600 scroll frames may move retained heap by <= 4 MiB,
   which still catches a path holding ~7 KiB per frame.
3. `TestFootprintInstrumentDetectsRetention` - the same non-vacuity guard
   `TestGate_AllocationInstrumentIsNotVacuous` gives the allocation criterion: 32 MiB deliberately
   held across a sweep is reported as +32.00 MiB, so criterion 2 is shown to see what it claims.

`gateComposerBitmaps` replaces the composer pool's inline `2` in `newHarness`; the bound in test 1
names that capacity, so it cannot silently drift from the harness it is measuring.

```bash
go test -count=1 ./test/gate -run TestFootprint -v   # 210.55 / +0.00 / +32.00
go test -race -count=1 ./test/gate -run TestFootprint
```

### Deferred, with reasons

- **A nightly step for peak RSS and cold start.** `.github/workflows/nightly-bench.yml` is the right
  home and both numbers are already obtainable (`/usr/bin/time -l` on a prebuilt binary - not on
  `go run`, whose compile would dominate the reading - and the artifact's first stamp). Left out of
  this round because it is a second deliverable, and a workflow change cannot be verified here
  without pushing a branch. *Closed for cold start by §15; peak RSS is still open. Note that the
  45 ms and 56 ms figures above are headless readings - §15 measures the same quantity through a
  real window and gets 179-215 ms.*
- **Heap fields in the gate artifact.** `FrameRecorder` writes the JSON and owns nothing that knows
  about `runtime.MemStats`; `cmd/goosie/report.go` already prints environment lines to stderr and is
  the honest owner. Adding it means choosing between widening the recorder's remit or splitting the
  artifact, which is a design decision rather than a test.
- **An absolute idle-heap budget at a small viewport.** Blocked on the product decision above; the
  measurement is a one-liner once the target geometry and cache budget are named.
- **`pprof` capture alongside the benchmarks** - the brief names it, and `-benchmem` totals already
  cover the regression class it would find first.

## 9. Fourth round: the §E security clauses, measured before they were claimed

The brief's §E asks for four things: races, unclosed response bodies, oversized payloads, and SSRF
prevention. Three were already carried by the code and its tests. One was not carried at all, and this
round fixes that rather than re-describing it.

A note on method, because it changed the answer: the first pass was a delegated audit whose quoted
snippets named a module path that is not this repository (`github.com/Vyquocvu/dev/goopie/internal/net`).
Nothing below is taken from that report. Every claim was re-read from the files named.

### What was already in place

| Clause | Where | Evidence it holds |
| --- | --- | --- |
| Oversized payload | `internal/net/http.go:23` `MaxResponseBytes = 8<<20`, `readBounded` at `:460` | `test/net/http_test.go:231` exercises declared-length, chunked **and gzip** bodies over the cap; transparent gunzip means the bound is on *decoded* bytes, which is what a decompression bomb needs |
| Local file confinement | `readFileURL` at `:471` (stat → nonblocking open → re-`Stat` the descriptor) | Closes the TOCTOU that an opening-only check would leave |
| Scheme and redirect confinement | `NormalizeURL` at `:170`, `checkRedirect` at `:325` | Every hop re-validated; ≤10 hops; credentials and opaque URLs refused |
| Subresource schemes | `internal/engine/limits.go:644` `resolveSheetURL` | `file:` and every other non-http(s) scheme is refused for sheets, images, fonts and `@import`, so a page cannot read the local disk |
| Mixed content | same function, `b.Scheme == "https" && u.Scheme == "http"` | `test/engine/mixed_content_test.go` |
| Unclosed response bodies | `internal/net/http.go:396` | The only raw `*http.Response` in the tree is closed by a `defer` set before any early return, and everything above it receives `[]byte`. A body leak is now structurally impossible rather than review-caught |
| Races | `go test -race` | `test/engine`, `test/entrypoints`, `test/net`, `test/gate` green under `-race -count=1` |

### The gap: no private-network confinement

`internal/net` validates the *shape* of a host and never its *scope*: `127.0.0.1:8080` and `[::1]:8080`
are in its accepted table, and nothing distinguished a public page pulling a subresource from
`http://169.254.169.254/` or `http://192.168.1.1/admin` — the viewer's own router, an internal admin
panel, a cloud metadata endpoint. A page on the public internet could turn the browser into a client
against whatever network the viewer is sitting on.

**What was built:** the rule is about *who is asking*, not about the address. `isLocalAuthority`
(`internal/engine/limits.go`) recognises loopback, RFC 1918 / 4193 private space, link-local (where
169.254.169.254 lives), link- and interface-scoped multicast, the unspecified address, `0.0.0.0/8`,
CGNAT `100.64.0.0/10`, and the RFC 6761 special-use names (`localhost`, `.local`, `.internal`,
`.home.arpa`). `resolveSheetURL` refuses such a target unless the initiator is itself local, where
local means a loopback or private-network origin, or a `file://` document the viewer opened. It is one
clause in the single function every subresource already funnels through, so sheets, images, fonts and
`@import` are all covered by it.

This is Chrome's Private Network Access split, and for the same reason: a blocklist of addresses would
break local development, which is the main thing a browser is pointed at on `127.0.0.1`.

**Why the policy sits in `engine` and not `net`:** `internal/archtest/rules.go:54` lets `engine` import
`internal/net` not at all, and that edge is worth keeping absent — the engine is deliberately
transport-agnostic, taking fetchers as function values. The engine is also the only place that knows a
URL is a *subresource* and what asked for it. The classifier uses stdlib `net` only.

### Tests

`test/engine/private_network_test.go`, written first and watched fail (every case fetched once, want 0):

- 20 local authorities × both schemes as an `<img>` subresource of a public page — refused.
- The same 20 as a `<link rel=stylesheet>` — refused.
- `TestLocalPageCanReachLocalNetwork` — a loopback page, a private page, and a `file://` document may
  all still load loopback assets.
- `TestPublicPageCanReachPublicNetwork` — non-vacuity: hostnames, bare domains and a public numeric
  address (`93.184.216.34`) must still load. Without it, "refuse everything" would pass this file.

The blocked-table cases deliberately use an `http://` public base. Under an `https://` base an `http://`
subresource is already refused as mixed content, and a test that passed for that reason would not have
been testing this rule.

### Verification

```bash
go test -count=1 ./...                                    # 50 packages, all ok
# the new file alone: 72 passing subtests (20 refused hosts x 2 schemes, 20 stylesheet cases, 4+4 allowed)
go test -race -count=1 ./test/engine ./test/entrypoints ./test/net ./test/gate
go vet ./...                                              # clean
gofmt -l internal/engine/limits.go test/engine/private_network_test.go
python3 testdata/parity.py --render --binary <built>       # 55/56, average 95.02% — unchanged
```

### Not covered, with reasons

- **DNS rebinding.** A public hostname that resolves to `127.0.0.1` is classified as public and loads.
  Fixing it needs the address resolved and pinned at dial time, which means `internal/net` owning a
  `DialContext` with its own resolver — a transport change, not a URL-resolution one. The classifier's
  comment says this out loud so the next reader does not assume otherwise.
- **Top-level navigation is not restricted.** The user typing `http://192.168.1.1` is the browser's
  purpose, not an attack on it. Chrome behaves the same way; recorded so it reads as a decision.
- **Response header bound.** `internal/net` used `http.DefaultTransport`'s client limit, which is
  10 MiB (`net/http/transport.go:333`, Go 1.26.5). Browsers cap far lower. That is closed in §10;
  the reason it was not done here was the inheritance risk in replacing the transport, not the size
  of the constant.
- `.github/workflows/security.yml` is still `go vet` plus the race suite; it has no security-specific
  job. The new tests run in the ordinary suite, so they are gated like everything else.

## 10. Fifth round: the response-header bound §9 left open

§9 named the gap and did not close it: one response's header block was bounded only by Go's
transport default of 10 MiB (`net/http/transport.go:333`), buffered before the first body byte, so
the 8 MiB body bound that `readBounded` enforces applies to a payload a server can grow into an
order of magnitude larger than that by header bytes alone.

### Where the number came from

No browser limit was cited, because none was measured. The cap was sized from this project's own
live corpus instead: `testdata/urls.txt`, twenty sites, fetched with the repo's Chrome UA, header
block measured per response including redirect hops.

| measurement | value |
| --- | --- |
| worst (Wikipedia, with its redirect hop) | 6 418 B |
| median | 454 B |
| sites that answered | 19 of 20 |

`MaxResponseHeaderBytes = 256 << 10` is forty times the worst real header block seen and a fortieth
of the default being replaced. That is the whole argument for the number: it is generous to real
sites and 40 000× tighter than the default for a hostile one.

### The change, and why it is a `Clone`

`clientTransport()` (`internal/net/http.go:279`) is `http.DefaultTransport.(*http.Transport).Clone()`
with the bound set on top, wired into all three production constructors. It is a `Clone` rather than
a literal `Transport{}` because the literal silently drops what the shared transport carries:
`Proxy: http.ProxyFromEnvironment`, the dialer and connection pooling, and the HTTP/2 settings. That
inheritance risk is exactly why §9 deferred the change; `Clone` removes it instead of accepting it.

One consequence worth knowing: `MaxResponseHeaderBytes` also seeds HTTP/2's advertised
`MaxHeaderListSize` (`transport.go:447`), so the bound is sent to the server rather than only
enforced locally.

### Tests

`test/net/http_test.go`, both Red before the constant existed:

- `TestGetRejectsOversizedResponseHeaders` - `MaxResponseHeaderBytes/1024 + 2` fields of 1 023 bytes
  each. Failed as `accepted a header block over 262144 bytes` before the transport was wired.
- `TestGetAcceptsLargeResponseHeadersUnderTheCap` - 250 such fields (250 KiB, just under), and the
  joined `X-Pad` header must arrive at exactly `fields*1023 + (fields-1)*len(", ")` bytes with the
  body intact. This is the non-vacuity half: a cap that refused everything would pass the first test.

### Still open from §E

DNS rebinding (§9), now closed in §12. Nothing else in §E is unmeasured now: body bound including
gzip, file-URL TOCTOU, redirect and URL-shape validation, scheme confinement for subresources, mixed
content, private-network confinement, header bound, body closure, `-race`.

An earlier draft of this section also listed "a security-specific CI job" as open. That was wrong:
`.github/workflows/security.yml` has existed the whole time and runs `go vet -all ./...` plus
`go test -race ./...` on every push and PR to `main` and on a weekly cron. The claim was written from
memory of the §E checklist rather than from the workflows directory. §13 found the real release-path
gap, which is a different one.

## 11. The warm-scroll gate's stray allocation, reproduced before it was explained

The first full-suite run of this round failed `TestGate_WarmScrollZeroAllocsZeroTiles` with
`1 allocations (32 bytes) across 600 frames, which averaged 0 allocs/op`. The message is the bug as
much as the failure is: a single allocation across 600 frames is the opposite of per-frame work.

### What the measurements said

| how it was run | result |
| --- | --- |
| that one test, 20 counts, alone | 1 failure |
| whole `test/gate` package, 8 counts | 0 failures |
| full `go test ./...`, 1 run | 1 failure |
| same test and `TestM3_WarmScrollZeroAllocs`, 40 counts after the fix | 0 failures |

Same size every time (32 B), same count every time (1), never in two consecutive passes over a
scroll path that is entirely deterministic. A frame path that allocated per frame would have
reported ~600, and it would have reported it twenty times out of twenty.

### The root cause is the instrument, not the frame path

`allocTotals` reads `runtime.MemStats`, whose `Mallocs` is the **whole process's** counter, and the
file already knows this: it pins `GOMAXPROCS` to 1 around the samples and turns the collector off
during them, precisely because a GC cycle started by an earlier test's 126 MB of tiles puts mark
workers' and span bookkeeping into the window. The comment claimed that made the warm criterion
safe, on the reasoning that a warm sweep submits no jobs so nothing else is awake to allocate.

That reasoning is what the data falsifies. Nothing being awake is not the same as the runtime being
silent, and `SetGCPercent(-1)` lowers the rate of those strays without removing them. One dirty
window therefore says nothing about *which* of the two made the allocation.

### The fix is the rule the cold burst already used

`assertBurstAllocatesNothing` has measured twice from the start and failed only when a freshly built
frame path reproduced the allocations, reporting the tolerated strays in a `t.Log`. The warm
criterion now argues the same way: `assertZeroAllocs` re-measures a dirty window on the same already
warm path, and `zeroAllocVerdict` - extracted as a pure function so the rule itself is testable -
fails only when both windows report allocations, because a per-frame allocation is exactly the thing
that recurs. A tolerated stray is `t.Log`ged with both numbers, not hidden.

`test/gate/alloc_verdict_test.go` holds the rule's four cases (clean, stray-then-clean, both dirty,
one-per-window-both) plus `TestZeroAllocVerdictCatchesPerFrameLeak`, which drives a closure that
allocates on every frame through the real instrument and requires the verdict to fail it. That is
the non-vacuity check for the loosened rule: it proves the two-window argument is a re-measurement,
not an amnesty. `TestGate_AllocationInstrumentIsNotVacuous` already proves the instrument sees both
a per-frame allocator and a one-off.

### One trap worth recording

The new file was first named `alloc_windows_test.go`. Go reads the `_windows` suffix as a
`GOOS=windows` constraint, so the tests silently did not compile: `go test -run TestZeroAllocVerdict`
answered `no tests to run` and `go test ./test/gate -list '.*'` did not list them. A test file that
never builds is a test that passes. Naming it `alloc_verdict_test.go` was the whole fix for that, and
`-list` is how to check for it in future.

## 12. Sixth round: DNS rebinding, the half §9 could only describe

§9 built private-network confinement and knew, in writing, that it was half a rule. The comment on
`isLocalAuthority` said the missing part outright: a hostname that is not special-use reads as
public even when DNS sends it to a private address. So a page refused for naming `169.254.169.254`
in an `href` is served by the same browser once the attacker registers `rebinding.example` with an
A record of `127.0.0.1` - or simply changes the record between the check and the connect. Resolution
time has no address to look at. The only moment that does is the dial.

### Why the guard is split across three packages, which is why it looks odd

The predicate belongs to `internal/engine` and the dial belongs to `internal/net`, and the import
table forbids either direction: `internal/net` imports nothing internal at all (which is what keeps
it substitutable in engine tests), and `internal/engine` must not reach the transport. A fourth
option - one new shared package holding both - would have made `internal/net` depend on internal
code for the first time.

So `internal/net` gained a *policy-free* mechanism and the join lives in `cmd/goosie`, the one
package where both halves are visible:

- `internal/net/address_guard.go`: `AddressGuard{Blocked, Lookup}` carried on the request context,
  and `dialAddressGuarded` installed as the transport's `DialContext`. No CIDR list, no notion of a
  document. A request with no guard dials exactly as before, so typing a URL to one's own router is
  unaffected.
- `internal/engine/limits.go`: `AddressBlocked(ip)` is the address half of `isLocalAuthority`
  (which now calls it, so the two rules are one list), and `InitiatorIsLocal(raw)` is the
  who-is-asking question that `resolveSheetURL` already answered about names.
- `cmd/goosie/main.go`: `subresourceCtx(ctx, base)` installs the guard on the three subresource
  closures - the stylesheet linker, the image fetcher, the font fetcher - and only when `base`, the
  document or sheet that asked, is public. The document fetch itself is left unguarded.

`clientTransport()` is the single place the dialer is set, and all three client constructors go
through it, so there is no path that silently keeps the standard dialer. The replacement dialer
copies `http.DefaultTransport`'s dialer literal (30s timeout, 30s keepalive) rather than inventing
one, and the transport is still a `Clone()` so `ForceAttemptHTTP2` survives - replacing
`DialContext` on a hand-built `Transport` would have quietly dropped HTTP/2 with it.

### Three properties, and the test that pins each one

`test/net/address_guard_test.go` drives a real `httptest` server on loopback addressed by a name no
local-address rule would refuse (`http://rebind.example:<port>`):

| property | test | what would otherwise drift |
| --- | --- | --- |
| a refusal is a refusal | `TestAddressGuardRefusesAHostThatResolvesToABlockedAddress` - `errors.Is(err, ErrBlockedAddress)`, 0 hits, 1 lookup | folding it into a dial error, which makes "this browser said no" indistinguishable from "the server did not answer" |
| the checked address is the dialed address | `TestAddressGuardDialsTheAddressItValidated` - exactly one lookup, and the server still sees `Host: rebind.example` | resolve, check, then dial the *name*: a second resolution, i.e. the same rebinding arriving as a time-of-check gap |
| skipping works | `TestAddressGuardSkipsABlockedAddressAndDialsTheNextOne` - `[::1, 127.0.0.1]` with IPv6 refused still connects | a guard that refuses the first answer and then refuses the connection, which is a browser that cannot reach a dual-stack host |

The rest are the edges that make the mechanism honest: a literal address is checked without
resolving (`TestAddressGuardChecksALiteralAddressWithoutResolving`, 0 lookups); no guard dials
normally (`TestAddressGuardWithoutAGuardDialsNormally`, the control for every case above); a name
that resolves to nothing is a *resolution* failure and not `ErrBlockedAddress`
(`TestAddressGuardRefusesANameThatResolvesToNothing`), because nothing was refused on policy grounds
and a log that conflates the two reports an NXDOMAIN as an attack.

`TestAddressGuardKeepsTheNameForTLS` is the one this round could have broken by building the fix:
pinning moves the connection, and HTTPS needs the name twice - SNI and the certificate's expected
identity. It asserts the TLS server was introduced to `rebind.example` while the guarded path dialed
the validated `127.0.0.1`. It uses `InsecureSkipVerify` and says so in the comment: the subject is
the name the peer saw, not the trust decision.

The two halves are then tied together where they are actually joined.
`test/engine/address_blocked_test.go` runs every address in the existing `localAuthorities` table
through `engine.AddressBlocked` and requires at least ten of them to be addresses, so the resolution
rule and the dial rule cannot drift apart silently and the comparison cannot be a handful of cases.
`cmd/goosie/address_guard_test.go` asserts the wiring itself: that the sheet fetch of a public page
arrives guarded, that the guard's predicate behaves like the engine's rule on
`169.254.169.254`/`127.0.0.1`/`10.0.0.5` and not on `93.184.216.34`/`8.8.8.8`, that `Lookup` stays
nil (overriding it in production code would replace DNS with a fixture), that the document fetch is
*not* guarded, and - `TestLocalPageSubresourcesAreNotGuarded` - that a loopback page's own loopback
sheet is still not guarded. The fixture fails if `/style.css` was never fetched, so none of this can
pass by testing an absent request.

### Why the tests inject the resolver

There is no test here that performs a real rebinding, because a real one needs either `/etc/hosts`
or a DNS record this repository does not control; `/etc/hosts` was not touched. `AddressGuard.Lookup`
exists for exactly this: it is the smallest hook that lets a test name an address without changing a
system, and it doubles as the seam a future `--host-resolver-rules` style setting would use. What
the injection does not prove is that the *system* resolver is reached correctly in production, which
is why the wiring test asserts `Lookup == nil` rather than assuming the default is right.

### Known limits, recorded rather than smoothed over

- **Proxy.** With a proxy configured the transport dials the proxy, so `addr` names the proxy and
  the guard judges that address, not the origin. The comment on `WithAddressGuard` says so; browsers
  with the same arrangement inherit the same caveat.
- **Response cache.** `DefaultClient`'s cache is keyed by normalized URL and consulted before
  dialing, so a URL the viewer themselves loaded unguarded can be served from cache to a later
  guarded subresource fetch of the same URL. It requires the viewer's own prior visit, and the cache
  is per-client, so this is not a page-reachable path; left as is and noted here.
- **Document-level navigation stays unguarded**, deliberately: that is how a browser reaches its own
  router, and it matches what §9 chose for names.
- **`cmd/dump-url`** wires the same three closures without the guard. It is a developer dump tool
  that is not pointed at untrusted pages; wiring it would be right if it ever moved into the binary's
  path.

### Verification

`go vet ./...` clean. `go test ./...` clean, including `internal/archtest`, which is what proves the
new `engine`-exported predicate did not become an import edge. `go test -race -count=1` clean on
`test/net` (6.5s), `test/engine` (3.5s), `cmd/goosie` (2.8s) and `internal/archtest` (6.3s) - the
guard is read from the context of concurrent image and font fetches, so that is the path worth
racing. All three new test files were run Red first and failed on the missing symbols
(`transport.AddressLookup`, `engine.AddressBlocked`, `net.AddressGuardFromContext`) before any of
the production code above existed.

The injected resolver left one thing unproven by the suite: that the guard reaches the *system*
resolver correctly, since every guarded dial in the tests went to an address a fixture named. Two
live runs close it. `go run ./cmd/dump-url -url https://example.com` laid the page out, so the
swapped `DialContext` still negotiates real TLS. `go run ./cmd/goosie -backend headless -screenshot
-url https://lobste.rs` rendered with its linked stylesheet applied and its avatar images present -
both fetched through the guard, over real DNS, to a real host, on the first try. A dialer that
resolved wrongly or lost the server name fails that run visibly, which is the point of doing it
rather than declaring the unit tests sufficient.

## 13. Seventh round: the release path approved a build it never ran

§E's output is "structured feedback **or approval to release/tag**", and the repo has a place where
exactly that decision is made automatically: `.github/workflows/release.yml`, which publishes a
five-platform matrix on every tag. It has a step named "Smoke test", and that step was:

```bash
chmod +x "${binary}" 2>/dev/null || true
echo "PASS: smoke test for ${{ matrix.goos }}/${{ matrix.goarch }}"
```

Nothing in it evaluates anything. The `|| true` makes even `chmod` failure non-fatal, and the echo
runs unconditionally, so the job's own logs say PASS for a file that does not exist. This is the same
fail-open class §1 found in `v2-gate-check.sh` - a gate that prints a verdict it did not compute -
one level closer to the user, because a tag is the last gate before a download link.

Red evidence, with `dist/` empty and the committed step body extracted verbatim:

```
PASS: smoke test for darwin/arm64
old-step exit = 0
```

### The replacement: `scripts/release-smoke.sh`, and what each assertion is for

`scripts/release-smoke.sh <binary> [-frames 16] [-out report.json]` runs the artifact headless and
judges its own transcript. The release job now calls `scripts/release-smoke.sh "${binary}" -frames 24`
and the job carries `timeout-minutes: 20`, because a step that executes a binary needs a ceiling -
without one, a build that blocks on a window nobody is drawing to holds the runner default.

The exit-status contract is the one `test/gatecheck` already pins for the pacing gate, chosen so a
human reading a red job knows where to look: **0** the artifact is fine, **1** the artifact is bad,
**2** the checker could not reach a verdict (bad invocation, missing or non-executable file, a
transcript it cannot parse). Both 1 and 2 fail the job; only 1 is a defect in goosie.

Assertions, in the order the script makes them:

| check | what it catches |
| --- | --- |
| argument shape: missing path, dangling `-frames`/`-out`, unknown flag, non-unsigned or ≤0 frame count | a typo in the workflow silently skipping the test - the exact bug class this round is about |
| `-e`, `-f`, `-x` on the path | publishing a matrix entry whose binary was never built, and the "artifact is a directory / forgot its +x" variants |
| the binary's exit status | a build that crashes on startup, the regression a smoke test exists for |
| a `goosie: run` line and a `goosie: counters` line exist | a build whose reporting changed shape, where every field below would read as empty |
| `frames` equals what was asked for | a run that came up on an empty window and so "passed" every other check trivially |
| `backend=headless` | a run that depended on a display the runner does not have, i.e. the smoke test measuring the panel rather than the binary |
| `stamped >= 1` | drawing zero frames, which is a zero-work pass |
| `failed=0`, `panics=0` | a rasterizer or worker pool that survived only by swallowing errors |
| `-out` file exists, is non-empty, carries `"report"` | the JSON artifact the workflow uploads alongside the binary |

`stamped > frames` and missing/blank fields exit **2**, not 1: they mean the transcript is
unrecognisable, not that the browser is broken.

Green evidence against the real artifacts this round built locally (`go build` for each tag-exact
name, `-frames 24`):

```
release-smoke: PASS dist/goosie-v0.0.0-smoke-darwin-arm64 drew 24/24 headless frames, backend=headless, failed=0 panics=0   # exit 0
release-smoke: nothing to smoke test: dist/goosie-…-linux-arm64 does not exist                                             # exit 2
release-smoke: FAIL: …windows-amd64.exe exit 126                                                                           # exit 1
Killed: 9
release-smoke: FAIL: /tmp/goosie-truncated exit 137                                                                         # exit 1
```

The last three are the point of the exercise: the same three paths the old step printed PASS for.
A truncated binary is what a partially-uploaded artifact looks like, and a `.exe` on a mac is what a
matrix entry that was built on another runner looks like when the workflow forgets to skip it.

### The script is CI code, so it has a contract test

`test/gatecheck/release_smoke_test.go` (15 tests, package doc extends the gatecheck preamble) holds
the contract, in the same shape as `gatecheck_test.go`: a healthy transcript fixture, then
`editedStub(t, old, new)` mutates exactly one field and requires the exit status the header promises.
The binary under test is a bash stub that records its argv, honours `-out`/`-frames`, and prints the
real `cmd/goosie` report format, so the tests judge the checker rather than the engine and stay
deterministic without a display. Coverage: every exit-2 invocation error named in its own message,
every exit-1 verdict, `--help` exiting 0 while documenting `-frames`, and one test that asserts the
argv the stub actually received contains `-bench -backend headless -frames 23 -scene checkerboard
-out` - so the flags cannot be dropped from the call without a test failing.

Two things that test file caught in itself, worth repeating:

- The first fixture rewrote frame counts with `sed 's/frames=[0-9]* stamped=[0-9]*/…/'` after
  applying a mutation, which **silently undid three of them** (`stamped=0`, `stamped=-`,
  `frames=600`) and reported PASS for cases that must fail. The counts now interpolate from an
  unquoted heredoc, and `editedStub` fails the test unless the mutation target appears exactly once
  in the stub, so a mutation that stops matching is a loud failure instead of a quiet no-op.
- The stub logged argv after its `while` loop had drained `$@`, so the forwarding assertion had
  nothing to read.

### Known limits, recorded rather than smoothed over

- **Only `darwin/arm64` executed for real here.** This host cannot run linux or windows artifacts.
  What was verified for the other four is weaker: that a missing file and a non-mach-o file both fail
  loudly instead of passing, which is the behaviour the workflow actually depends on.
- **Nothing here asserts timing.** Mean and p99 budgets belong to `v2-gate-check.sh` and the nightly
  macOS job; a release smoke test that measured a runner's float latency would be the flaky gate §1
  argues against.
- **The checksum step still runs before the smoke step**, and `shasum -a 256 *` still hashes every
  file in `dist/`. Publishing is gated correctly - the `action-gh-release` step is after the smoke -
  but the ordering of those two bash steps is a leftover and is out of scope for this change.
- `go test -race ./...` now appears in three jobs (`build.yml`, `ci.yml`, `security.yml`). Real
  duplication, left alone: consolidating it is a workflow refactor with no coverage upside.
- Cross-compiling `darwin/amd64` from this arm64 host fails with `undefined: darwin.NewClipboard`.
  That is Go disabling cgo for cross-compiles, which excludes `internal/platform/darwin` and selects
  the stub build tags - it is a property of building on a laptop, not a repo defect, and the release
  matrix builds each darwin entry on a native runner for exactly this reason.

### Verification

`gofmt -l` clean on the touched Go files. `go vet -all ./...` exit 0. Full-repo `go test -race
-count=1 ./...` exit 0: 50 `ok` packages, no `FAIL`, no `DATA RACE`.
`go test -count=3 ./test/gatecheck/` → `ok github.com/vyquocvu/goosie/test/gatecheck 32.250s`, which
matters more here than elsewhere because the tests spawn bash and each mutation depends on a temp
dir's ordering. All five workflow files parse as YAML, and the parsed `release.yml` shows the smoke
step before the publish step with `timeout=20`.

## 14. Eighth round: the documented check that could not fail

§E clause 1 is "idiomatic Go patterns", and the repo's stated rule for that is
`CONTRIBUTING.md:19`:

```bash
gofmt -w .
go vet ./...
go test -count=1 ./...
go test -race ./...
```

Three of those four can report a miss. The first cannot: `-w` rewrites the files and exits 0
whether or not anything changed, so a contributor who skipped it - or who ran it and committed
only their own files - saw no failure, and no test or workflow asserted formatting either. This
is the fail-open class for the third time in this file, after `v2-gate-check.sh`'s unguarded
fields (§1) and release.yml's echo-a-PASS smoke step (§13).

### What it had already cost

17 of the module's 324 `.go` files are committed unformatted, across `internal/css`,
`internal/dom`, `internal/image`, `internal/style`, `internal/surface`, `cmd/`, and six test
packages; `test/engine/invalidation_test.go` is indented with spaces from its first line. The
number is the finding, and so is how I got it wrong first: my initial probe was
`gofmt -l cmd/goosie test/gatecheck test/net test/engine`, which answered three files, and I
nearly wrote the task up as three. A partially-scoped check is how this rule went unenforced in
the first place.

### Why the gate is not `gofmt -l .`

At the repo root that command reports 34 files, because `.worktrees/phase1-document-lifecycle/`
holds a second full checkout of this module and 17 of its files are unformatted too. Judging a
stale checkout is not the only way that hurts: it means the documented command cannot be the
gate, because nobody reading its output can tell current drift from fossil drift.

`archtest.UnformattedGoFiles` therefore walks like `CheckCgo` already does - dot-directories and
`node_modules` pruned, for the reason that function's comment gives - plus `testdata`, which is
what `go list ./...` never builds: a deliberately malformed `.go` fixture is a parser test's
input, not drift. Everything else that ends in `.go` is judged, including files no package builds.

The comparison itself is `go/format`, the package `cmd/gofmt` is built from, against the file's
own bytes. That avoids both a `gofmt` to find on `PATH` and a hand-rolled printer to keep in step
with the real one, and it agrees with what an editor's format-on-save decides. The two
implementations were cross-checked by running them: the new rule's Red named exactly the same 17
paths as `gofmt -l .` minus `.worktrees/`.

One case is deliberately *not* a violation: a file that does not parse returns an error rather
than being skipped. gofmt has no opinion about such a file's formatting, and a walk that quietly
passed it would be this round's own bug - a tree reading green while one file is not Go at all.

### Tests, in Red-first order

`internal/archtest/format_test.go`, four tests:

1. `TestRepoIsGofmtFormatted` - the gate. Red #1 was `undefined:
   archtest.UnformattedGoFiles`; Red #2 named all 17 files.
2. `TestUnformattedGoFilesFlagsDriftedFiles` - a temp tree with one clean and two drifted files
   reports exactly those two. Without it, a rule that always returned nothing would pass test 1
   and enforce nothing.
3. `TestUnformattedGoFilesPrunesExcludedDirs` - drifted files under `.worktrees/`,
   `node_modules/`, and `testdata/` are not reported, while the clean file at the root keeps the
   assertion from being satisfied by an empty walk.
4. `TestUnformattedGoFilesErrorsOnUnparseable` - the error names the file.

`test/gate/m1_gate_test.go:TestGate_ArchtestBoundaries` now applies the same rule alongside the
import-graph, toolkit and cgo ones, because that test's whole premise is that "M1 CI green" is
one command and the rules are not maintained twice. No workflow file needed changing: `build.yml`
and `ci.yml` run `go test ./...`, `security.yml` runs `go test -race ./...`, and the nightly runs
the gate suite, so all of them pick the rule up on the next commit. `CONTRIBUTING.md`'s conventions
list now says which command is the fix and which is the check.

### The formatting pass, and what proves it changed nothing

`gofmt -w` over those 17 files. gofmt preserves meaning by construction, so the thing worth
proving is that the pass touched nothing *else*: for each file, `git show HEAD:<path> | gofmt` is
byte-identical to the working file for 16 of the 17. The exception is
`internal/style/style.go`, which this session's earlier rounds already changed - its
non-whitespace diff against HEAD is that work (`parseBackgroundRepeat`'s empty-value guard and
`splitColorTokens`), which survives intact.

### Known limits, recorded rather than smoothed over

- `.worktrees/phase1-document-lifecycle/` is a stale full checkout of this repo that nothing
  prunes. Removing it is not this round's call, so the rule sidesteps it instead. Any *other*
  tool a contributor runs at the repo root - `gofmt -l .`, `grep`, a coverage pass - still reads
  it.
- The gate reports drift per file and fixes nothing. A contributor can satisfy it by deleting a
  file, which the compile then answers; a formatting test is not a build test.
- Formatting drift is not a browser defect, so the 17 reformats have no behaviour to verify
  beyond the suite that already covers them: no new test was written for the reformat itself, and
  none would mean anything.

### Verification

Reds as above, then green: `go test ./internal/archtest/` and
`go test ./test/gate -run TestGate_ArchtestBoundaries` both pass after the pass, and
`gofmt -l .` at the root now reports only the `.worktrees/` copy. `go vet -all ./...` exit 0.
Full `go test -count=1 ./...` exit 0 with the 17 files reformatted - the reformatted set includes
`internal/css/parser.go`, `internal/dom/node.go`, `internal/style/style.go` and
`internal/surface/window.go`, so the whole module had to be rebuilt and re-run, not just the
package under edit.

## 15. Ninth round: the cold start, as a number a gate can read

§1's first rule is that a check must be able to report a miss, and this round is that rule
applied to the brief's second primary target. §8 measured "spawn to first present" by hand -
subtract the spawn epoch from `frames[0].presented_at` in a Python one-liner - and then deferred
the gate to its own Deferred list. A hand-computed figure in a plan document is not an SLA:
nothing reads it, and nothing fails when the number moves.

The hand method also could not be moved into the shell wholesale, which is what made this a
design question rather than one line of `jq`. `internal/frame/recorder.go`'s own comment states
the reason: "Deltas are not a shell's job", because the timestamps encode as RFC 3339 strings and
a coerced null in a subtraction reads as a suspiciously fast frame. So the delta had to be
computed where the timestamps are already `time.Time`.

### The fail-open case this round had to avoid

Before writing anything I probed bash: `printf '%.3f' ''` prints `0.000`. An artifact with no
startup figure would therefore have been reported as **a 0 ms cold start** - the fastest start
possible - and would have passed every budget ever written for it. That is the same class as §1's
unguarded fields and §13's echo-a-PASS step, and it is why absence is encoded as `null` in the
artifact, `not measured` on stderr, and exit 2 under a budget, rather than as a zero-valued
`float64` field.

### What was built

The recorder owns the measurement, for one reason: it already writes the artifact the pacing gate
reads, so the cold start cannot disagree with the frames. `internal/surface` needed no change -
`draw()` leaves `PresentedAt` zero on an idle frame, so "the first mark with a non-zero
`PresentedAt`" is exactly "the first frame that reached a window".

- `recorder.go:97-98` holds `startupRef` and `firstPresented` as fields rather than reading them
  out of the ring. The reason is §8's geometry: the gate window is the last 600 marks, so the frame
  the user saw first is precisely the one that is no longer in the window.
- `recorder.go:112` `SetStartupReference`, `:128-129` the latch in `Record`, `:147` cleared by
  `Reset`, `:222-224` the delta in `Report`.
- `recorder.go:81` `StartupMS *float64` - a pointer so that "not measured" marshals to `null`.
- `main.go:82` `var processStart = time.Now()`, handed to the recorder at `:400`. Package
  initialisation runs before `main`, so this is as close to `exec` as a timestamp taken inside Go
  code can be.
- `report.go:69-71` prints `startup_ms=%.3f`, or `startup_ms=not_measured`.
- `v2-gate-check.sh:163-173` reads it, refuses a non-number, and gates it only under `-startup-ms`
  (`:59`, `:77`) - the reported-vs-gated split §1 established. A negative delta is refused too: it
  would mean the reference was stamped after the first present, i.e. a caller that measured the
  wrong instant.

`nightly-bench.yml:57` now carries `-startup-ms 1000`. That runner is the only one with a real
`CVDisplayLink`, so it is the only place a sub-second target means anything; headless CI asserts
the deterministic counters.

### Tests, in Red-first order

Red was `rec.SetStartupReference undefined`, `Report().StartupMS undefined`, and
`v2-gate-check: unknown argument -startup-ms` followed by `exit = 0, want 2` - the tests were not
vacuous before the code existed.

`test/frame/bitmap_test.go`, four tests on fake epochs:
1. `TestRecorderReportsStartupLatency:373` - an idle mark followed by a present at 120 ms reports
   120, not 200: the *first* present, not the last frame.
2. `TestRecorderStartupSurvivesRingEviction:394` - capacity 4, 40 marks, first present at 250 ms
   still reported after the window has wrapped.
3. `TestRecorderStartupIsNilWhenNotMeasured:414` - no reference → nil; reference and no present →
   nil; and after `Reset()` the next present reports 400 rather than the stale 250. That last
   assertion is deliberately about the *new* figure: "nil right after Reset" would have been
   satisfied by `Report()`'s empty-window early return and proved nothing about the stamp.
4. `TestRecorderStartupJSONIsNullRatherThanZero:449` - unmeasured marshals as a present key holding
   `null`, measured as the number 80.

`test/gatecheck/gatecheck_test.go`, three new plus three extended:
`TestStartupGatedOnlyWhenSupplied:379` (142.5 printed and "reported, not gated"; `-startup-ms 1000`
→ 0; `-startup-ms 100` → 1 naming `startup_ms`), `TestUnmeasuredStartupFailsClosed:403` (null under
a budget → 2; null ungated → 0 with "not measured" and no `0.000` on that row; key absent under a
budget → 2), `TestNonNumericStartupExitsTwo:429` (`"startup_ms":"fast"` → 2), and `startup_ms`
added to the negative-metric list `:350`, to the non-numeric-budget list `:320`, and to the
`--help` assertions `:289`. A `metricLine` helper now pins assertions to the row rather than to one
word appearing anywhere in the output.

### What it measures, on this machine, 2026-09-26

| run | `startup_ms` |
| --- | --- |
| `-backend headless -frames 120` | 61.797 |
| `-backend native -frames 120` | 190.191 |
| `-backend native -frames 5` ×3 | 199.139, 209.732, 210.181 |
| `-backend native -frames 1` ×3 | 178.898, 202.247, 214.773 |

**The target is met with ~5x headroom, and §8's 45 ms figure was the wrong measurement.** Its
median came from a headless run whose "present" is a timer, not a display. Through a real macOS
window the same quantity is 179-215 ms, and it is flat in `-frames`: the time is window creation
and the first commit, not the sweep. A 1000 ms budget sits above the worst reading by 4.7x, so it
would catch a regression that doubled or tripled it without firing on a noisy CI runner.

The whole nightly command was rehearsed locally, in the workflow's own argument order, against the
120-frame headless artifact and a synthetic bench transcript:

```
  startup_ms                   61.797  ok, budget 1000 ms
v2-gate-check: PASS     rc=0
```

and `-startup-ms 5` against the same artifact:

```
  startup_ms                   61.797  FAIL over budget 5 ms
v2-gate-check: FAIL     rc=1
```

### Known limits, recorded rather than smoothed over

- **What `processStart` excludes.** Kernel `exec`, dynamic loading, and the runtime's own start-up
  all happen before it. A native run's total wall time is larger than its reported startup: one
  reading put `startup_ms=191.6` inside ~314 ms of shell-measured wall, so ~122 ms sits outside the
  measurement. That figure is an upper bound rather than a measurement of `exec` - the two shell
  timestamps were themselves taken by spawning a Python interpreter.
- **This is time-to-first-pixel, not first contentful paint.** In an interactive run the first
  presented frame is the window's first paint - toolbar and blank placeholder - so the number is a
  lower bound on "the browser was usable" and an upper bound on nothing.
- **`-gate` cannot reach the null path**: flag validation refuses `-frames 0`, so every CLI gate run
  presents at least one frame. The branch exists for `Report()`'s empty-window return, and because
  the script cannot tell a `null` from a key that is absent, and neither reading may become 0.000.
- **The nightly timing is not reproducible locally**: GitHub's macOS runners differ from this
  machine in CPU, GPU, and how the display session is hosted. The 1000 ms budget is a regression
  fence, not a published spec figure, and the first nightly run is what will show whether it is
  loose enough for that hardware.
- Peak RSS remains ungated. §8's deferred item covered both numbers; this round closed one.

## 16. Tenth round: the load path, measured before it was bounded

§D tells QA to "run memory profiler (`go test -bench . -benchmem`, `pprof`)" and §G tells the
performance role to flag a heap regression, and both of those name a quantity nobody in this repo
had ever measured for loading a page. The benchmarks that did exist were frame-path benchmarks:
`-gate` and `test/gate` drive the *synthetic* scene generator, which never touches the HTML
tokenizer, the selector matcher or the box tree. §15 closed cold start to the first presented
frame, and that figure is likewise synthetic-scene work. So the brief's own instrument had a hole
in it: parse → style → layout → paint was the one part of the engine with no number attached.

### What the hole was worth

`test/engine/load_bench_test.go` loads `testdata/perf/gate_scroll.html` - a real 186 KB page,
82,120 px tall once laid out, owned by the perf work and unused by any Go test, so nothing here
touches the parity fixtures. The first measurement, `count=3`:

| | ns/op | ms/op | B/op | allocs/op |
|---|---|---|---|---|
| before | 74,326,583 | 74.33 | 146,240,997 | 420,394 |

Three counts agreeing within 0.05 %, so the quantity is stable enough to fence. ~790 bytes
allocated per byte of input. The memory profile then ranked the causes, and the CPU profile
showed the consequence: `runtime.madvise` was 26 % of load CPU, which is what a 146 MB churn over
a 67 ms path buys you.

Top allocation sites by object count: `strings.Fields` 14.0 %, `regexp.(*Regexp).replaceAll`
11.1 %, `layout.collectInline` 10.8 %, `paint.(*Builder).appendRun` 8.8 %,
`style.(*ruleIndex).candidates.func1` 8.5 %, `style.computeStyle` 8.5 %, `layout.splitWords`
7.4 %.

### The win that was provable rather than plausible

The cheapest of those was the regexp, because it was being run on text that could not match it.
`style.inViewport` (`internal/style/style.go:597`) rewrites viewport units on the way into every
property switch, once per declaration per element; a declaration whose parsed value is already a
plain length fast-paths out, but everything else hands its text to `css.ResolveViewportUnits`.
A probe over ordinary declarations - `"2px solid #ccc"`, `"bold"`, `"0 auto"`, `"#f0f0f2"`,
`"center"` - measured **3 allocations per call** for a value with no viewport unit in it: the
bit state, the output buffer, and the scan that fills them.

The filter is `internal/css/calc.go:45` `viewportTermHints`, and its correctness is a property of
the pattern rather than of testing luck. Every alternative in `viewportTerm` contains one of four
lowercase bigrams: `vw` outright, and so does each `s`/`d`/`l`-prefixed form; `vh` likewise;
`vmin` carries `mi`; `vmax` carries `ma`. A string carrying none of the four cannot contain a
viewport unit, so skipping the regexp is *identical* to running it and matching nothing - same
text back, same `false`. Which also means the rewrite's existing quirks survive untouched, and
the test pins them rather than fixing them: a unit inside a quoted string is still rewritten, and
an uppercase `10VH` is still not a viewport unit at all.

That argument is only worth anything if it cannot rot. The obvious check - render the parity
corpus with and without the filter and diff the PNGs - was rejected: it means editing production
source to disable the filter, and one half-executed attempt left `if !possible && false` in
`calc.go:66` before I caught it. So the lemma is instead proven where it is actually derivable, in
`internal/css/viewport_hint_test.go`, which reads the unit list *out of the compiled pattern*
rather than restating it: four tests, and if `viewportTerm` gains a unit the hint set does not
cover, they fail instead of the rewrite quietly stopping applying to that unit. See "Tests" below.

| | ms/op | B/op | allocs/op |
|---|---|---|---|
| before | 74.33 | 146,240,997 | 420,394 |
| after | 67.24 | 145,390,876 | 351,335 |

-16.4 % objects and -9.6 % wall for a behaviour-preserving eleven-line filter. The *bytes* moved
0.6 %, which is the honest reading: 69,059 objects went away and they were small ones. If the
brief's 50 MB clause is read as bytes-allocated, this round barely helped; read as objects and
wall - the two numbers that showed up in the profile as cost - it is the largest single win in
this file so far.

### Tests, in Red-first order

`test/css/viewport_units_test.go`, four tests:

1. `TestResolveViewportUnitsLeavesValuesWithoutAViewportTerm` - the 14 values come back byte
   identical and `found == false`. Green before the change, and it is the change's contract.
2. `TestResolveViewportUnitsWithoutAViewportTermIsFree` - **the Red.** `AllocsPerRun(200, …) != 0`
   for every value but the empty string, 3 per call for 13 of them and 2 for `" "`.
3. `TestResolveViewportUnitsAllocationsAreNotVacuous` - the same instrument over
   `"calc(100vw - 20px)"` must report *above* zero. Without it, test 2 could be satisfied by a
   rewrite that measured nothing at all, which is §1's rule applied to an allocation assertion.
4. `TestResolveViewportUnitsRewritesEveryUnit` - all ten units, a `calc()` term, a box-shadow
   with no viewport in it, the quoted-string case, the uppercase case.
5. `TestResolveViewportUnitsFallsThroughOnHintsWithoutUnits` - `maroon`, `canvas`, `max-content`,
   `middle`: three of these carry a hint bigram and must still fall through unchanged. This is
   the case the filter could have got wrong.

`internal/css/viewport_hint_test.go`, four tests, and the first in-package test in `internal/css`.
`patternUnits` reads the alternation out of `viewportTerm.String()` and then refuses to trust
itself: every unit it derives must satisfy `viewportTerm.MatchString("10"+unit)`, so a pattern whose
shape this reader no longer understands fails loudly instead of narrowing the corpus to nothing.

1. `TestViewportTermHintsCoverEveryPatternUnit` - the lemma itself, one assertion per unit.
2. `TestNoHintValueIsNeverAPatternMatch` - 23 values with no hint in them, checked against the
   regexp rather than against its source, so hints and pattern cannot disagree out of sight.
3. `TestHintedLookalikesStillGetThePatternsAnswer` - `maroon`, `max-content`, `middle`,
   `min-width`, `Mama`: these reach the regexp, and the regexp's own "nothing here" must survive.
   The filter's only job is to decline early; this is the set it was allowed to get wrong.
4. `TestEveryPatternUnitIsStillRewritten` - the end-to-end direction. If a hint goes missing, the
   failure shows up as a computation a page would see rather than as a source-text mismatch.

Two of my own expected values were wrong on first run and I corrected the test, not the code:
`100vw` at a 1440 px frame is 1440 px and the `- 20px` is left for the expression evaluator (I
had written 1420), and `5vw` is 72 px, not 720. A `calc()` carrying a viewport unit was already covered at the cascade level by
`test/style/logical_box_test.go`; these were my arithmetic, and the Red showed it.

`test/style/viewport_cascade_test.go` is the guard that the fast path may not change what a page
computes: the same document resolves `1vh`/`2vw`/`50vw` against the frame, keeps a `3px` alongside
them, and still turns `maroon` into `#800000` and `rgba(0, 0, 0, 0.5)` into alpha 128 - the two
values that *look* like hints to the filter. It passes before and after, which is the point: it
pins behaviour rather than asserting new behaviour.

### The fence, and the proof it can fail

A benchmark cannot fail a build, so §G's flag has to live in a test.
`test/engine/load_bench_test.go:77` `TestDocumentLoadAllocationFence` brackets the same quantity
from both ends - floor 250,000, ceiling 400,000 measured 351,334 - and refuses to mean anything
unless the document really laid out (the fixture's height must exceed 4,000 px; it is 82,120).
The floor is there because a ceiling-only fence can be satisfied by a load that stopped doing
work, which is this file's recurring failure mode in a new costume.

Both ends were rehearsed by moving them, not by reasoning about them:

```
# ceiling 300_000
a document load allocated 351334 objects, over the fence of 300000; this is the regression §G asks to be flagged
# floor 400_000
a document load allocated 351334 objects, under the floor of 400000; the load path stopped doing real work, or this instrument stopped measuring it
```

It runs under `go test ./...`, so `build.yml` and `ci.yml` get it on the next commit and the
nightly's existing "Run all tests" step gets it too. No workflow file changed.

### Two findings this round opened and did not fix

**`calc()` inside a box shorthand is dropped whole.** Writing the cascade guard, I asserted
`padding: calc(10vh - 4px) 3px` would give 86 px. It gave 0, and so did the `3px` beside it. A
probe over five forms:

| declaration | result |
|---|---|
| `padding: 5px 3px` | 5 / 3 |
| `padding: 90px 3px` | 90 / 3 |
| `padding: calc(10vh - 4px) 3px` | **0 / 0** |
| `padding: calc(10vh - 4px)` | **0** |
| `margin: calc(10vh - 4px) 3px` | **0 / 0** |

One unparsed argument fails the whole shorthand, silently, and the element renders with no
padding at all. That is a parity defect worth its own round and its own Red, not a change to
smuggle into a performance one - and it is a *shorthand* bug, so `internal/layout`'s function
handling of `calc()` elsewhere (the `max(1rem, calc(50vw - 720px + 1rem))` case in
`logical_box_test.go`) does not cover it.

**Live heap after a load is 68 MB, which is over the brief's 50 MB.** Measured with the caches
warm and a GC immediately before: `HeapAlloc` 23 MB → 68 MB at the end of the load → 0 MB after
the session is dropped and collected. Three things follow, and none of them is a threshold. That
68 MB is *not* a peak - Go reports no peak live heap, and the true maximum during the load is
above it. It is GC-timing-sensitive, so it is not a fenceable number yet, and per §8's rule about
measuring before gating I am not going to invent a budget for it. And "idle" in the brief means
the process at rest with no page loaded, which is a different quantity again: after a load and a
collection the document retains about nothing, so the 50 MB clause stays where §8 left it -
waiting on a target geometry and a tile-cache budget, which is a configuration decision rather
than a measurement.

### Known limits, recorded rather than smoothed over

- The fence brackets a *count of objects*, the cheapest stable thing to count. Bytes per load move
  with the tile cache and the glyph atlas, so they are reported by the benchmark and not fenced.
- `testing.AllocsPerRun` turns the collector off while it counts, so the fence test holds roughly
  three loads' worth of garbage at once - about 436 MB transiently. That is fine on a CI runner
  and is the knob to turn (fewer runs, or a smaller page) if it ever is not.
- The fixture is one page. A document with 10,000 elements would put a different weight on
  `layout.collectInline` and `paint.appendRun`, which are the two sites this round did not touch.
- `strings.Fields` is still the largest single allocation site in a load, at 14 %. It is not a
  one-line fix: it appears at ~20 call sites in the property parsers, and replacing it with
  `strings.FieldsSeq` (the module is on go 1.25, so it is available) is a mechanical sweep across
  `internal/style` with a real chance of a subtle behavioural miss in the exact parsers §D asks to
  be tested against malformed input. Deferred, with the profile attached, rather than bundled.
- The regexp's *time* cost is what the -9.6 % bought; the four-bigram scan is not free, it is just
  allocation-free, and it runs on every non-length declaration.

### Verification

Red first: `go test -count=1 -run 'ResolveViewportUnits' ./test/css/` reported exactly the
allocation test failing at 3 allocations per call, with the unit-rewrite and fall-through tests
green. Then green on the same command. `go test -count=1 -run TestDocumentLoadAllocationFence -v
./test/engine/` logs `351334 (document is 82120 px tall)` and passes, and the two rehearsals above
are its failure output. `go test -count=1 ./test/css/ ./test/style/ ./test/layout/ ./internal/css/`
all ok. `gofmt -l` clean on the touched files, `go vet -all ./...` exit 0, and `go test -count=1 ./...` exit 0
with all of this in the tree.

The hint-coverage tests got the same treatment as the fence - rehearsed by breaking the thing they
guard, with `calc.go` backed up and restored inside the one command. Dropping `"ma"` from
`viewportTermHints`:

```
--- FAIL: TestViewportTermHintsCoverEveryPatternUnit
    viewport unit "vmax" contains none of the hints [vw vh mi]; ResolveViewportUnits would skip it and never rewrite it
--- FAIL: TestHintedLookalikesStillGetThePatternsAnswer
    "maroon" carries no hint, so it is testing the skip rather than the fall-through
--- FAIL: TestEveryPatternUnitIsStillRewritten
    ResolveViewportUnits("10vmax") found no viewport term; the rewrite stopped applying to unit "vmax"
```

`grep -n 'viewportTermHints = ' internal/css/calc.go` after the rehearsal reads
`{"vw", "vh", "mi", "ma"}` again, and the four tests are green on the restored file.

**Behaviour preservation, and what it does *not* rest on.** The parity run against this change was
not performed. The plan was to render the 56-fixture corpus with and without the filter and diff
the PNGs; that needs the filter switched off in source, the command was rejected partway through,
and the half-executed version is what left the `&& false` in `calc.go:66` described above. What
the change's safety rests on instead is: the lemma and its four rehearsal-backed tests, the exact
value pins in `test/style/viewport_cascade_test.go` (which is where `maroon` → `#800000` and
`rgba(0,0,0,0.5)` → alpha 128 come from), and `go test -count=1 ./...` green. The recorded parity
baseline for the tree as it stands is **55/56 fixtures passing, average 95.02 %**, the single FAIL
being `semantic/03-aside-figure.png` at 59.07 % - unchanged from the previous round, and that
fixture's failure is an `aside`/`figure` layout gap that this round did not touch. If a rendering
A/B is still wanted, it can be done without touching source by building two binaries from a branch
rather than from a working-tree edit.

## 17. Eleventh round: the shorthand that swallowed a `calc()` whole

§16 closed with a defect it opened and deliberately did not fix: one unparsed argument failed a
whole box shorthand, silently, `padding: calc(10vh - 4px) 3px` computing to 0 on *both* sides.
This round is that fix.

### Root cause

`strings.Fields` splits on every whitespace, including the whitespace inside a function argument.
By the time a declaration reaches a shorthand parser, `style.inViewport` (`internal/style/style.go:597`)
has already rewritten the viewport terms, so `padding: calc(10vh - 4px) 3px` arrives as the text
`calc(90px - 4px) 3px` and `Fields` cuts it into `["calc(90px", "-", "4px)", "3px"]`. Four
arguments, of which only the last is a length. The one-to-four arity switch then assigns a fragment
to each side, every fragment resolves to 0, and the perfectly good `3px` is lost with them. Nothing
can report it, because in this engine a shorthand cannot distinguish "absent" from "zero".

The helper that splits this correctly was already in the same file: `splitTopLevelSpace`
(`internal/style/style.go:1582`) cuts only on whitespace at parenthesis depth zero, and `logicalPair`
(:1568) already used it. That is why `padding-inline: calc(10vh - 4px) 3px` was already right and
`padding: calc(10vh - 4px) 3px` was not - the logical and physical forms of the same property were
parsed by two different splitters.

### The change

Six call sites moved to `splitTopLevelSpace`, all of them argument lists whose arguments are lengths:

| site | property |
|---|---|
| `parseMarginShorthand` :2471 | `margin` |
| `parsePaddingShorthand` :2499 | `padding` |
| `parseBoxShorthand` :2524 | `border-width` |
| `radiusGroup` :2742 | `border-radius`, both halves of the `/` |
| `parseCornerRadius` :2771 | the four `border-*-radius` longhands |
| `parseFlexShorthand` :2783 | `flex`, third argument |

The last four came from a deliberate widening of scope. The brief's §F bans flexbox, but this tree
implemented it and `internal/layout/block.go` consumes `FlexGrow`/`FlexBasis`, so `flex: 0 0
calc(100% - 2rem)` - how a real page sizes a sidebar - was being dropped, not merely unparsed. The
three sites still on `Fields` were left for recorded reasons rather than oversight:
`parseBorderStyles` (:2580) takes keywords, where a parenthesised length cannot appear, and
`parseBackgroundSize`/`parseBackgroundPosition`/`parseFontShorthand` also split on `/` and carry
quoted strings, so a whitespace-only fix would be a third of the answer applied to the wrong half.

### The regression this round caused, and the test that caught it

The `flex` change was not safe as written. `parseFlexShorthand` dispatches on `len(parts)` with arms
for 1, 2 and 3 and no default, so before the change an unclosed `flex: 0 0 calc(10vh -` gave five
fragments, matched no arm, and set nothing; after it, three arguments - and the third fell through
`resolveLength` to **0**, i.e. "size to zero", from a value that is not a length at all. The
malformed-input test written for §D is what said so:

```
--- FAIL: TestFlexAndRadiusShorthandsKeepAFunctionArgumentTogether/flex-basis
    box_shorthand_calc_test.go:117: "flex: 0 0 calc(10vh - 4px)" = grow 0 basis -1, want grow 0 basis 86
--- FAIL: TestFlexAndRadiusShorthandsKeepAFunctionArgumentTogether/border-radius
    box_shorthand_calc_test.go:138: "border-radius: calc(10vh - 4px)" corner 0 = 0/0, want 86/86
    box_shorthand_calc_test.go:138: "border-radius: calc(10vh - 4px)" corner 2 = 4/4, want 86/86
    box_shorthand_calc_test.go:138: "border-radius: calc(10vh - 4px) 3px" corner 1 = -0/-0, want 3/3
```

Note the second line: with `border-radius` the defect was never merely silent. `calc(10vh - 4px)`
split into fragments hands `4px` to the third corner, so the old code did not drop the radius, it
drew the wrong one.

A probe over `css.ParseValue` explains both, and fixes the gate:

| text | `Value.Type` | `ToLength()` |
|---|---|---|
| `calc(10vh - 4px)` | 8 `ValueFunc` | 86 |
| `max(1rem)` | 8 `ValueFunc` | 16 |
| `calc(10vh -` | 7 `ValueList` | 0 |
| `10px` | 2 `ValueLength` | 10 |
| `auto` | 0 `ValueKeyword` | 0 |

An unclosed function does not parse as a broken function; it parses as a *whitespace list*, which is
exactly the shape that must not become a length. So `case 3` now assigns `FlexBasis` unless the
third argument is a `ValueList`. The first attempt gated on `lengthSpecified` instead, and the same
test failed again for the opposite reason - a legitimate `calc()` is a `ValueFunc`, which
`lengthSpecified` does not admit - so the gate is a negative one against the garbage type rather
than a positive one over the accepted types.

That test asserts per argument, not per declaration: `flex: 1 1 calc(10vh - 4px` still sets grow 1
and shrink 1, because those arguments *are* parseable, and "an unparseable value leaves the whole
declaration alone" would have been a claim the code does not implement.

### Tests, in Red-first order

`test/style/box_shorthand_calc_test.go`, four tests at a 1440x900 frame with a `cs.FontSize != 16`
guard so the `em` rows cannot be right by accident:

1. `TestBoxShorthandKeepsAFunctionArgumentTogether` - nine rows. Four pins of behaviour that was
   already correct (`5px 3px`, `1em 2rem`, `0 auto` → `MarginAuto`, `padding-inline`), five defect
   rows now resolving to 86 px where they used to resolve to 0.
2. `TestBoxShorthandWithAnUnclosedFunctionSetsNoSide` - the padding/margin half of the garbage case.
3. `TestFlexAndRadiusShorthandsKeepAFunctionArgumentTogether` - the widening: two `flex` rows, three
   `border-radius` rows, four `border-*-radius` longhand rows. Each longhand row also asserts the
   other three corners stayed 0, so a splitter that over-grouped would show up as a leaked corner.
4. `TestFlexAndRadiusWithAnUnclosedFunctionChangeNothing` - the one above, for malformed input.
5. `TestBoxShorthandSplitsOnEveryCSSWhitespace` - five separators: newline, `\r`, CRLF, form feed,
   tab. This is the test that found the second defect, and it is the one I wrote to *find* rather
   than to confirm.

`test/layout/box_shorthand_calc_test.go` is the §D-style coordinate check: with
`div { height: 20px; padding: calc(10vh - 4px) 3px }`, the following `section` must start at y = 192
and the `div`'s child `p` at x = 3. Writing it taught me something about the check rather than the
code - my first version asserted the sibling's `X`, which is set by its own containing block (the
body) and is 0 no matter what the padded box does. The horizontal side has to be read off a child.

The longhand rows were rehearsed by reverting only `parseCornerRadius` to `Fields`:

```
--- FAIL: TestFlexAndRadiusShorthandsKeepAFunctionArgumentTogether/border-radius-longhand
    "border-top-right-radius: calc(10vh - 4px)" corner 1 = 0/-0, want 86/86
    "border-bottom-right-radius: calc(10vh - 4px) 3px" corner 2 = 0/-0, want 86/3
    "border-bottom-left-radius: calc(1em + 2px)" corner 3 = 0/0, want 18/18
```

and the file was restored from a copy in the same command that ran the rehearsal, with
`TestFlexAndRadiusShorthands…` green afterwards. The `5px 3px` row passing under the reverted code is
the point of keeping it: it shows the failure is about parentheses, not about the property.

### Known limits, recorded rather than smoothed over

- **The two splitters were not equivalent, and the difference turned out to matter.**
  `strings.Fields` splits on `unicode.IsSpace`; `splitTopLevelSpace` split on space, tab and
  newline only. Six properties therefore stopped seeing `padding: 5px\r3px` as two arguments - and
  that is reachable, because a `\r` is legal CSS whitespace and a CRLF stylesheet can put one where
  a value wraps. `TestBoxShorthandSplitsOnEveryCSSWhitespace` was written to find out rather than to
  confirm, and it failed on exactly the two cases the gap predicted:

  ```
  --- FAIL: TestBoxShorthandSplitsOnEveryCSSWhitespace
      carriage return: "padding: 5px\r3px" = top 0 right 0, want 5/3
      form feed: "padding: 5px\f3px" = top 0 right 0, want 5/3
  ```

  The splitter now cuts on `' ' '\t' '\n' '\r' '\f'`, which is CSS's whitespace set, so the fix
  generalised a hazard that `logicalPair` had already been carrying since before this round. What
  still differs from `Fields` is non-ASCII whitespace - a no-break space splits an argument under
  `Fields` and does not now, which is the behaviour the spec wants.
- **`resolveLength` still turns garbage into 0, everywhere.** `ValueList → ToLength() → 0` is not a
  flex problem; it is how every property that does not gate on the type reads a value it cannot
  parse. This round gated `flex-basis` because that is where the widening made it observable. A real
  fix needs an "unspecified" representation distinct from both 0 and `auto`, which is a type change
  across `ComputedStyle` and belongs to a round of its own.
- **No fixture exercises this.** `grep -rl 'calc(' testdata/render` finds viewport-unit `calc()` in
  `logical_box`-style cases but no box shorthand whose argument is a `calc()`, so parity can only
  show that nothing else moved - it cannot demonstrate the fix. The demonstration is the coordinate
  test in `test/layout/box_shorthand_calc_test.go`, which fails before the change and passes after.
- `border-radius` still does not support the `4 values / 4 values` elliptical form *with* a
  `calc()` in it, because the `/` split is done before either half is parsed and a `calc()`
  containing a division would be cut in half. Out of scope here: that is a `/`-aware splitter, the
  same gap `background-size` has, and it is the next thing to fix if either ever comes up.

### Verification

Red first, twice over: the widening's own failures quoted above, then the rehearsal that reverted
one function to prove the longhand rows can fail. Then, on the final tree -

```
go vet ./...                                  # exit 0
go test -count=1 ./...                        # exit 0, zero FAIL lines
go test -count=1 -race ./test/style/ ./test/layout/ ./internal/style/ ./internal/css/
                                              # exit 0, all ok
go build -o /tmp/goosie-parity ./cmd/goosie   # exit 0
gofmt -l .                                    # 17 files, every one under .worktrees/
```

That `gofmt` count is not drift in this tree. `.worktrees/phase1-document-lifecycle/` is a second
full checkout with its own unformatted files, and `internal/archtest` excludes it on purpose
(`rules.go:344`) precisely because `gofmt -l .` would otherwise grade someone else's worktree. The
enforced gate - the archtest check the suite runs - is clean, as is `gofmt -l cmd internal test`.

Parity needed one repair first, and the failure is worth recording because it looks like a
regression and is not. The render pass produced all 56 PNGs and `score()` still printed
`no renders found` with exit 1: the Chromium reference cache lives at `/tmp/playwright-renders`,
macOS had purged `/tmp`, and `score` skips any pair with a side missing rather than reporting a
missing reference. So "no renders" means "no refs", never "goosie drew nothing". Regenerated with
`node testdata/render-compare.js` (56 refs) and re-scored against the same goosie PNGs:

```
55/56 passing (98.2%), average 95.02%
 59.07% FAIL semantic/03-aside-figure.png
```

That reproduces the previous round's recorded baseline exactly, same fixture, same percentages to
two decimals, which also confirms the regenerated refs came from the same Chromium build. Nothing
moved, which is the whole claim parity can make for this change: no fixture puts a `calc()` in a box
shorthand, so parity cannot show the fix - only that the six widened call sites left the corpus
alone.

## 18. Twelfth round: the last three length lists, and the veto that hid behind them

§17 moved six sites to `splitTopLevelSpace` and left three on `strings.Fields` with a stated reason:
`border-spacing`, `background-size` and `background-position` "also split on `/` and carry quoted
strings, so a whitespace-only fix would be a third of the answer applied to the wrong half". This
round is those three - and the pause turned out to have been holding a second defect, which the first
one was masking.

### Root cause, part 1: the same splitter

`border-spacing: calc(10vh - 4px) 3px` arrives as the text `calc(90px - 4px) 3px`, `Fields` cuts it
into four fragments, and the two-axis reader takes `calc(90px` and `-`. Neither is a length, both
resolve to 0, and the whole gap collapses. Unlike `padding`, the loss is not subtle: an 86 px gap
becomes 0 across an entire table.

### Root cause, part 2: a syntactic veto in front of the length reader

For the two background properties the split was not the only obstacle. `parseBackgroundSize`
(:2339) and `parseBackgroundPosition` (:2373) read their tokens through `parseBgLen` (:2442), which
handled a bare number, a `%` and a `px` - and nothing else - and the shorthand's classifiers
`isPositionTok`/`isSizeTok` began with `if strings.ContainsRune(t, '(') { return false }`. So even
with a parenthesis-aware splitter, a `calc()` token reaching either property was rejected *by shape*
before anything tried to read it, and in the shorthand it was then dropped from the position and size
token lists entirely, silently. Two guards, each of which made the other look sufficient.

The fix is therefore two-sided, and the second half is a type test rather than a shape test:

| site | change |
|---|---|
| `border-spacing` case :1540 | `strings.Fields` → `splitTopLevelSpace` |
| `parseBackgroundSize` :2340 | `strings.Fields` → `splitTopLevelSpace` |
| `parseBackgroundPosition` :2374 | `strings.Fields` → `splitTopLevelSpace` |
| `parseBgLen` :2442 | takes `cs` and reads a length function: `css.ValueFunc` with a non-empty `Str` and no `%` → `resolveLengthEm(v, 0, cs.FontSize)` |
| `isPositionTok` :2317, `isSizeTok` :2327 | the `(` veto removed; they now reject nothing by shape and defer to `parseBgLen` |
| `parseBackgroundShorthandExtras` :2242 | passes `cs` to both classifiers |

`v.Str != ""` is the load-bearing part of the new predicate, and it was a bug I introduced and
caught mid-round. `internal/css/value.go` has exactly two `ValueFunc` constructors: :87 for the length
functions (`calc`/`min`/`max`/`clamp`), which keeps the expression text in `Str`, and :97 for
everything else - gradients, and any other function - with `Parts` and an **empty** `Str`. Without
the guard, `background: url(a.png) rgba(1 2 3)` read its colour as a length of 0 and moved the layer
to the origin. Dropping the veto is only safe because the type, not the shape, says what a token is.

`resolveLengthEm` rather than a plain `ToLength()` because `em` has to survive: `inViewport` rewrites
viewport terms before the property switch, so by this point `calc(10vh - 4px)` is already px, but a
`calc(1em + 2px)` is still an `em` expression that only the font size can finish.

### Tests

`test/style/length_split_calc_test.go`, five cascade tests at 1440x900:

1. `TestBorderSpacingKeepsAFunctionArgumentTogether` - seven rows: two pins of already-correct
   behaviour (`4px` → 4/4, `2px 5px` → 2/5), a function in each slot and in both, `calc(10vw + 1px)
   calc(1vh + 1px)` → 145/10, and a newline-separated pair.
2. `TestBackgroundSizeKeepsAFunctionArgumentTogether` - `cover`, `100px 20px`, `50% 100px` as pins
   (the last also asserting the percentage flag, so a reader that conflates pct and length fails),
   then a function alone and beside a plain length in either order.
3. `TestBackgroundPositionKeepsAFunctionArgumentTogether` - `center 20px`, `10px 20px`, function in
   either slot, and the lone-value case, which has to leave the *other* axis at `BgPosCenter`: a
   single value is one axis only, and CSS centres the rest. I had that expectation wrong first and
   the test said so.
4. `TestUnclosedFunctionLeavesTheseLengthListsAlone` - `calc(10vh -` for all three properties, each
   compared against the same document with no declaration at all.
5. `TestBackgroundShorthandAcceptsALengthFunction` - the shorthand path (`url(a.png) calc(...)
   no-repeat`, `url(a.png) center / calc(...)`), and three rows that must **not** move the layer:
   `linear-gradient(...)`, `rgba(0, 0, 0, 0.5)`, `url(a.png) rgba(1 2 3)`. Those three are the
   regression that the `Str != ""` guard exists for.

`test/layout/length_split_calc_test.go`, three coordinate tests. Writing them corrected an
assumption of mine, so the numbers are worth recording: I first asserted the gap between a cell's
right edge and its neighbour's left edge, and the row distance for a 3 px → 4 px change, both of
which failed on a correct engine. In this layout the columns share whatever the gaps leave of the
table's specified width, so `border-spacing: 86px` and `85px` differ by 0.5 px of *column* advance
(300 − 0.5·s), and a cell's own 1 px padding is in the way of the edge gap. The clean assertions are
the absolute position of the first cell, which the probe confirmed as exactly the two axes
(`86/3`, `85/10`), plus whole-grid equality against the resolved px declaration:

| sheet | cells (x/y/w/h of the four `td`) |
|---|---|
| `border-spacing: calc(10vh - 4px) 3px` | 86/3, 343/3, 86/27.2, 343/27.2 - 169 wide |
| `border-spacing: 86px 3px` | identical |
| `border-spacing: calc(10vh - 4px)` | 86/86, 343/86, 86/193.2, 343/193.2 |

### Rehearsals

Reverting only the three splitters, with the new length reader in place - the cascade and the grid
both fail, and the failure mode is the interesting one (`v -0` is a negative zero from a `-`
fragment, not an absent value):

```
--- FAIL: TestBorderSpacingKeepsAFunctionArgumentTogether
    "border-spacing: calc(10vh - 4px)" = h 0 v -0, want 86/86
    "border-spacing: calc(10vh - 4px) 3px" = h 0 v -0, want 86/3
    "border-spacing: 3px calc(10vh - 4px)" = h 3 v 0, want 3/86
    "border-spacing: calc(10vw + 1px) calc(1vh + 1px)" = h 0 v 0, want 145/10
--- FAIL: TestBackgroundPositionKeepsAFunctionArgumentTogether
    "background-position: calc(10vh - 4px) 20px" modes = 0/0, want 3/3
    "background-position: 20px calc(10vh - 4px)" modes = 3/0, want 3/3
--- FAIL: TestCalcBorderSpacingInAShorthandMovesTheGrid
    first cell x = 0, want 86 (the shorthand's first argument)
    first cell y = 0, want 3 (the shorthand's second argument)
```

Then the reverse: splitters correct, the `parseBgLen` function branch removed. The same background
tests still fail, in different ways - `0/3` and `0/1` rather than `0/0`, i.e. the token is now whole
and still unread - which is the proof that the veto removal is load-bearing and not carried by the
split:

```
--- FAIL: TestBackgroundSizeKeepsAFunctionArgumentTogether
    "background-size: 10px calc(10vh - 4px)" height = -1 (pct false), want 86 (pct false)
--- FAIL: TestBackgroundPositionKeepsAFunctionArgumentTogether
    "background-position: calc(10vh - 4px)" modes = 0/1, want 3/1
--- FAIL: TestBackgroundShorthandAcceptsALengthFunction
    "background: url(a.png) calc(10vh - 4px) no-repeat" x = 0/0, want length 86
    "background: url(a.png) center / calc(10vh - 4px)" size = 0/0, want length 86
```

Both files were restored from a copy inside the same command that ran each rehearsal, and the cascade
tests were green afterwards.

### 12b: the mirror image, found by reviewing the helper this round widened

Reviewing `splitTopLevelSpace` after the change rather than before it turned up the opposite defect,
which no §17 or §18 test reaches. The depth counter was decremented without a floor, so one unmatched
`)` in a hand-written or truncated sheet took it negative - and from there *no* whitespace is at top
level, so the remainder of the value never splits. Concretely `padding: 7px) 3px` became one
unparseable token and the perfectly good `3px` beside it was dropped. That is exactly the failure this
round was fixing, arrived at from the other direction, and it is reachable from a stray keystroke.

Written as `TestUnbalancedCloseParenDoesNotSwallowTheNextArgument` (`test/style/length_split_calc_test.go`)
and run before touching the helper:

```
--- FAIL: TestUnbalancedCloseParenDoesNotSwallowTheNextArgument
    length_split_calc_test.go:179: `padding: 7px) 3px` = right 0 left 0, want 3/3 (the parseable argument survives)
    length_split_calc_test.go:184: `border-spacing: 2px) 5px` = v 0, want 5
```

The fix is a clamp, not a decrement: `if depth > 0 { depth-- }`, the same shape
`splitOutsideParens` (`internal/css/parser.go:375`), `splitTopLevel` (:693),
`expandCSSImports` (`internal/engine/imports.go:92`) and `cssBudget.check` (`internal/engine/limits.go:164`)
already used. The
third assertion in that test - `padding: calc(max(1em, 2px) + 4px) 3px` resolving to 20 px / 3 px -
passed before the change and is kept as the pin that nested functions still group.

**The remaining `depth--` walkers were audited and left alone, with evidence.** Grep for `depth++`
across `internal/` finds nine decrements still unclamped: `css/value.go:143`, `:163`,
`css/parser.go:544`, `:604`, `:803`, `:854`, `css/calc.go:249`, `style/gradient.go:320`, and
`css/selector.go:225`. They are not the same defect, for two reasons this round can show rather than
assert:

- Most of them only hunt a *matching* close and never split a list, so a negative depth cannot merge
  two legitimate values - `selector.go:219` and `gradient.go:227` even start at 1 and loop on
  `depth > 0`, which makes negative unreachable.
- The ones that *do* split a list were probed directly through the unexported functions:
  `splitArgs("1px), 2px")` returns one merged argument, so the hazard is real there, and
  `calcPx` returns `ok = false` for `"10px) + 5px"`, `"(10px)) + 5px"` and `"10px) -( 5px )"`.

That difference is what decides it. In `style` the swallowed sibling was independently parseable, so
the engine drew a *wrong number*; in `css` the merged argument carries the stray `)` itself and cannot
parse either way, and `calcPx` reports failure instead of silently returning 0. Clamping those would
change the shape of a rejection with no behaviour gained, so they stay as they are and this records
why. (`css/value.go:137`'s `lengthFuncName` is worth naming too: it returns as soon as depth reaches 0
mid-string, so negative depth is unreachable on its accept path.)

### Known limits, recorded rather than smoothed over

- **A `calc()` mixing a percentage with a length is still unread** here as it was in §17: `parseBgLen`
  returns a `(value, isPct)` pair, which has no representation for "86 px plus 10 %", so such a token
  now reads as unparseable and the callers fall back to `auto`. That is deliberately *not* the old
  behaviour of taking whichever half parsed. Making it work needs a type change in `ComputedStyle`,
  which is the same deferred item §17 recorded.
- **`tokenizeBackground` (:2278) still omits `\f`** from its whitespace set, unlike
  `splitTopLevelSpace`. The shorthand's `/` split is parenthesis-aware, so the gap is only reachable
  through a form feed inside a `background` value; recorded rather than widened, because no test in
  this repo can tell me which behaviour a real sheet expects.
- **`grid-template-rows`/`-columns` still split on `Fields`** (`parseGridTemplateShorthand` :2846), so
  `grid-template-columns: 1fr calc(10vw - 4px)` has exactly this defect. Not fixed here: that parser
  also handles `[named-line]` brackets and `minmax()` as one argument, so the correct splitter for it
  is depth-aware in parentheses *and* brackets, and grid is beyond §F's Phase-1 scope in any case.
- The `4 values / 4 values` elliptical `border-radius` and a `/` inside a `background-size` expression
  remain the `/`-aware-splitter gap §17 recorded; this round did not touch it.
- **No fixture exercises any of this.** `grep -rniE
  '(border-spacing|background-size|background-position)[^;]*calc\(' testdata/render` matches nothing,
  and the corpus contains exactly one `calc()` anywhere - `height: calc(100vh - 70px)` in
  `complex/01-dashboard.html` - so §17's "viewport-unit `calc()` in `logical_box`-style cases"
  overstated it. Parity therefore can only show that nothing else moved. The demonstration is the
  coordinate test, which fails before the change.

### Verification

Red first, three times over: the two rehearsal failures quoted above (each file restored from a copy in
the same command that ran the rehearsal), then 12b's `TestUnbalancedCloseParenDoesNotSwallowTheNextArgument`
against the unclamped helper. Then, on the final tree -

```
go vet ./...                                  # exit 0
go test -count=1 ./...                        # exit 0, zero FAIL lines
go test -count=1 -race ./test/style/ ./test/layout/ ./internal/style/ ./internal/css/
                                              # exit 0, all ok
gofmt -l cmd internal test                    # silent
go build -o /tmp/goosie-parity ./cmd/goosie   # exit 0
```

and the parity render over the 56-fixture corpus with that binary:

```
55/56 passing (98.2%), average 95.02%
 59.07% FAIL semantic/03-aside-figure.png
```

Byte-for-byte the baseline §17 recorded - same fixture, same percentages to two decimals - so the three
splitter sites, the length reader behind them, the veto removal and the clamp left the corpus alone. As
that section's own limit says, no fixture puts a `calc()` in any of these three properties, so parity
cannot demonstrate this fix; the demonstration is `test/layout/length_split_calc_test.go`, which fails
before it.

## 19. Thirteenth round: the leak that was not a leak, and the ledger that can tell the two apart

### The ticket this round closed was wrong, and the evidence said so before any fix was written

The open item from the twelfth round read "frame-proportional live-heap leak: 720 MB after 600 gate
frames, ~1.2 MB per frame". A per-frame figure is a leak shape, and a leak shape is what the M1
zero-allocation criteria say cannot exist. So the ticket was tested rather than fixed: the same
headless run at 1, 200, 600 and 1400 frames.

| frames | `tile_bytes` | `budget` | evictions | `tiles_rasterized` | maxrss |
| --- | --- | --- | --- | --- | --- |
| 1 | 60.0 MiB | 749.0 MiB | 0 | 0 | 91.4 MiB |
| 200 | 294.0 MiB | 749.0 MiB | 0 | 1,164 | 408.9 MiB |
| 600 | 747.0 MiB | 749.0 MiB | 0 | **2,988** | 864.5 MiB |
| 1400 | 747.0 MiB | 749.0 MiB | 0 | **2,988** | 907.7 MiB |

Both plateaus are the story. Tile bytes stop rising at the configured budget and process memory stops
rising with them, and the rasterized count is *identical* at 600 and 1400 frames - 2,988 buffers of
`frame.TileSizeBytes()` = 783,286,272 B, which is 747.0 MiB to the byte - so by 600 frames the whole
scene has been drawn once and 800 more frames of scrolling rasterize nothing further. Nothing grows
per frame, which is the shape "~1.2 MB per frame" claimed did exist; `evictions=0` says the cache
never even had to free a buffer to stay inside its authorisation. `TestFootprintInstrumentDetectsRetention`
remains the guard that this measurement can see real retention, so "it plateaued" is not "it cannot
tell".

The recipe matters more than the numbers, because the obvious way to run it is wrong. Each row is one
`-bench -backend headless -scene checkerboard -frames $n -out …` invocation of the *shipped binary*,
with `tile_bytes` / `tile_budget` / `tile_evictions` / `tiles_rasterized` read out of that run's JSON
artifact and maxrss read from that run's own child. The child must be fresh per stop:
`resource.getrusage(RUSAGE_CHILDREN).ru_maxrss` is a cumulative high-water mark over *all* children,
so a loop of four runs in one process reports the largest run's peak four times, and only the first
child of a process is cleanly attributable.

*Correction, recorded rather than smoothed over.* The table this section was first written with had a
"live heap" column and a buffer count of 2,943, both remembered from an interim print rather than
copied from a run - the same failure mode §12 of this document names. The figures above are the ones
the artifact and `RUSAGE_CHILDREN` produced on the final tree, and they are what the conclusion rests
on. The conclusion itself did not change: it was reached from the plateau, which held in both sets.

**The cause is a configuration, not a defect**: `cmd/goosie/main.go:379` gives a paced synthetic run a
tile budget of the *whole* document plus eight, so the gate scene's cache is authorised at 749 MiB
while the same figure for a real page is capped at 256 tiles / 64 MiB by
`engine.TileCacheBudget` (`internal/engine/limits.go:58,132`, used at `main.go:793` and `:1303` and
fenced by `test/engine/viewport_test.go:53`). The document is deliberately long -
`paint.DefaultDocHeight` is 600 scroll frames plus two screens - because a measured run must not press
against the end of the page while it is being timed.

Two consequences were drawn from that rather than argued:

- **No peak-RSS fence goes on the nightly command as it stands.** It would gate the synthetic scene's
  cache size, which is a knob, and pass or fail on it without learning anything about the browser.
  The measured whole-process figures for the *capped* path are the ones that bear on "lightweight":
  194.3 MiB maxrss for a 1440x900@dpr 2 headless run, measured last session through
  `resource.getrusage(RUSAGE_CHILDREN)` - note that `/usr/bin/time -l` reports 5.3 MiB for the same
  binary and is wrong here, which is worth knowing before anyone builds a fence on it.
- **"<50MB idle" stays a configuration decision, not a bug.** Retained memory at rest is cache budget
  × viewport geometry × the two whole-surface rings; naming a number under 50 MB means picking a tile
  cap below what the gate scene asks for. That choice is handed over, with these two figures as the
  inputs.

### What was actually missing, and what this round built

The nightly artifact reports `tiles_rasterized` and the run's stderr carries `bytes=` / `budget=`,
which are `grid.Bytes` / `grid.Budget` proxied through `raster.Stats` - but the JSON the gate script
reads carried no tile memory at all, and no eviction count anywhere. So "the cache filled what it was
configured to hold" and "tile buffers escaped the accounting that authorised them" produced
byte-identical artifacts, and the only way to tell them apart was a heap profile pulled by hand. That
is the distinction a memory gate has to be able to make, so the artifact now carries it:

- `frame.Report` gained `tile_bytes`, `tile_budget`, `tile_evictions` as `*int64`, filled by
  `FrameRecorder.SetTileLedger(frame.GridStats)` - the second fact the recorder is handed from
  outside the frame path, after `SetStartupReference`, and for the same reason: the grid is not in the
  ring of marks, so no window of frames can observe it.
- `cmd/goosie/report.go` reads the ledger from `Scheduler.GridStats()`, i.e. the grid the scheduler is
  bound to right now, so the artifact's ledger and the stderr counters cannot describe different
  caches after a tab switch or a navigation. The counters line gained `evictions=`.
- `scripts/v2-gate-check.sh` prints the three as a ledger - held MiB against the MiB the run's own
  configuration authorised - and gates them only when `-tile-mib` names a figure. Absent `tile_bytes`
  with a budget supplied is exit 2, never a comfortable zero.

### Tests, in Red-first order

1. `test/gatecheck/gatecheck_test.go` - five cases against the script, written and run failing first
   (`v2-gate-check: unknown argument -tile-mib`, and no `tile_mib` line at all). Reported-not-gated by
   default; `-tile-mib 800` passes and `-tile-mib 700` exits 1 on the same artifact; a stripped
   `tile_bytes` exits 2 when gated and prints "not reported" - not "0.000" - when not; a
   non-numeric `tile_bytes` exits 2 rather than bash-coercing its way to a pass.
2. `test/frame/bitmap_test.go` - `TestRecorderReportsTileLedger` and
   `TestRecorderTileLedgerJSONIsMissingRatherThanZero`, both run failing first (compile error: no
   `SetTileLedger`, no fields). A measured zero evictions stays the number 0; an unmeasured ledger is
   `null`, because zero held bytes is a cache that never filled and would gate as the best result
   there is.
3. `test/gate/artifact_test.go` - `TestGateArtifactCarriesTheTileLedger` builds the shipped binary,
   runs 16 headless frames, and reads the artifact back. This one is the round's actual gap: the
   recorder's own tests pass and the script's pass, and the wiring between them was still absent.
   Run failing first against the built binary (`tile_bytes holds <nil>; want a number of bytes`), and
   it pins `held > 0`, `budget > 0` and `held <= budget` where they are produced.

### Verification

```
go vet ./...                                   # exit 0
gofmt -l cmd internal test                     # silent
go test -count=1 ./...                         # exit 0, 28 packages ok, zero FAIL
```

and the round's own claim, end to end through the real artifact:

```
$ goosie -bench -frames 16 -scene checkerboard -out /tmp/ledger.json
goosie: counters ... bytes=81788928 budget=785383424 evictions=0 ...
$ jq -c '.report | {frames, tile_bytes, tile_budget, tile_evictions}' /tmp/ledger.json
{"frames":16,"tile_bytes":81788928,"tile_budget":785383424,"tile_evictions":0}
$ scripts/v2-gate-check.sh /tmp/ledger.json -mean 40 -tile-mib 800 | grep tile
  tile_mib                     78.000  ok, budget 800 MiB
  tile_evictions                   0  reported, not gated
```

Ungated the same artifact reads `78.000 of 749.000 MiB configured, reported, not gated`. (That run's
`p99_ms` fails its default 33 ms budget: it is a 16-frame headless burst on a laptop, not the
nightly's display-paced 600-frame run, and nothing here claims otherwise.)

### Also fixed, found while reading the numbers

`internal/paint/synthetic.go:36` documented `DefaultBudgetTiles = 256` as "16 MiB at TileSize". 256 ×
262,144 B is 64 MiB - a four-off in the comment that would have made the next reader discount the
budget as small. Corrected in place; the value was already what the design wanted.

### Deferred, with the reason

- **The nightly does not gate the ledger yet.** `-tile-mib 800` is verified above but a flat figure
  re-gates the scene's configuration, which is what this round's diagnosis argues against. The fence
  worth having is the one `test/gate/footprint_gate_test.go` already uses: held may exceed *its own
  reported budget* by a slack, and not by a surface bitmap's worth. That is a new script mode
  (`-tile-slack MIB`, comparing two numbers in the artifact rather than one against a flag) and a
  threshold decision, so it is a round of its own rather than an add-on here.
- **`<50MB idle`** - above: the user's geometry-and-cache decision, now measurable from the artifact
  instead of from a heap profile.
- **Whole-process retention across tabs** is not what this ledger reports. It names one grid - the
  scheduler's current one - because that is the cache whose bytes are already on the counters line.
  Per-tab totals belong to the footprint gate, which measures the process rather than a subsystem.
- `-screenshot` writes no run report at all (it exits 0 silently), so the capped document path cannot
  be measured through it. Noted as a reporting gap, not fixed here: it is a different surface from
  this round's, and no claim in this document depends on it. (Round fourteen closed the measurement
  half of this by another route - `-bench -url` now measures the capped path - and left the
  screenshot's own artifact silence where it was.)

## 20. Fourteenth round: the measured path that measured nothing, and the artifact that now says what it drew

### The hole, found before it was fixed because the numbers did not add up

§19 established that the only tile budget which is a *cap* - `engine.TileCacheBudget`, 256 tiles /
64 MiB (`internal/engine/limits.go:132`) - belongs to real documents, and that the gate's 749 MiB is a
scene configuration. So the figure that bears on "heap allocation exceeds 50MB" was the one the gate
could not reach, and trying to reach it produced this:

```
$ goosie -bench -backend headless -url file:///…/01-dashboard.html -frames 60 -out /tmp/doc.json
goosie: run … scene=checkerboard …        # what was configured, which was all the artifact had to say
$ jq -c '.report | {tile_bytes, tile_budget}' /tmp/doc.json
{"tile_bytes":25165824,"tile_budget":27262976}
```

`cmd/goosie/main.go` skipped `-url` for every paced run (`if c.url != "" && !c.gate && !c.bench`), so
`-bench -url X` exited 0 with an artifact full of plausible timings measured on the synthetic
checkerboard, and `-screenshot -url X` wrote no artifact at all. A 26 MiB budget that belongs to a
one-viewport blank layer is indistinguishable in the JSON from a 26 MiB budget for a page - and a
reviewer reading the file has no page to read it against. That is the same shape as the round-thirteen
gap (an artifact whose provenance the instrument could not state), one level up: the recorder could
not say *what* it timed.

### What this round built

- `frame.Report` gained `document` and `doc_height`, filled by `FrameRecorder.SetContent(url, height)`
  - the third fact the frame ring cannot observe, after the startup reference and the tile ledger, and
  set before the window is consulted: what a run drew is true of the run, not of the marks still in
  the ring. A synthetic scene is `document=""` with the scene's own `doc_height`, which is a reported
  value rather than a missing one.
- A paced run with `-url` now loads synchronously, installs the layer as the plan **and** as
  `f.layer` - the driver's scroll range is `f.layer.Bounds.H() - viewport` (`cmd/goosie/drive.go:45`),
  so swapping only the plan would time a run scrolling a one-viewport page while reporting a tall one -
  and `cmd/goosie/report.go` prints `document="…"` on the run line. `scene=` still names what was
  *configured* and reads `checkerboard` on a document run: it is a flag echo that
  `scripts/release-smoke.sh`'s test grades, so it was left meaning what it meant and `document=` was
  made the key that decides the question.
- Because startup is anchored at process start (`main.go:405`), a document run's `startup_ms` now
  includes fetch, parse and layout: 25.4 ms for a local 204-line page at 1440×900@dpr 2. That is the
  path on which the brief's "sub-second cold start" can be gated against real content rather than
  against a window.

### Tests, in Red-first order

1. `test/frame/bitmap_test.go` - `TestRecorderReportsContent`, Red as a compile error (no
   `SetContent`, no fields): both figures present on a run with no marks at all, `""` for a scene, and
   a `document` string next to a `doc_height` *number* in the JSON, since a stringified height is what
   a script silently stops reading.
2. `test/gate/document_artifact_test.go` - new, Red as `the artifact has no "document" key` against
   the built binary while `TestGateArtifactCarriesTheTileLedger` kept passing. It writes a 40-block,
   8000-px fixture and asserts the artifact names it, reports a height that a dropped URL could not
   (a blank layer is exactly one viewport tall), and carries a budget `<= engine.MaxTileCacheBytes` -
   the capped path, gated where it is produced. Its companion pins the scene case: no document, the
   scene's own height, and a budget *above* the cap, so the two paths cannot be confused for each other.
3. `runBench` in `test/gate/artifact_test.go` became `runArtifact(t, args…)` with the frame-count
   check against whatever the caller asked for, so the document test reuses the same round trip that
   was proved to work rather than a second copy of it.

### The page-height question this raised, answered by a width sweep rather than by reading the layout code

The dashboard reports `doc_height=1800` - exactly one viewport at 1440×900@dpr 2. Two readings: the
URL was dropped again, or the page genuinely lays out to 900 CSS px here. Varying the viewport width
separates them, because a dropped URL is one viewport tall at every width:

| fixture | w=1440 | w=700 | w=400 | w=300 |
| --- | --- | --- | --- | --- |
| `01-dashboard.html` | 1800 | 2156 | 2990 | 2990 |
| `03-blog-article.html` | 7997 | 8199 | 11100 | 13892 |

The document is loaded and laid out. What 1800 means is that a page built from `display:flex` and
`height: calc(100vh - 70px)` collapses to about one screen under a box-and-flow layout engine, which is
the §F scope decision being applied rather than a measurement defect - and it is a render-fidelity gap
worth naming: Chrome lays that dashboard out taller than its viewport, goosie does not, and the
parity harness is where that is scored, not this artifact.

### The first gated figures off the capped path

Each row is one fresh measuring process around one `-bench -backend headless -frames 60` invocation of
the shipped binary, per §19's recipe (a shared process would report one cumulative `ru_maxrss`):

| run (1440×900@dpr 2) | `document` | `doc_height` | held | budget | evictions | rasterized | maxrss | `startup_ms` | mean frame |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| scene | - | 63600 | 129.0 MiB | 749.0 MiB | 0 | 516 | 237.2 MiB | 17.8 | 76.8 ms |
| dashboard | ✓ | 1800 | 24.0 MiB | 26.0 MiB | 0 | 96 | 128.8 MiB | 25.4 | 42.6 ms |
| blog article | ✓ | 7997 | **64.0 MiB** | **64.0 MiB** | **3,586** | 3,831 | **175.0 MiB** | 29.9 | 104.7 ms |

The third row is the one §19 could not produce. The cap is not decorative: held sits exactly on budget
and nearly every rasterization after the cache filled had to free a buffer first, which is what a
256-tile cache under scroll looks like, and the round-thirteen eviction counter reports it for the first
time. Whole process, a 60-frame scroll of an 8000-px article peaks at **175.0 MiB** - above the brief's
50 MB figure, and now stated as a measurement rather than as an inference from a heap profile. The route
to under 50 MB is the tile cap (and the two whole-surface rings), which is the configuration decision
handed back in §19 with these numbers as its inputs.

`rasterized`, `evictions` and the frame mean move between identical invocations - the row above is one
run's artifact, and repeats gave 3,782/3,526 and 3,887/3,651 with means of 106.5 and 114.2 ms - because
worker arrival order decides which tiles are pinned when a frame needs the space. What did not move
across those three runs is the half the conclusion rests on: `held == budget` every time, and maxrss
between 174.6 and 175.0 MiB.

The frame means are headless and unpaced - the fps counter says 320 to 990, so these are raster-bound
costs, not the nightly's display-paced figures, and no 60fps claim is being made from them. The
existing `v2-gate-check.sh` fences are scene-path fences; document-path fences need a fixture and a
threshold, which is a round of its own.

### Verification

```
go vet ./...                                   # exit 0
gofmt -l cmd internal test                     # silent
go test -count=1 ./...                         # exit 0, 28 packages ok, zero FAIL
go test -race ./cmd/... ./internal/frame/ ./test/frame/   # ok
```

and the two things the round actually claims:

```
$ goosie -bench -backend headless -url file:///…/03-blog-article.html -width 1440 -height 900 -dpr 2 -frames 60 -out /tmp/doc.json
goosie: run backend=headless … frames=60 … scene=checkerboard … startup_ms=20.469 \
        document="file:///…/03-blog-article.html"
$ jq -c '.report | {doc_height, tile_bytes, tile_budget, tile_evictions}' /tmp/doc.json
{"doc_height":7997,"tile_bytes":67108864,"tile_budget":67108864,"tile_evictions":3635}
```

`-screenshot` moved no pixels: the PNG from the new binary is byte-identical to the old one
(`eb0b9761adb3d54a`), which is the check that matters because the screenshot run and the paced bench
run now share the loading branch.

### Deferred, with the reason

- **No document-path pacing fence yet.** The 104.7 ms mean above is one headless machine's unpaced
  raster cost; naming a threshold needs either the nightly's display-paced path pointed at a fixture
  or an accepted headless figure with a documented margin, and that choice is a configuration
  decision, not a code one.
- **`<50MB idle`** - now measurable end to end (64 MiB of cache alone exceeds it), still the user's
  cap/geometry call, per §19.
- **`-screenshot` still writes no artifact** (`run()` returns before `report()` on that branch).
  Unchanged this round: the measurement it owed was the document budget, and `-bench -url` now
  carries it.
- **The dashboard's collapsed height** is a layout-scope consequence, tracked by the render-parity
  harness rather than by this artifact.

## 21. Fifteenth round: the `@font-face` table that outlived its document

### The clause, and why this one was a bug rather than a style preference

§B of the brief asks for "production-ready, idiomatic Go code … and **zero global state**". Fourteen
rounds had treated that as a code-quality wish. In `internal/style` it was neither: three
process-level pieces of mutable state decided which font a paragraph drew, and one of them was the
family table that `@font-face` writes into.

The shape at HEAD (`git show HEAD:internal/style/style.go`, lines 1946-1975):

```go
var (
    customFontsMu      sync.RWMutex
    customFontIdx      map[string]uint16
    customFontRegistry map[CustomFontKey]uint16
)
func SetCustomFonts(m map[string]uint16) …
func SetCustomFontRegistry(m map[CustomFontKey]uint16) …
```

`engine.Session.loadFonts` built a fresh map per document and then *published it over the process*,
so the second tab or the second navigation rewrote the family names the first one's cascade would
look up. Worse, the indices in those maps are rasterizer-scoped: `Session.fontReg.Register` returns a
1-based index that only means something inside *that* document's `FontRegistry` (`internal/engine`).
A global table therefore does not merely lose precision across documents, it points document A's
`font-family: AlphaFace` at document B's glyph slot - or, when the lookup misses, at no face at all.

A `sync.RWMutex` around it bought thread safety and nothing else: the race was semantic, not
bitwise. That is the reason to fix the clause rather than file it.

### The Red, and what it actually said

`test/engine/custom_font_scoping_test.go` - two sessions, each with its own `fakeRegistry`, each
declaring one family, then re-reading document A's `FontSlot().CustomIdx` after B loads:

```
$ go test -count=1 -run TestCustomFontResolutionIsPerSession ./test/engine/
--- FAIL: TestCustomFontResolutionIsPerSession (0.00s)
    custom_font_scoping_test.go:57: after document B registered its own @font-face set, document A
        resolves AlphaFace to index 0: the family table is process-global, so B's registrations
        replaced A's
```

Index 0 is "no custom face", so the failure is not a wrong glyph - it is the document silently
falling back to the system font for text whose `@font-face` it had successfully downloaded and
registered. The same line also proves the first assertion had passed a moment earlier: A resolved to
1 until B existed.

### What replaced it

One type, owned by the session, injected through the cascade:

```go
type CustomFonts struct { mu sync.RWMutex; families map[string]uint16; variants map[CustomFontKey]uint16 }
func (f *CustomFonts) Register(family string, idx uint16, bold, italic, light bool)
func (f *CustomFonts) has(name string) bool
func (f *CustomFonts) index(family string, bold, italic, light bool) uint16
```

`Session.customFonts` is built by `loadFonts` and passed to `style.ResolveViewport` /
`style.ResolvePseudoElements`, which thread it down through `resolveNode` → `computeStyle` →
`applyCascadeEntry` → `applyProperty` → `parseFontFamily`/`parseFontShorthand`. Both maps are made
lazily, so a document with no `@font-face` - which is most of them, and all of the parity corpus -
allocates one empty struct and nothing else.

The index is now written *at the end of the cascade* rather than by the declaration that names the
family:

```go
func resolveCustomFont(cs *ComputedStyle, fonts *CustomFonts) {
    if cs.CustomFamily == "" { cs.CustomFontIdx = 0; return }
    cs.CustomFontIdx = fonts.index(cs.CustomFamily, cs.FontWeight >= WeightBold,
        cs.FontStyle != FontStyleNormal, cs.FontWeight > 0 && cs.FontWeight < WeightNormal)
}
```

because the answer depends on the element's final weight and slant, which no single declaration
owns. `computePseudoStyle` gained the same tail call so `::before` and `::after` resolve identically.

### Why eager resolution is the same behaviour, not a new one

The claim worth defending is that moving resolution into the cascade cannot change what anyone was
already drawing. `git show HEAD:internal/style/style.go | grep CustomFontIdx` returns exactly three
lines: the field declaration (298), the read in `FontSlot` (453), and `cs.CustomFontIdx =
parent.CustomFontIdx` in `inheritFromParent` (1101). Nothing ever *computed* it, so at HEAD the field
was 0 for every element - inheritance only ever copied a zero - which means the lazy global lookup
inside `FontSlot` ran unconditionally and its `if cs.CustomFontIdx != 0` shortcut was dead. Eager
resolution at the end of the cascade therefore reproduces the same query, on a table that now belongs
to the right document, plus variant matching the old single-key map could not express. The
inheritance copy was deleted as dead with the rest; `CustomFamily` still inherits, because the
family name is what inheritance is *about* and the index is derived from it per element - which also
closes a subtler case where a child that re-declares its own family would otherwise have kept a
parent's index.

### Tests, in Red-first order

1. `test/engine/custom_font_scoping_test.go` - the Red above; Green keeps all three assertions, so
   it pins both directions (A alone, B alone, A after B).
2. `test/engine/custom_font_variant_test.go` - new, characterises the selection rule against a
   `seqRegistry` that hands back 1, 2, 3 in document order: regular→1, `font-weight: bold`→2,
   `font-style: italic`→3, `font-weight: 900`→2 (bold is the heaviest face the family has). With
   only a regular face registered, all four→1, which is the fallback a browser owes you; with a
   family no `@font-face` declares→0. Its first run failed on all eight slots returning 0 for a
   different reason than the code: a `@font-face` rule registers a face but claims no text, so the
   fixture needed a `p { font-family: Face }` rule. Fixed in the test, which is the right way round -
   the code was not wrong, the page was.
3. Call-site updates only, and two deletions. `git grep applyDeclarations HEAD` shows the helpers
   (`style.go:1118`, `style.go:1163`) had exactly one caller outside each other -
   `internal/style/clip_test.go:15` - so both went as dead, and the clip test now loops
   `applyProperty` over the rule's declarations, which is what the helper did. The remaining edits
   are `Resolve` / `ResolveViewport` / `ResolvePseudoElements` call sites in `test/style`,
   `test/layout`, `internal/style` and `engine/refresh.go` gaining the fonts argument.

### Verification

```
go vet ./...                                   # exit 0
gofmt -l cmd internal test                     # silent
go test -count=1 ./...                         # exit 0, 28 packages ok, zero FAIL
go test -race -count=1 ./...                   # zero FAIL, zero DATA RACE (test/gate 252 s)
```

Pixel identity, which is the check this refactor owes: no file under `testdata/render` mentions
`@font-face` (`grep -rl "@font-face" testdata/render/` is empty), so every path the change touches is
inactive on the corpus - and the corpus was rendered anyway with the current binary into a fresh
directory and compared against the 56 PNGs measured at 12:46 today, i.e. before the first edit of
this round at 15:12:

```
rendered: 56 png
identical=56
changed:  (none)
```

The §G instrument, `BenchmarkDocumentLoad` (186 KB real page, `testdata/perf/gate_scroll.html`),
against §16's post-filter baseline:

| | ms/op | B/op | allocs/op |
|---|---|---|---|
| §16 after | 67.24 | 145,390,876 | 351,335 |
| this round, ×3 counts | 66.05 / 66.04 / 64.91 | 145,391,325 / 145,390,904 / 145,391,284 | 351,336 / 351,335 / 351,335 |

+16 to +449 bytes and 0 to +1 objects on 145 MB and 351 k objects, i.e. noise plus the one empty
`CustomFonts` struct per document load. `resolveCustomFont` returns immediately for an element that
names no custom family, so the per-element cost is a string compare, and the family lookup that used
to happen lazily per `FontSlot` call now happens once per element against a table that is usually
nil. Nothing here moved the load-path numbers, which is the expected result for a change whose whole
subject is a path the fixture never walks.

Coverage of the package that lost the globals, `go test -coverpkg=./internal/style ./...`: **83.4 %**
of statements, 104 functions, **zero at 0.0 %** - which retires §4's "16 functions at 0%" backlog
item (two of those sixteen were the setters this round deleted). Lowest now: `parseGridTemplateShorthand`
35.7 %, `resolveCurrentColor` 36.2 %, `gradientKeywordAngle` 37.0 %.

### Deferred, with the reason

- **`css.SetMediaViewportWidth` - the last one.** `internal/css/parser.go:11` holds
  `var mediaViewportWidth float32 = 1280`, set by `engine.go:199` on every load and read by @media
  evaluation at parser.go:458/462/662/664. It is the same defect class with a different subject: tab
  B's window width re-points tab A's `@media (max-width: …)` matches, and a resize of one tab
  invalidates another's conditionals. It is deferred rather than bundled because the fix is not the
  same shape - `css.Parse` bakes the value in at parse time, so the width has to reach the *parser*,
  not the cascade, and either the sheet cache becomes per-viewport or parsed sheets stop being
  shareable. That is a parser-interface round of its own, and after §20 the repo has one `Set*`
  global function left, not three.
- **`style.UserAgentStylesheet()`'s `sync.Once` cache** (`style.go:466`) is a global too, but a
  write-once memo of an immutable parse of a constant string: no document can change what another
  reads, so it fails the letter of §B and none of its intent. Left, and named.
- **Multi-document font rendering has no visual test.** The fix is proven at the API
  (`customIdx`), not at the pixel, because the parity corpus has no `@font-face` fixture and adding
  one is a fixtures change that the parity rules in project memory put out of bounds for this agent.
  A font-bearing fixture with a committed reference PNG is the way to close that, and it needs the
  harness, not a source edit.

## 22. Sixteenth round: the `@media` width one document set for every other one

### The hole, and why "deferred with a reason" undersold it

§21's Deferred list named `css.SetMediaViewportWidth` as the last mutable global and predicted the fix
would be about a sheet cache. Reading the call graph first showed a narrower change and two defects
that had not been stated:

```
internal/css/parser.go:11   var mediaViewportWidth float32 = 1280   # the only writer is NewSession
internal/engine/engine.go:199  css.SetMediaViewportWidth(viewportW)  # …at every document load
internal/css/parser.go:458/462  min-width / max-width          # read while parsing
internal/css/parser.go:662/664  the MQ4 range form (400px <= width < 1200px)
```

(`mediaDevicePixelRatio` is a `const`, so resolution never joined the problem.)

1. **A restyle never re-pointed it.** `Refresh` and `RefreshNode` take a `viewportW`, call
   `recordViewportWidth`, and then `css.Parse` - which decides every `@media` in the sheet against
   the width the *last `NewSession` anywhere in the process* happened to pass. So a resize or a
   restyle at a new width re-flows the page and re-runs the viewport units for that width while
   keeping the media matches of another one. That is deterministic, not a race, and it is user-visible:
   a narrow restyle of a document that loaded wide keeps the wide document's conditionals.
2. **Tab loads are off the UI thread.** `loadURLCtx` (`cmd/goosie/main.go:1239`) is invoked from
   `go func(){}` blocks at main.go:527 and :699, so the write at engine.go:199 and the reads in the
   parser are unsynchronised accesses to a bare `float32` from two goroutines. `-race` had reported
   nothing for fifteen rounds because no test had ever driven two concurrent loads.

### What this round built

The width travels with the parse, because `@media` is decided during parsing:

- `css.ParseForViewport(input, viewportWidthPx)`; `Parse` is kept and documented as *parse at the
  default desktop width*, which is a statement worth making only for sheets without `@media`.
- `parser` gained `mediaWidth`, and the condition chain - `mediaConditionTrue`, `mediaGroupTrue`,
  `mediaPartsTrue`, `mediaRangeHolds`, `widthSatisfies` - became parser methods rather than taking a
  width parameter through five signatures. Both nested parsers (`@media` bodies and `@supports`
  bodies) inherit it, which matters: an `@media` inside an `@supports` had been reading the global
  like everything else.
- `checkedSheets` gained a `viewportW` and passes it to every sheet parse; its single caller hands it
  the already-validated `viewportW` (`validateWidth` at engine.go:195 rejects non-finite, `<= 0` and
  over-bound widths before anything parses, so the parser does not re-check).
- `refresh.go`'s three parses use `s.viewportW` **after** `recordViewportWidth`, so a caller passing a
  garbage width gets the session's last good one for media matching instead of a poisoned default.
- `var mediaViewportWidth` and `SetMediaViewportWidth` are gone. What is left at package level in
  `internal/css` is three tables nothing assigns to after their initializers: `viewportTerm`
  (`calc.go:40`), `viewportTermHints` (`calc.go:45`) and `namedColors` (`value.go:408`).

What was *not* done: moving `@media` evaluation to resolve time against a `Viewport`. §21 raised it as
the alternative, and it is unnecessary - there is no parsed-sheet cache anywhere in the repo (the only
memo is the UA sheet, below), so every sheet is parsed once for one document at one width, and
evaluating at parse time is sound once the width is an argument. Lazy matching would move condition
evaluation into the cascade, rebuild the rule index lazily, and cost more than it buys.

### Tests, in Red-first order

1. `test/engine/media_viewport_scoping_test.go:32` - the refresh case, Red as
   `after restyling the same document at 500 px, p color = {R:0 G:0 B:0 A:255}, want {R:255 G:0 B:0 A:255}`.
   It asserts the negative first (1200 px must *not* match `max-width: 600px`), so the fixture cannot
   silently stop testing what it claims.
2. `…:55` - 24 concurrent loads alternating 400/1200 px, Red in both directions
   (`a document that loaded at 400 px does not match`, `a document that loaded at 1200 px matched`),
   and it is the first test that makes `go test -race ./test/engine/` report the write/read pair at
   `parser.go:21` × `engine.go:199` at all. Errors are `t.Errorf`, since `Fatalf` is illegal off the
   test goroutine.
3. `test/css/media_test.go` - `TestMediaWidthTravelsWithTheParseCall`, the package-level twin: one
   source, two widths, both orders, each parse keeps its own match. The existing five media tests kept
   their 1280 expectations and now state the width explicitly through the helper
   (`rulesTextAt(t, 1280, src)`) instead of setting a global and hoping the default matches.

### Why every existing figure had to be unchanged, and was

The change is invisible at exactly the widths the harnesses use, which is checkable rather than
asserted: no file under `testdata/render` contains `@media` (`grep -rl "@media" testdata/render/` is
empty), `parity.py:31` renders the fixture corpus at `-width 800` - the width its `NewSession` was
already putting in the global - and `parity_urls.py:43` renders the live corpus at `WIDTH, HEIGHT =
1280, 800`, which is the old default. Both corpora were therefore parsing at the same width before and
after; the difference shows up only for a second tab or a resized window, which is the bug.

```
rendered: 56 png, diff -r against the pre-change renders: identical, changed (none)
55/56 passing (98.2%), average 95.02%   # semantic/03-aside-figure is the known pre-existing FAIL
```

`BenchmarkDocumentLoad` (186 KB page), against §21's row:

| | ms/op | B/op | allocs/op |
|---|---|---|---|
| §21 (this round's starting tree) | 66.05 / 66.04 / 64.91 | 145,391,325 / 145,390,904 / 145,391,284 | 351,336 / 351,335 / 351,335 |
| this round, ×3 | 64.09 / 63.80 / 64.30 | 145,390,924 / 145,390,936 / 145,390,928 | 351,335 / 351,335 / 351,335 |

Allocation-neutral by exactly 0 objects in all three runs, and within the noise band on the low side:
one global store per load removed, one field per parser added. `TestDocumentLoadAllocationFence`
passes, so the §G figure did not move.

### Verification

```
go vet ./...                                   # exit 0
gofmt -l cmd internal test                     # silent
go test -count=1 ./...                         # exit 0, 28 packages ok, zero FAIL
go test -race -count=1 ./test/engine/ ./test/css/ ./internal/style/ ./test/style/   # ok
go test -race -count=1 ./...                   # exit 0, 28 packages ok, zero FAIL, zero DATA RACE (test/gate 243 s)
```

### Deferred, with the reason

- **A gate for "zero global state" would be the durable win, and it cannot be written yet.** The rule
  wanted is in `internal/archtest`: a package-level `var` that is assigned outside its own
  initializer or `init()` fails the build. That definition fails today on legitimate cases the repo
  depends on - `style.uaSheet` (`internal/style/style.go:463`, the `sync.Once` memo §21 named),
  `dom.atomLookup` / `dom.attrLookup` (filled by `init` from the atom tables) - and every one of
  those is write-once-after-init, which is the distinction the gate has to encode before it can be a
  fence instead of a list of exceptions. Deciding that exemption (init-only writes? `sync.Once`?
  unexported and immutable-after-freeze?) is the work, and it is its own round.
- **The UA stylesheet parses at the default width, once, ever** (`style.go:463`). `uaCSS` contains no
  `@media`, so this is inert today; the moment one is added, the memo freezes 1280 for every
  document. Now a documented latent limitation instead of a data race, and the reason `Parse` keeps
  existing.
- **No multi-document visual test for media matching** - the same boundary §21 hit: the fixture corpus
  has no `@media` file, and adding fixtures plus reference PNGs is harness work, not a source edit.
  The browser-level behaviour is pinned at the API and at `-race`.
- **`Refresh` still styles at the validated `s.viewportW` while laying out at the raw `viewportW`**
  (`refresh.go:32-34`, `:50-52`, `:82-84`): for a non-finite or `<= 0` width the two disagree. That
  predates this round and this round did not touch it; the media path is the one that had a global in
  it.
- Carried unchanged from §20/§21: the `<50MB idle` cap/geometry decision, no document-path pacing
  fence, `-screenshot` writes no artifact, `-tile-slack MIB` remains config-relative, the depth-aware
  split iterator, the deferred zoom paint-scale defect, and roadmap gate 6 (Goja JS runtime).

## 23. Seventeenth round: a build rule for "zero global state", with no exemption table

### The premise §22 left, and what measuring it showed

§22 deferred the durable §B fence on the belief that its definition "fails today on legitimate cases the
repo depends on", naming `style.uaSheet`, `dom.atomLookup` and `dom.attrLookup`. Those are three
examples, not a census, and the round that was supposed to be blocked turned out to need one `go/ast`
walk written as a scratch program. With the rule's own definition applied to the whole tree - *a
package-level `var` assigned anywhere outside its own initializer or `init()`, counting writes through
a map, slice or struct field* - `internal/` contained **exactly one** violation:

```
internal/style/style.go:463: uaSheet assigned in UserAgentStylesheet(): package-level var assigned outside its initializer and outside init()
```

`dom.atomLookup` / `dom.attrLookup` and `platform_darwin.go`'s `native` / `clipboardFactory` are
declared without an initializer and written only by `init()`, so the definition already accepts them:
the exemption §22 predicted would have to be invented is what the definition *is*. That reframed the
round from "design an allowlist" to "delete the last violation", which is what happened.

### What this round built

`archtest.MutableGlobals(dir)` (`internal/archtest/rules.go`) reports every offending write as a
`GlobalWrite{Pkg, File, Line, Fn, Var}` whose `String()` renders one locator line. Files are grouped
per package directory rather than analysed one at a time, because a package var is visible from every
file of its package - the setters rounds 15 and 16 deleted assigned vars declared in a *different*
file, and a file-at-a-time walk would have missed them. Test files are excluded: a test staging a
package var freely is not engine state. Dot-directories, `node_modules` and `testdata` are pruned for
`UnformattedGoFiles`' reasons, which matters concretely here: `.worktrees/phase1-document-lifecycle/`
is a stale second full checkout whose older copies of these files would otherwise be reported as this
tree's violations.

Names are the whole difficulty. A write counts only if its root identifier is a package var *and* is
not shadowed by anything the enclosing function binds: receiver, parameter, named result, `:=`, `var`,
range key, function-literal params and results, and the two-value-form `ok` are all locals for this
purpose. `_` is skipped, so `_ = f.Close()` - five occurrences in `cmd/goosie/` - assigns nothing.
A *method* named `init` on a type is an ordinary function and still flagged; only the package-level
`init` is the accepted writer.

Wired in twice, the way the other archtest rules are: `internal/archtest/globals_test.go` for the
rule's own behaviour, and `TestGate_ArchtestBoundaries` (`test/gate/m1_gate_test.go`) so the gate calls
the same function rather than a copy. `CONTRIBUTING.md` states the rule next to the cgo rule.

### Tests, in Red-first order

1. `TestMutableGlobalsFlagsWritesAndAcceptsInit` - the shape of the rule: `cached =`, `rows` appended
   and `settings[k] =` all flagged as `p.Warm.*`, while a var written only in `init()` and a `const`
   are not, and a write inside a `skip_test.go` is ignored.
2. `TestMutableGlobalsSeesAcrossPackageFiles` - `q.Bump.counter`, where `counter` lives in another file
   of the package. Red before the per-package grouping existed, and this is the case a "don't
   reassign globals" review comment most reliably misses.
3. `TestMutableGlobalsSkipsReceiverParamAndResultNames` - self-review caught a false-positive class:
   treating only the *body's* bindings as local makes a receiver named `rows`, a parameter named `rows`
   and a named result `rows int` each read as a write to the package-level `rows`. Red as
   `mutable globals = [r.Take.rows], want none` (receiver form) and, after dropping `fd.Type.Params`
   to isolate the case, `[r.Filter.rows]` (parameter form). Green with `fieldNames`, which also
   collapsed the FuncLit parameter loop. The reason to care is in the test comment: *a rule that
   reports writes nobody made is a rule people switch off*.
4. `TestMutableGlobalsPrunesExcludedDirs` - the `.worktrees` / `testdata` prune.
5. `TestInternalHasNoMutableGlobals` - the whole-tree assertion, which is the fence: it fails if
   anyone reintroduces a setter.

### Why `uaSheet` was deleted rather than allowlisted

`UserAgentStylesheet()` used to parse `uaCSS` inside a `sync.Once` and store the result in a package
var. The sheet is 4,529 bytes and ~68 rules with no `@media`, and `css.Parse` has been total and
stateless since §22, so the memo bought nothing an initializer does not: `var uaSheet =
css.Parse(uaCSS)` runs during package initialization, before `main`, and `UserAgentStylesheet()` is a
`return uaSheet`. Two consequences worth owning:

- Startup now pays the UA parse unconditionally, including for a `--help` invocation. Measured below,
  and it is inside the existing band.
- The UA-origin check in `matcher.go` compares `sheet == uaSheet` by pointer identity, which the
  change preserves - one parse, so one address.

§22's latent-limitation note is sharper rather than gone: the sheet parses once, ever, at `css`'s
default width, so adding `@media` to `uaCSS` would freeze 1280 for every document. That is now a
documented limitation with a rule behind it instead of a data race.

### Cmd, test, and the shapes the walk cannot see

The same rule over `cmd/` and `test/` reports **0 violations each**, so the whole module - not just
`internal/` - has no mutable package state after init. What a source-text rule cannot see, checked by
hand on 2026-09-27 and recorded here because the gap is permanent:

- **A write through an address a package var handed away.** `return &counter` escapes the var and the
  rule stays silent. Every `return &` of a package-level address in `internal/` was audited; there is
  none. The live candidate is `UserAgentStylesheet()` itself, which hands out a `*css.Stylesheet`
  globally. `matcher.go:49-76` shows what that does and does not reach: `buildRuleIndex` allocates a
  fresh `ruleIndex` per cascade (its two callers are `style.go:615` and `pseudo.go:22`, and the
  mutable `mark` stamp lives on the index's own entries), so it is not global state - but it stores
  `rule := &sheet.Rules[i]`, so pointers into the process-wide sheet travel with every candidate the
  cascade sees. Nothing writes through them today: `style.go:712/720` and `pseudo.go:69` read
  `cand.rule.Declarations`, and the only assignments to `rule.Selectors` / `rule.Declarations` are at
  `internal/css/parser.go:792-797`, inside the parse that builds that sheet. The blind spot is real
  and currently unexploited, which is an audit result rather than a guarantee.
- **A cross-package write into an exported var.** The alias surface this would need is enumerable, and
  small: the module declares exactly **8** exported package-level vars, of which **6 are immutable
  `error` values** (`net.ErrResponseTooLarge`, `net.ErrBlockedAddress`, and four in `raster`:
  `ErrNilDestination`, `ErrTilePanic`, `ErrNoRasterizer`, `ErrNoViewport` - `errors.New` hides its
  fields, so nothing outside the package can mutate one) and **2 are reference-typed tables in
  `archtest` itself** (`Allowed`, the import-boundary map; `FreeDirs`) - harness configuration, not
  engine state. Zero writes into any of them from another package: qualified assignments to exported
  `internal/` vars appear only as writes through receiver fields. The caps in
  `internal/engine/limits.go` are consts and the rule-truncation counter at `:559-612` mutates a
  per-call struct, not a global.

### Numbers

`uaSheet`'s memo removal was the only engine-path change, so this round is expected to be
allocation-neutral and visually inert. `BenchmarkDocumentLoad` (186 KB page), against §22's row:

| | ms/op | B/op | allocs/op |
|---|---|---|---|
| §22 (this round's starting tree) | 64.09 / 63.80 / 64.30 | 145,390,924 / 145,390,936 / 145,390,928 | 351,335 / 351,335 / 351,335 |
| this round, ×3 | 63.77 / 64.48 / 64.22 | 145,390,885 / 145,390,887 / 145,390,902 | 351,335 / 351,335 / 351,335 |

Exactly 0 objects in all three runs, ~40 B/op lower, and inside §22's spread on milliseconds.
`TestDocumentLoadAllocationFence` passes, so §G's figure did not move.

Cold start, `-gate -frames 1`, three runs each. The headless column is a *different* measurement from
§15's 61.8 ms, which came from a 120-frame run and is not comparable to a 1-frame figure, so the two
are reported separately rather than as a delta:

| | native ms | headless ms |
|---|---|---|
| §15 band | 179-215 | - |
| this round, ×3 | 177.4 / 150.7 / 154.7 | 31.2 / 31.2 / 31.7 |

The 1000 ms budget holds with ~6× headroom and the eager UA parse did not push native startup outside
the existing band - at the low end it improved it.

Render parity (56 fixtures at the harness's 800 px): `55/56 passing (98.2%), average 95.02%`, the same
figures as §22, with `semantic/03-aside-figure` the known pre-existing FAIL. Stronger than the score:
`diff -rq` against this round's starting-tree renders is **identical** (exit 0, no changed files), so
the one path that could have moved pixels provably did not.

### Verification

```
go vet ./...                                   # exit 0
gofmt -l cmd internal test                     # silent
go test -count=1 ./...                         # exit 0, 28 packages ok, zero FAIL
go test -race -count=1 ./...                   # exit 0, 28 packages ok, zero FAIL, zero DATA RACE (test/gate 243 s)
```

### Deferred, with the reason

- **Closing the escaped-address blind spot for the shared UA sheet is the durable follow-up.** The
  audit above shows nothing writes through the `*css.Rule` pointers that `buildRuleIndex` copies out of
  the global sheet, but that is a fact about today's code and the new rule cannot see either way.
  Making it structural - unexported `Rules`/`Declarations` so a write outside `internal/css` fails to
  compile, or a per-document copy - is a wider change than one round's fence, and it is the difference
  between a guarantee and a review habit.
- **The gate walks `internal/` only; the rule can walk any dir.** `cmd/` and `test/` measure clean
  today, so widening the gate to `cmd/` is a one-line change. `test/` stays out on purpose: tests
  mutate package vars as fixture staging, and exempting `_test.go` per file while still failing on a
  non-test helper in `test/` is worth less than the complexity it adds.
- **No visual evidence for the UA-sheet change beyond byte-identity on the corpus** - the same boundary
  §22 hit. `-screenshot` still writes no artifact, so the proof above is the 56-fixture diff, not a
  live page.
- Carried unchanged from §20/§21/§22: the `<50MB idle` cap/geometry decision, no document-path pacing
  fence, `-tile-slack MIB` remains config-relative, the depth-aware split iterator, the deferred zoom
  paint-scale defect, no multi-document visual test for media matching, `Refresh` styling at the
  validated width while laying out at the raw one, and roadmap gate 6 (Goja JS runtime).

## 24. Round 19: fuzz targets for the two entry points that eat untrusted bytes

Numbering note: §-numbers and round numbers now differ by one. §23 was round 17; round 18 - extending
`MutableGlobals` to flag `&pkgVar` - was passed over on purpose, and why is in the deferred list below.

### The clause still open when §23 closed

§D asks for unit tests "for the HTML parser and CSS rule matcher against edge cases (malformed HTML)"
and §E for "safe handling of malformed input". Both had been satisfied by hand-written cases, which is
what a table-driven test can be. The read-only audit that round 17 finished with found **no fuzz target
anywhere in the repository**, so nothing here searched for the malformed input nobody had thought to
write. That is the gap this round closes, and it is also the dynamic half of the §23 story: the static
rule cannot see a write through an address a package var handed away, and a determinism fuzz target can.

### What this round built

| Target | Package | Unit under test | Assertion beyond "no panic" |
|---|---|---|---|
| `FuzzParseDocument` | `test/dom` | `dom.ParseBounded`, the only HTML entry the engine calls | walked node count and root-to-node path length within the limits |
| `FuzzParseStylesheet` | `test/css` | `css.ParseForViewport` | parse `a`, parse an unrelated `b`, parse `a` again - the two parses of `a` must agree |
| `FuzzResolveCascade` | `test/style` | `style.ResolveViewport`, and with it `buildRuleIndex` | same `a`/`b`/`a` shape, plus every element in the tree must receive a computed style |

Three design choices are load-bearing enough to be worth stating with their reasons.

**The limits are tiny, not the engine's.** 64 nodes and depth 4 rather than `MaxDocumentNodes` 50000 and
`MaxDocumentDepth` 128, because a bound the fuzzer can only cross after 50000 tokens is a bound it will
never cross. The engine's real caps keep their own fence in `test/engine/bounds_test.go`, which is where
a wrong *value* for a cap would show up; this target is looking for a cap that can be escaped.

**The determinism check needs a second input.** Parsing `a` twice and comparing proves only that the
parser is a function of its own argument. Parsing `b` in between is what turns it into a claim about
process state, and it is the same claim §23 makes statically. The cascade target exists for a specific
shape the static rule cannot reach: `buildRuleIndex` dedupes a query with a stamp counter
(`matcher.go:121`), and a stamp that outlived the index it belongs to would mis-collect selectors on the
*second* document - which is exactly what `a`/`b`/`a` asks for.

**Seeds are `f.Add`, not `testdata/fuzz/`.** A seed written as Go source is reviewable and diffable; the
directory format is for repro files, and there were none to write. When a search finds one, the
instruction in `CONTRIBUTING.md` is to copy the generated file into `<pkg>/testdata/fuzz/<Target>/`,
which turns a finding into a permanent seed without anyone retyping it.

### Numbers

75 s per target on this machine (10 cores), `go test -fuzz=... -fuzztime=75s`. The three searches ran one
after another, but `gofmt`, `go vet` and the full suite were run over the tail of the last one, so the
cascade rate below is a floor rather than a clean measurement:

| Target | execs | execs/sec | corpus after the run | crashers |
|---|---|---|---|---|
| `FuzzParseDocument` | 33,302,802 | ~444k | 524 (6 seeds + 518 new) | 0 |
| `FuzzParseStylesheet` | 20,053,449 | ~267k | 1,348 (6 seeds + 1,342 new) | 0 |
| `FuzzResolveCascade` | 3,202,234 | ~43k | 1,169 (4 seeds + 1,165 new) | 0 |

All three PASS: no panic, no limit escaped, no cross-sheet state bleed found in roughly 57 million
executions. The nightly job gets 120 s per target, which at those rates is about 53M, 32M and 5.1M
executions. The cascade is an order of magnitude more expensive per exec than either parser - it is a
whole-document walk against an index built from both sheets - and that is where the coverage is thinnest.

### Verification

- `go test -count=1 -run 'Fuzz' ./test/dom ./test/css ./test/style`: all seeds pass individually.
- `go test -count=1 ./...`: exit 0, 28 packages `ok`, no FAIL and no panic, so the seeds cost the fast
  suite milliseconds and the searching stays where it belongs - the nightly job.
- `gofmt -l test/dom test/css test/style`: silent. `go vet` on the same three: exit 0.
- The nightly step was dry-run under `bash` with `go test` replaced by `echo` before it was trusted,
  which is what caught the defect described below.

### The bug this round found was in its own CI step

Two, actually, and both are the same failure mode §6 spent a round removing from `release.yml`: a step
that reports success while doing nothing.

- The first attempt looped over `"test/dom FuzzParseDocument"` with `set -- $pkg`. Under zsh an
  unquoted `$pkg` does not word-split, so `$1` became the whole string and `$2` empty, `go test` was
  handed a directory path with a space in it, and every iteration failed with `directory not found`
  while the loop itself exited 0. A green check on a step that never compiled anything.
- The rewritten loop passed `${target%%:*}`-style splits, which work in both shells, and its dry run
  then printed `go test ... ././test/dom`: the loop prefixed `./` onto targets that already had it.
  Harmless to `go test`, wrong to read, and it would have shipped to CI unseen had the dry run not been
  run first. Fixed to pass `"$pkg"` as written.

The `pkg:name` form is what `nightly-bench.yml` now uses, and it is the form that was actually executed.

### What a clean run does not prove

The document target returns early when `ParseBounded` refuses the input, so for a refused document the
node-count and path-length walk never runs. At limits this small, a real share of the 33M executions are
refusals - the fuzzer spends part of its time finding inputs *too big* for 64 nodes, and every one of
those is a vacuous pass. "0 crashers" therefore means "nothing escaped that the harness could see", not
"the bounds hold". Tightening it means measuring the refusal rate, which is the first deferred item.

### Deferred, with the reason

- **Measure how much of `FuzzParseDocument` is vacuous.** Instrument the target to count refused versus
  accepted inputs over a fixed search, then either raise the limits until refusals are a minority or
  assert against the partial tree the builder built before it refused. Until that number exists, the
  bound-checking claim this round makes is weaker than its exec count suggests.
- **Round 18 waits, and this round is why.** The `&pkgVar` extension to `MutableGlobals` measures zero
  occurrences across `internal/` today: it would formalise a shape nobody writes. Fuzzing closed a brief
  clause that had no harness at all, in code that runs on every untrusted byte the browser reads. Given
  one round, the second is the larger gap. The extension and the copy-of-a-reference-typed-global limit
  it cannot fix are still the next item on the global-state line.
- **Nightly coverage does not accumulate across runs, probably.** The corpus lives under `$GOCACHE/fuzz`
  and `setup-go` caches `GOCACHE`, so a restored corpus may carry over - untested, and not assumed by
  anything here. Each nightly therefore starts from the checked-in seeds, which is deterministic and is
  what the numbers above measure. If a finding is ever committed to `testdata/fuzz/`, it joins the seeds
  and the question answers itself.
- **`dom.Parse` is an exported, unchecked parse entry with zero non-test callers.** Every fuzz target in
  this repo now aims at the bounded API, which makes the unbounded one an open door a caller can still
  walk through. Deleting it in favour of a test helper over `ParseBounded` is a small, self-contained
  round; leaving it needs a comment that says who it is for.
- **`style.resolveNode` compares `n.Type == 2`** where `dom.NodeText` exists. Found while reading the
  cascade for this round's target; it is a one-token idiomatic fix (§E) in a file this round only
  touched by reading, so it did not belong in the change.
- Carried unchanged from §21/§22/§23: the `<50MB idle` cap/geometry decision, no document-path pacing
  fence, `-tile-slack MIB` remains config-relative, the depth-aware split iterator, the deferred zoom
  paint-scale defect, no multi-document visual test for media matching, `Refresh` styling at the
  validated width while laying out at the raw one, no structural immutability for the shared UA sheet,
  and roadmap gate 6 (Goja JS runtime).

---

## 25. Round 20: the fuzz target that checked its bounds on a minority of inputs, and the door it was standing in front of

### The two clauses §24 opened

§24's deferred list starts with the sentence this round exists for:

> **Measure how much of `FuzzParseDocument` is vacuous.** Instrument the target to count refused versus
> accepted inputs over a fixed search, then either raise the limits until refusals are a minority or assert
> against the partial tree the builder built before it refused. Until that number exists, the bound-checking
> claim this round makes is weaker than its exec count suggests.

and:

> **`dom.Parse` is an exported, unchecked parse entry with zero non-test callers.** … Deleting it in favour of
> a test helper over `ParseBounded` is a small, self-contained round.

Both are closed. One of them found a bug, and it was not the bug anyone was looking for.

### The number, measured

A throwaway probe ran the 4000-input synthetic corpus against five limit sets and recorded how many inputs
`ParseBounded` *accepted* — the only ones the old target asserted anything about, since it returned early on
an error:

| limits (nodes / depth / attributes / attribute bytes) | accepted | share that reached an assertion |
|---|---|---|
| 64 / 4 / 3 / 16 — round 19's | 1,109 | **27.7%** |
| 128 / 8 / 4 / 32 | 2,091 | 52.3% |
| 256 / 16 / 6 / 64 | 3,984 | 99.6% |
| 1024 / 32 / 8 / 128 | 4,000 | 100% |
| 8192 / 128 / 64 / 4096 | 4,000 | 100% |

So §24's "33,302,802 executions" is honestly about 9.2 million bound checks, and 72% of the fuzzer's time
went to finding inputs too big to test. The node limit is not what refuses anything at these sizes — depth
and the attribute counts are, which is worth knowing before raising `Nodes` to "fix" it.

The probe was throwaway and is gone. What stayed is the assertion the second option asked for.

### Refusals now carry an assertion

`assertParse` puts every input in exactly one bucket, and each bucket has a check rather than a return:

| bucket | claim |
|---|---|
| `boundChecked` | the tight parse accepted, so the tree is walked: node count, root-to-node depth, attribute count and attribute bytes, all within `tightLimits` |
| `refusalExplained` | the tight parse refused, so the same bytes parse under `looseLimits`, and that tree *must* break one of the tight bounds — otherwise the refusal was not the accounting firing |
| `refusalDropped` | an attribute refusal whose loose tree breaks no bound, because the tree builder threw the token away (see the finding below) |
| `refusedBoth` | refused under both limit sets; the assertion thins to "both errors name a limit" |

`TestParsePathCoverage` runs the same function over the fixed corpus and fails if either main bucket empties
(one-sided target), if `refusedBoth` exceeds 10% (loose limits too tight), or if `refusalDropped` exceeds 5%
(the weakest escape getting common). The current split is 44.4% / 55.6% / 0% / 0%.

**Instrumenting a live search does not work, and that is worth recording.** The first attempt counted buckets
in `atomic.Int64` vars and printed them from `f.Cleanup`. Over a 90 s search the print said "4000 execs" while
the fuzzer reported 4,768,757: `-fuzz` runs the target in *worker processes*, so the parent's counters see
only the parent's executions. There is no in-target way to measure a search's coverage split, which is why
the corpus is what the numbers come from.

### The finding: an oversized attribute that never existed

The first search after the change failed in about four seconds:

```
parse of "<0><BodY 000\x87\x87" refused the tight limits {24 8 4 8} with "HTML attribute byte limit exceeded (8)",
but the same input parses under {4096 64 32 2048} as 3 nodes at most 2 deep with at most 0 attributes and a
0-byte attribute, which breaks none of the tight bounds
```

Root cause, traced rather than guessed: `<0>` opens a stray element, which implicitly creates `<body>`; the
following `<BodY …>` is then a *second* body start tag, which the tree builder ignores outright — attributes
and all. The tokenizer had already refused the document while scanning those attribute bytes, so the parser
reports a bound violation for a tree that retains nothing oversized. Parsed alone, the same tag does carry
the attribute, and the loose tree then does break the bound.

The parser is right and the test's claim was too wide. An attribute limit is a bound on what the tokenizer
scans, not only on what survives; refusing early is the §E-correct behaviour for an oversized payload even
when the element was going to be dropped. So the escape is in the harness (`refusalDropped`), the parser was
not touched, and the input is pinned under `test/dom/testdata/fuzz/FuzzParseDocument/` as a permanent seed.
Node and depth refusals deliberately keep the strict form: a bound the parser hit has to be present in the
tree it stopped building, and if a search ever contradicts that, the escape widens with a reason rather than
by default.

### The limits were chosen by mutation, not taste

A harness that asserts is only as good as its ability to fail, so each bound check in `internal/dom` was
disabled once (`parser.go` copied to `/tmp`, restored, `shasum` compared — `git checkout` was not usable
because the file already held this round's edits):

| bound disabled | tightest config that catches it | failures on the 4000-input corpus |
|---|---|---|
| node count | `24 / 8 / 4 / 8` | 311 |
| depth | same | 121 |
| attribute count | same | 764 |
| attribute bytes | same | 4 |

At round 19's `64 / 4 / 3 / 16`, and then at `128 / 8 / 4 / 32`, **disabling the node limit and the
attribute-byte limit both left the suite green**: a 40-token corpus cannot reach 128 nodes, and no token in
it has an attribute over 32 bytes. The final limits are the ones that make all four checks reachable, and
attribute bytes is still the thinnest of the four by two orders of magnitude.

### `dom.Parse` is gone

`ParseBounded` is now the only exported parse entry in `internal/dom`; the doc comment on `ParseLimits` says
so in the imperative ("Every field must be positive: there is no unlimited parse in this package"). Twelve
fixture call sites across `test/layout`, `test/style`, `test/dom` and `internal/style` moved to
`domtest.Parse` (`test/domtest`), which parses under the engine's real caps and panics on refusal — so the
accounting paths that the deleted function skipped now run for every fixture in the repo. Deleting `Parse`
also removed the two branches that existed only to serve it: `allowNode`'s `Nodes == 0` "unlimited" case and
`insertText`'s nil-map fallback.

Two things took a turn each to get right:

- `domtest` cannot import `internal/engine` to read the caps it mirrors — engine imports style imports dom
  imports nothing, and the cycle closes through any test that touches both. The caps are literals in
  `domtest`, and `TestFixtureLimitsTrackEngineCaps` asserts they equal `engine.MaxDocument*` from a package
  that may legally import both.
- The fence behind the deletion is `test/dom/entry_test.go`, which reads function *signatures* in
  `internal/dom` rather than names: exported, receiver-less, takes a string, returns `*Document`, no error.
  That is what makes `MustParseHTML` fail the same test as `Parse` did, and `NewDocument` pass although it
  returns a document — it takes no input. It went red on `parser.go:Parse` before the deletion landed, which
  is the only reason it is trusted.

### Numbers

| measurement | value |
|---|---|
| coverage split on the 4000-input corpus | 44.4% checked / 55.6% explained / 0% dropped-token / 0% refused-both |
| `FuzzParseDocument`, 90 s, final tree | 27,681,936 execs, ~302k/sec, 0 crashers |
| round 19's same target, 75 s | 33,302,802 execs, ~444k/sec, of which 27.7% asserted anything |
| seeds now in `testdata/fuzz/` | 1 (`cac19d5d58bb4ab6`, the dropped-attribute finding) |

The rate dropped by ~32% and that is the point: every refused input is now parsed a second time and walked
twice, which is work a vacuous target did not do.

### Verification

- `go test -count=1 ./...`: exit 0, no FAIL.
- `gofmt -l test internal cmd`: silent. `go vet ./test/dom ./test/domtest ./internal/dom`: exit 0.
- `go test -count=1 -v -run TestParsePathCoverage ./test/dom`: the split above, PASS.
- `go test -count=1 -run 'FuzzParseDocument/cac19d5d58bb4ab6' ./test/dom`: fails on the pre-escape target,
  passes on the current one.
- Four bound mutations, each `CAUGHT`, each restored; `shasum` of `parser.go` identical to the pre-mutation
  copy afterwards.
- `go test -fuzz='^FuzzParseDocument$' -fuzztime=90s ./test/dom`: PASS, 0 crashers.

### What a clean run does not prove

- It does not prove the *engine's* caps hold — those are 50000 nodes deep, and this target's limits are 24.
  `test/engine/bounds_test.go` owns the values; this owns the mechanism.
- It does not prove `refusalDropped` is right in general. It is justified by one traced input and by the
  spec-shaped behaviour it shows (ignored second `<body>` drops its attributes); its guard is that it stays
  under 5% of the corpus.
- The node and depth refusals are untested against a dropped-token counter-example. None exists yet.
- `FuzzParseStylesheet` and `FuzzResolveCascade` were not re-run this round; their claims are §24's.

### Deferred, with the reason

- **Local corpora do accumulate, CI's probably do not.** This machine's corpus went 546 → 702 entries across
  the searches in this round, so a new search starts from the last one's findings. Whether `setup-go`'s
  `GOCACHE` cache gives the nightly runner the same continuity is still unmeasured, and §24's numbers were
  taken from the checked-in seeds either way.
- **Round 18 (`&pkgVar` in `MutableGlobals`) is still open** — task #17. This round displaced it because the
  vacuity question had a number attached to it and the alias question does not yet have a case.
- **`style.resolveNode` compares `n.Type == 2`** where `dom.NodeText` exists. Still a one-token fix in a file
  this round only read.
- Carried unchanged from §21/§22/§23/§24: the `<50MB idle` cap/geometry decision, no document-path pacing
  fence, `-tile-slack MIB` remains config-relative, the depth-aware split iterator, the deferred zoom
  paint-scale defect, no multi-document visual test for media matching, `Refresh` styling at the validated
  width while laying out at the raw one, no structural immutability for the shared UA sheet, and roadmap
  gate 6 (Goja JS runtime).

---

## 26. Round 21: "<50MB idle", measured as a resident set for the first time

### Why this round exists

The brief's headline target is `<50MB idle`, and §8's first-round note admitted in its own title that the
program had "not established a total-process RSS/CPU bound". Since then the only footprint fence has been
`test/gate/footprint_gate_test.go`, which is deliberately **capacity-relative** - retained heap minus the
tile budget and the surface rings - and whose header says the absolute figures, "along with the process
resident-set numbers", are recorded in this document. They were not. The resident set had been measured
exactly once (§8, peak, two geometries, never gated), and no number in the repo was comparable to 50MB.

So: `scripts/idle-footprint.sh`, which builds `cmd/goosie-headless` and reports peak resident set size per
configuration, optionally fencing it (`scripts/idle-footprint.sh 60` fails above 60 MiB). Measured on this
machine, 2026-09-27:

| config (fixture, viewport, DPR) | peak max RSS |
|---|---|
| tables, 320x480 @1 | 37.2 MiB |
| blank, 800x600 @1 | 40.0 MiB |
| tables, 800x600 @1 | 43.4 MiB |
| tables, 1440x900 @1 | 58.1 MiB |
| tables, 1440x900 @2 | 130.8 MiB |

### The verdict, which is a boundary rather than a yes or a no

`<50MB idle` **holds up to roughly one megapixel of device surface and fails past it.** 800x600 at DPR 1 is
480k device pixels and lands at 43.4 MiB; 1440x900 at DPR 1 is 1.3M pixels and lands at 58.1; doubling DPR
quadruples the surface and gives 130.8. These are peaks over a one-shot render, so they bound the true idle
figure from above - a browser sitting still cannot exceed its own render peak - and the claim is therefore
as close to conservative as this harness allows.

That is a config decision, not an engineering failure, and it is handed back rather than resolved here: the
number is dominated by `width × height × dpr² × 4 bytes` times the number of retained surface rings. To
make `<50MB idle` true at a laptop viewport, either DPR 2 has to shed a ring, the viewport has to be capped,
or the target has to be restated as "under 50MB below 1M device pixels", which the table above is evidence
for and which nobody has yet approved.

### Two ways this measurement lies if you are not careful

- **A failed run reports a pass.** The first sweep in this round produced a beautiful "5.4 MiB at every
  geometry": the binary had exited immediately on a missing input. `/usr/bin/time` does not care whether its
  child succeeded. The script now treats a non-zero exit as fatal and prints the child's message.
- **`/usr/bin/time -l` prints two memory figures, and they are not the same quantity.** "maximum resident
  set size" is the OS-visible footprint the target is about; "peak memory footprint" is what the kernel
  tracked for the address space, and for the 1440x900 @2 row here it read 128.1 MiB against 130.8 MiB of
  resident. The gap is small at one geometry and meaningless to extrapolate, so the script reads only the
  resident column, and §8's 245.2 MiB - a footprint figure from a different harness - is not comparable to
  the table above in either direction.

### The bug this round found, and did not fix

Rendering a 200 KB single-word text run (`<p>` + 200k base64 characters) at 1440x900 fails outright:

```
goosie-headless: build session: layout object 9 geometry must be finite with absolute value <= 1048576 CSS pixels
```

The guard is correct and the bound is honest, but the *consequence* is that one unbreakable word kills the
whole document rather than overflowing one box. A real equivalent is a long URL or a base64 blob pasted into
a `<pre>`-less container. Triage needs a minimal in-repo fixture before any claim about which layer should
soft-wrap; that is its own round.

### Verification

- `bash scripts/idle-footprint.sh` - the table above, exit 0, five configurations.
- `bash scripts/idle-footprint.sh 45` - bites at the first row above 45 MiB:

  ```
  blank 800 x 600 @dpr1                  39.8 MiB      1s
  tables 320 x 480 @dpr1                 37.4 MiB      0s
  tables 800 x 600 @dpr1                 43.3 MiB      0s
  tables 1440 x 900 @dpr1                57.5 MiB      0s
  regression: tables exceeds 45 MiB
  exit=1
  ```

  The four figures reproduce the table above within ~1%, which is the run-to-run noise band a fence on this
  number has to allow for.
- No CI wiring, on purpose: these figures are machine- and geometry-relative, so a nightly threshold is the
  same cap decision as the one above and should not be smuggled in through a YAML file.

### Deferred, with the reason

- **Idle RSS for the native GUI is still unmeasured.** The honest way is a running `cmd/goosie` with a blank
  tab and no navigation, sampled over seconds; that needs a display, and CI must pass without one
  (CONTRIBUTING). The headless numbers bound the engine's contribution, not AppKit's.
- **`-tile-slack MIB` remains config-relative** (§21), and now has a sibling question: the same
  absoluteness-vs-relative-ness argument applies to every figure in the table above.
- **Round 18's `&pkgVar` extension to `MutableGlobals`** is still the next item on the global-state line
  (task #17); round 20 displaced it, round 21 did not touch it.
- **`style.resolveNode` compares `n.Type == 2`** where `dom.NodeText` exists - still a one-token fix.
- Carried unchanged from §21-§25: no document-path pacing fence, the depth-aware split iterator, the
  deferred zoom paint-scale defect, no multi-document visual test for media matching, `Refresh` styling at
  the validated width while laying out at the raw one, no structural immutability for the shared UA sheet,
  nightly corpus continuity unverified, and roadmap gate 6 (Goja JS runtime).

## 27. Round 22: one unbreakable word no longer refuses the whole document

### The defect round 21 found and handed over

`<html><body><p>` + 300,000 characters with no space, rendered at 1440x900, was not slow and not ugly.
It was *refused*: `build session: layout object 9 geometry must be finite with absolute value <= 1048576
CSS pixels`. One text node denied the entire page, and the same shape occurs in the wild as a pasted
base64 blob or a very long URL. §E asks for "safe handling of malformed input"; a bound that converts a
pathological string into a blank document is safe but not acceptable, so the guard was doing its job and
the layer feeding it was not.

### Root cause

`internal/layout/inline.go` gives a word object `W = measureWord(word)`, which is unbounded in the word's
length, and `engine.MaxTextRunes` allows 1<<20 runes per document while `engine.MaxGeometry` allows 1<<20
*pixels* of width. The two caps are the same number in different units, and a rune is worth several pixels
of advance, so the text bound permits roughly five times more width than the geometry bound can tolerate.
The gap was structural, not a missing check: nothing had ever bounded a word's width.

### The fix, and what it deliberately does not change

`splitOverlongWords` breaks a word into rune chunks only when its measured width exceeds
`maxWordFragment = 1 << 16` CSS pixels, accumulating advances with the same arithmetic `measureWord` uses
so a chunk the splitter calls narrow is still narrow where the line box places it. Every token below that
width is copied through untouched, which is the point of the threshold: an ordinary word that merely
overflows its line keeps Chromium's overflow behaviour and cannot move a parity fixture, while a word far
wider than any viewport - which no one can scroll to read either way - stops being able to kill a document.
The advance accumulator makes one rune per fragment when a single rune cannot fit, so the split always
terminates.

The divergence is stated rather than hidden: CSS `overflow-wrap` defaults to `normal`, under which Chrome
lays the long word on one overflowing line. This engine breaks it. That is a deliberate availability over
fidelity choice for a case where fidelity is unreadable, and `validateArena` stays exactly as strict as it
was as the backstop for anything else.

### Verification

- Test-first. `test/engine/longword_test.go` was written before the implementation and failed with the
  production error verbatim: `NewSession refused a document that only contains one long word: layout
  object 6 geometry must be finite with absolute value <= 1048576 CSS pixels`. It now passes, asserting
  three things: the session builds, the 300,000 runes are all still present across fragments, and no
  fragment is wider than `engine.MaxGeometry`.
- Real binary, same input as the repro: `goosie-headless -in longword.html -out out.png -width 1440
  -height 900 -dpr 1` exits 0 and writes a 5,942-byte PNG at `presents=2`. Before the change that command
  printed the refusal and produced nothing.
- `go test -count=1 ./...`: 28 packages `ok`, no FAIL line. `gofmt -l internal/layout test/engine`: silent.
  `go vet ./internal/layout ./test/engine`: exit 0.

### Deferred, with reasons

- No parity fixture for a >65536px word. A reference PNG for it would have to come from Chromium, and
  fabricating one is out of the question; the change is covered by the engine test instead.
- `maxWordFragment` is a judgement call handed over rather than tuned: it is 64x a wide desktop viewport
  and 1/16 of the guard, which is defensible but is not derived from a spec or a measurement.
- The unit mismatch between `MaxTextRunes` and `MaxGeometry` is now mitigated at the layout layer but still
  exists at the bound layer, where a document of many *separately bounded* words can still accumulate width
  through `x += wordW`.
- `internal/layout/inline.go:42` and `:210` compare `k.Node.Type == 2` instead of `dom.NodeText`, the same
  magic number already noted in `style.resolveNode`.

## 28. Round 23: the global-state rule now sees `&pkgVar`

### Why this item was queued and then twice displaced

§B's "zero global state" has been enforced statically since round 17, and the rule shipped with a
documented blind spot: a write through a pointer that a package var handed out (`return &counter`) is
not an assignment to the var, so nothing in the file names the var being mutated. Round 18 was meant to
close it, was passed over for round 19's fuzz harnesses, then for round 20's vacuity measurement, then
for round 21's footprint and round 22's layout defect. The recorded reason each time was the same: the
alias question "does not yet have a case". That is true and was never a reason to skip the fence - a
regression rule is bought for the violation that has not happened yet.

### What was added

`globalWrites` now reports an `*ast.UnaryExpr` whose operand resolves to a package-level var of the same
package, with the same local/receiver/result exclusions the assignment scan already applies, as a
`GlobalWrite` with `Escape: true` rendered as `file:line: &var taken in fn()`. A var's *field* counts
(`&cfg.N` escapes `cfg`), a local's does not, a receiver field's does not, and `init()` stays exempt for
the same reason it is exempt for assignment: after start-up that var is configuration, not state.

### The number this round produces

`internal/` contains **zero** escaped package vars today. The repo-wide check (`TestInternalHasNoMutableGlobals`)
passes unchanged, so the extension adds no exemptions and fixes no existing code - it converts the one
shape the rule was documented as blind to into the shape that now fails CI. The remaining documented gap
is the heavier one: a write from *another* package into an exported var, which needs whole-program alias
analysis and is deliberately not attempted here.

### Verification

- `TestMutableGlobalsFlagsAddressOf` (new fixture, non-vacuity case): a package with `return &counter`,
  `return &cfg.N`, `return &x` (local), `return &t.v` (receiver field), `return counter` (read only) and
  `p = &counter` inside `init()` reports exactly `e.Field.cfg e.Handle.counter` - two findings, not zero
  and not six.
- `go test -count=1 ./internal/archtest/`: ok, including the repo-wide scan and `TestRulesAreNotVacuous`.
- `go vet ./internal/archtest/`: exit 0. `gofmt -l internal/archtest`: silent.
- `go test -count=1 ./...`: no FAIL line, 29 packages ok. Round 22's run on this same tree reported 28; the
  count is recorded as measured here, and the extra package is in the untracked working tree rather than
  something this round added.

## 29. Round 24: layout, paint and the cascade ask `dom.Node` what it is

### The clause this covers

Brief §B asks for "production-ready, idiomatic Go". This is the only part of that clause still open
without a config decision attached to it: `internal/dom/node.go:65-68` already defines
`func (n *Node) Element() bool { return n.Type == NodeElement }` and
`func (n *Node) Text() bool { return n.Type == NodeText }`, and `NodeText` is the third `iota` constant,
so `2`. Every consumer that compares a node's `Type` against a bare `1` or `2` is restating a predicate
that already exists, in a file the next reader has to open to check the numbering.

### What was measured before changing anything

`grep -rn '\.Type [!=] [12]' internal cmd`, non-test files: **25 sites in 8 files** — `layout/block.go` 9,
`layout/anonymous.go` 4, `layout/inline.go` 4, `paint/builder.go` 3, `layout/grid.go` 2,
`layout/position.go` 1, `style/style.go` 1, `cmd/debug-layout/main.go` 1. §27's deferred bullet named two
of them; the real count was an order of magnitude higher, which is why it was counted rather than
estimated. The `case 1:` / `case 2:` hits in `css/value.go`, `style/style.go`'s shorthand switches and
`platform/darwin/window.go` are other enumerations and were left alone.

### Verification

- After the swap: `grep` for the same pattern returns **0**. `gofmt -l internal cmd`: silent.
  `go build ./...`: ok. `go vet ./...`: clean.
- `go test -count=1 ./...`: no FAIL line, **29 packages ok** — the same count and result round 23 ended on.
- `go test -race ./internal/layout/ ./internal/style/ ./internal/paint/ ./test/...`: no FAIL line.
- `python3 testdata/parity.py`: **55/56 passing (98.2 %), average 95.02 %**, the one failure being
  `semantic/03-aside-figure` at 59.07 %.

### Why the swap cannot account for that failing fixture

`semantic/03-aside-figure` lays its page out with `.container { display: flex; gap: 20px }`,
`main { flex: 3 }` and `aside { flex: 1 }` — the two-column rendering the reference captures *is* the
flex distribution, and 59 % is the shape of a page whose flex container failed to distribute rather than
one whose text moved by a pixel. Nothing in the swap touches flex sizing, and each predicate is the
comparison it replaces, written out in `node.go`, with the same nil-pointer behaviour. The remaining
flex-box fidelity is the engine's known soft spot (`parseFlexDirection` was reported at 40.0 % covered in
§21's gap hunt), so the failure belongs to that, not to this round.

What has *not* been done is re-running the harness against a pre-swap build: this tree carries uncommitted
work from rounds 15, 16 and 22, so no binary built from `HEAD` is a valid control. The attribution above
is structural, and it is offered as the reason the number needs no pre-swap control rather than as a
measured one.

### Deferred, with reasons

- Attribution for `semantic/03-aside-figure`. The cheap path is one `git stash`-free worktree copy of the
  tree with these 25 sites reverted, run through `parity.py`; it belongs to the parity corpus, not to this
  clause.
- `test/css/fuzz_test.go`, `test/css/media_test.go`, `internal/style/clip_test.go` and
  `cmd/goosie/report.go`'s neighbours still compare `.Type` in tests. Left deliberately: a test that
  writes `Type: dom.NodeText` proves the constant is what the production code assumes, which a predicate
  call would hide.
- Round 22's `maxWordFragment = 1 << 16` and the `<50 MB idle` cap/geometry question are unchanged and
  still the user's call.

### An environment fact that cost this round a false report

`sed -i` over those 8 files appeared to succeed — the same command then reported 0 remaining numeric
comparisons — but the next command found the files untouched, and the `cp` backups in `/tmp` were an empty
directory. Bash-side file writes in this workspace are not durable; only the Edit/Write tools persist. The
first read of that state was reported as "the refactor is half-applied", which was wrong in both
directions: nothing had been applied, and nothing had been reverted. Recorded so the next round uses
per-site edits for a mechanical sweep instead of losing a cycle to a script that silently does nothing.

## 30. Round 25: gate 6's first sub-project is written as 18 requirements, and the tree is red on purpose

### What §4's backlog asked for

> Roadmap gate 6 (JS runtime, Goja per the chosen ADR) needs its test cases written before the runtime
> lands.

That bullet survived four rounds of backlog re-ordering because nothing else in the list is worth as much
per hour: the runtime is the first component in v2 that executes *attacker-chosen code*, and an
implementation written to no contract becomes the contract. So round 25 writes the contract as tests and
ships the tree failing.

### The shape that was fixed, and what each piece costs

`internal/js`, sub-project 1 of five (classic scripts in document order, `console`, `window`/`document`
stubs, interruptibility). Nothing else: no DOM bindings, no tasks or microtasks, no `Fetch`, no CSP.

| API | Requirement it exists to make testable |
| --- | --- |
| `New(Options) (*Runtime, error)` | one realm per document, built from a struct — there is no package-level entry point to share |
| `Options{Timeout, Console, URL, Title}` | every host input arrives at construction, including the writer `console` uses, so nothing reaches `os.Stderr` unasked |
| `Run(src, sourceURL) error` | one `<script>` element, run to completion, blocking; `sourceURL` is what makes an error attributable |
| `Global(name) (Value, bool)` | the host reads what the page computed, which is the only way a script's effect is observable from Go |
| `Title() string` | `document.title` is a tab label before it is a web API |
| `Interrupt()`, `Close()` | the host stops a script from another goroutine, and a torn-down document stops executing |
| `DefaultTimeout`, `MaxScriptBytes` | two bounds a reviewer can read out of the source and a test can measure against |
| `ErrInterrupted`, `ErrClosed`, `ErrTooLarge`, `*Error` | each failure mode is distinguishable with `errors.Is`/`errors.As`, not by string matching a panic |

`internal/archtest`'s table gained `"internal/js": nil`. That entry is the design review: sub-project 1
imports no v2 package at all, and when sub-project 2 needs `internal/dom` the gate suite will refuse the
commit until somebody widens the line deliberately.

### The one piece of production code that came first, and why

The stub: the types, the constants, the sentinels, and a `Runtime` whose every method returns
`ErrNotImplemented`. That is 117 lines of which the executable part is a dozen returns and a formatter,
written before the tests were run once, because a
test against a package that does not exist is a build error and a build error tells you nothing about
which requirements are hard. With the stub in place each failure names the requirement it is waiting on.

`(*Error).Error()` is the only logic in the file, and its test passes today — it is a formatter over three
fields, and leaving it unimplemented would have made `errors.As` in the throwing-script test unverifiable
rather than failed.

### The measured red

`go test -count=1 ./test/js/ -v` → **17 fail, 1 passes**. The one green is
`TestDefaultTimeoutIsBoundedAndNotZero`, which asserts a constant that exists and is in range; that is the
whole requirement, so the pass is real.

All 17 others stop at `js.New`. That is the cheapest red to read but the least informative: one unmet
method hides every assertion behind it. The round that implements `New` will therefore be the first round
with a real estimate of what `Run` costs, and should be planned as if the surprises are still ahead.

One pass was caught before it was shipped. `TestNewRejectsNegativeTimeout` asserted only `err != nil`, and
a stub that rejects *everything* satisfies that — the test was green while proving nothing, which is the
round 20 lesson in a new costume. It now fails `errors.Is(err, ErrNotImplemented)` explicitly, so a
runtime that rejects all options cannot pass it, and it is red in the list above where it belongs.

### Verification

- `go build ./...`, `gofmt -l internal cmd test` silent, `go vet ./internal/js ./test/js ./internal/archtest` clean.
- `go test -count=1 ./...` → **29 packages ok, exactly one FAIL: `github.com/vyquocvu/goosie/test/js`**. The
  red is confined to the new package; every pre-existing suite is untouched.
- `go test -race ./internal/archtest ./test/gate` → both ok (gate in 254 s). The architecture and cgo rules
  accept `internal/js`: no cgo, no forbidden edge, no mutable package var.
- No dependency was added. `go.mod` still requires `golang.org/x/image` and nothing else, because the
  contract is written against our own API and deliberately does not import Goja yet. Choosing the
  interpreter is not a test-writing decision, and pinning it here would have made the contract unreviewable
  on its own terms.

### What this leaves the branch

`go test ./...` on `feat/v2` is red until the runtime lands. That is the point of the round rather than an
accident of it — §7's operating rule is that an unmet expectation must be visible, and a skipped or
build-tagged test file is exactly the silent pass that rule forbids. The blast radius is measured: CI
workflows trigger on `main` and on PRs to `main`, so no pipeline goes red, and `feat/v2` is local. Anyone
who wants the branch green again has one commit to revert, and the round's value (the table above) survives
the revert because it is in this file.

### Deferred, with reasons

- The engine side: extracting `<script>` elements during the HTML parse and calling `Run` in document
  order, blocking. It is the integration half of sub-project 1 and it cannot be tested before the
  interpreter exists, because it needs a runtime that actually mutates a realm.
- A goroutine-leak assertion around `Interrupt`. Interrupting a script that never returns is the risky
  path for a wrapper that parks a VM thread, and it is only worth measuring against a real implementation.
- Whether `MaxScriptBytes` should be a per-script source bound at all, or whether the engine's
  `MaxDocumentBytes` already covers inline scripts and this is a second number for the same quantity. It
  reads as a distinct cost (compile vs. bytes-on-hand) so it is encoded that way; a reviewer may disagree.
- Sub-projects 2-5 (DOM bindings and mutation/invalidation, tasks and microtasks, `Fetch`, and the
  cross-origin/CSP/storage sweep). `TestUnsupportedWebAPIsFailLoudly` pins the interim behaviour for three
  of them: calling `setTimeout`, `document.querySelector` or `fetch` must error and name the missing API.
- Inline event handlers and `javascript:` URLs remain out of gate 6 sub-project 1 by the ADR's own scope
  note, and no test here implies they are supported.

# Goosie v2 — the frame path

v2 is a rewrite of Goosie's frame path: a fixed 256px tile grid, a CPU tile rasterizer
with a worker pool, a damage-blit composer, a vsync-paced UI loop, and two window
backends. It shares no code with the `internal/` tree and imports nothing from it.

M1 draws a synthetic document. There is no DOM, no CSS cascade, no style or layout pass,
and `-url` is refused rather than ignored. That absence is what the frame path is
measured on: a scroll frame in M1 runs zero style and zero layout passes, by counter, and
a regression that reintroduces per-frame relayout is caught by that counter rather than
by a profile.

The design is in
[`docs/superpowers/specs/2026-09-11-goosie-v2-m1-m3-frame-path-design.md`](../docs/superpowers/specs/2026-09-11-goosie-v2-m1-m3-frame-path-design.md);
the M1 plan is
[`docs/superpowers/plans/2026-09-11-goosie-v2-m1-frame-path.md`](../docs/superpowers/plans/2026-09-11-goosie-v2-m1-frame-path.md);
the numbers M2 and M3 have to beat are in
[`docs/perf/v2-m1-baseline.txt`](../docs/perf/v2-m1-baseline.txt).

## Layout

    v2/cmd/goosie            the binary: flags, pipeline assembly, the run report
    v2/internal/frame        geometry, bitmaps, the tile grid, plans, frame recording
    v2/internal/paint        display lists and the synthetic scene generator
    v2/internal/surface      the UI-thread loop and the composer
    v2/internal/raster       glyph atlas, tile rasterizer, worker pool, per-vsync scheduler
    v2/internal/platform     backend selection; headless/ and darwin/ are the backends
    v2/internal/archtest     the import and cgo boundaries, as a test
    v2/test                  every test, one package per unit under test, plus test/gate

Imports point down this list and never up: `frame` ← `paint` ← `surface` ← `raster` ←
{`platform/*`, `cmd/*`}. `surface` depends on no platform code; a backend satisfies
`surface.Window`. cgo appears in exactly one directory, `v2/internal/platform/darwin`.
All of that is enforced, not documented:

```bash
go test ./v2/internal/archtest/
```

## Run it

Requires Go 1.25 or newer. On macOS, CGO_ENABLED=1 (the default) for a real window.

```bash
# a window on this machine, scrollable with a trackpad. Ctrl-C ends it and prints the report.
go run ./v2/cmd/goosie

# the same frame path with no window: works over ssh, in CI, on Linux
go run ./v2/cmd/goosie -backend headless

# a different document, viewport, or ratio
go run ./v2/cmd/goosie -scene plain -width 1024 -height 640 -dpr 1
```

`-scene checkerboard` and `-scene plain` are the same document — a page of bordered
cells, one image, and 1,200 positioned glyph runs — except that `plain` gives every cell
one colour. The alternation is what makes a damaged tile differ from its neighbour, so
`checkerboard` is what a blit-correctness run should use and `plain` is what isolates the
text layout below it.

An interactive run scrolls a 20,000-device-pixel page at the default cache budget, which
is what makes watching the tile cache reach its ceiling and start evicting part of what
the binary shows. A `-gate` run instead gets the gate suite's document — tall enough that
a 600-frame sweep of 100px steps never runs off the end — and a budget that can hold all
of it, because a cache that loses pages during the sweep rasterizes during it and the
report would then be timing the worker pool rather than the frame path.

`go run ./v2/cmd/goosie -h` lists every flag. Nothing else is configurable, because
nothing else exists yet.

## Measure it

Three instruments, and they measure different things:

```bash
# 1. throughput: as many frames as the clock allows, headless, no display involved
go run ./v2/cmd/goosie -bench -frames 2000

# 2. pacing: 600 scripted scroll frames at real vsync, with a JSON artifact
go run ./v2/cmd/goosie -gate -scene checkerboard -frames 600 -out /tmp/gate.json

# 3. the two scroll budgets, as benchmarks
go test -run='^$' -bench='BenchmarkWarmScroll|BenchmarkColdScrollBurst' \
  -benchmem -benchtime=200x -count=3 ./v2/test/gate
```

`-bench` forces the headless backend unless one is named explicitly, because a rate that
depended on a panel's refresh would be reporting the panel. `-gate` keeps the backend you
ask for, and on macOS defaults to the real display — a gate run that silently fell back to
headless would time a frame path nobody is comparing. The report line says which one you
got: `backend=headless interactive=false` versus `backend=darwin interactive=true`.

The report goes to stderr — a `run` line, a `timings` line, a `counters` line, an `env`
line, and a `present` line when the backend can say what the display did with the frames —
plus the JSON artifact named by `-out` (`-` for stdout). Read
`docs/perf/v2-m1-baseline.txt` before comparing any two numbers between two runs;
`mean_ms` in particular is not the same quantity on the two backends.

## The gate, and what CI does

`scripts/v2-gate-check.sh` turns a gate artifact and a benchmark transcript into a
pass/fail table:

```bash
scripts/v2-gate-check.sh /tmp/gate.json -mean 16 -p99 33 \
  -bench /tmp/bench.txt -warm-ms 5 -cold-ms 8
```

Exit status is 0 when every budget holds, 1 when one is missed, 2 when the invocation or
the artifact is unusable — an artifact with zero frames in it fails rather than passing
every budget by having nothing to measure. `jq` is required.

The split between the two kinds of gate is deliberate:

| where | what it asserts | why |
| --- | --- | --- |
| CI, ubuntu-latest | counters only: `allocs/op`, tiles rasterized per frame, jobs == stale tiles, style/layout passes, import boundaries, no GPU symbols | runner core counts and memory bandwidth vary enough that a wall-clock gate flakes, and a flaky gate is ignored within a month |
| nightly, macos-latest | wall clock: mean ≤ 16 ms, p99 ≤ 33 ms over 600 display-paced frames, warm ≤ 5 ms/op, cold ≤ 8 ms/op | this is the machine with a display attached, which is the only place a pacing number means anything |

The local equivalent of the nightly job, for a pre-push run on a Mac:

```bash
go build ./v2/... && go vet ./v2/... && go test -count=1 ./v2/...
go test -run='^$' -bench='BenchmarkWarmScroll|BenchmarkColdScrollBurst' \
  -benchmem -benchtime=200x -count=3 ./v2/test/gate > /tmp/bench.txt 2>&1
go run ./v2/cmd/goosie -gate -backend native -scene checkerboard -frames 600 \
  -out /tmp/gate.json 2> /tmp/report.txt
cat /tmp/report.txt
# The run has to have been paced by a real display. A macOS runner with no window
# session falls back to nothing else when -backend is named - the run fails - but the
# check is cheap and it is the check the nightly makes, so make it here too.
grep -q 'backend=darwin' /tmp/report.txt || { echo "not display-paced"; exit 1; }
scripts/v2-gate-check.sh /tmp/gate.json -mean 16 -p99 33 \
  -bench /tmp/bench.txt -warm-ms 5 -cold-ms 8
```

The `v2-macos` job in `.github/workflows/nightly-bench.yml` runs these same commands and
uploads the three artifacts. Naming `-backend native` is not decoration: with the backend
left empty, a machine that cannot reach a window server quietly chooses headless, still
produces a full report, and every budget below would hold against a frame path that never
had a display attached.

## Testing

```bash
go test ./v2/...              # everything, including the gate's counter assertions
go test -race ./v2/...        # the loop, the worker pool, and the shim's atomics
go test ./v2/test/gate -run TestGate -v
```

`go test ./v2/...` must pass on a machine with no display, which is a requirement rather
than a hope: it is how v2's CI runs, and no test in the tree is allowed to open a window.

// Package gatecheck tests scripts/v2-gate-check.sh, the M1 pacing gate the nightly
// benchmark workflow relies on. The gate is CI code with no assertions of its own: it
// reads a JSON artifact and a `go test -bench` transcript, does float and median math in
// awk, and reports a verdict. A gate that quietly mis-parses its inputs is worse than no
// gate, so these tests pin the exit status contract in scripts/v2-gate-check.sh's header.
package gatecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

// healthy is a passing artifact: a scroll-only run, inside every budget the script gates.
const healthy = `{"report":{"frames":600,"mean_ms":12.5,"p99_ms":28.0,"max_ms":31.0,` +
	`"present_mean_ms":1.2,"present_p99_ms":4.0,"startup_ms":142.5,` +
	`"style_passes":0,"layout_passes":0,` +
	`"tiles_rasterized":120,"tiles_reused":480,` +
	`"tile_bytes":783286272,"tile_budget":785383424,"tile_evictions":0}}`

func script(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is not installed")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	path := filepath.Join(root, "scripts", "v2-gate-check.sh")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("gate script: %v", err)
	}
	return path, root
}

// run executes the gate and returns its combined output. artifact and bench are written
// to the test's temp dir when non-empty, so a case can feed the script whatever shape it
// is probing without touching the repo's real testdata. The script takes the artifact as
// its first argument and every budget as a following flag, so the file goes ahead of args.
func run(t *testing.T, artifact, bench string, args ...string) (string, int) {
	t.Helper()
	path, _ := script(t)
	dir := t.TempDir()
	argv := append([]string{}, args...)
	if artifact != "" {
		file := filepath.Join(dir, "gate.json")
		if err := os.WriteFile(file, []byte(artifact), 0o644); err != nil {
			t.Fatalf("write artifact: %v", err)
		}
		argv = append([]string{file}, argv...)
	}
	if bench != "" {
		file := filepath.Join(dir, "bench.txt")
		if err := os.WriteFile(file, []byte(bench), 0o644); err != nil {
			t.Fatalf("write bench: %v", err)
		}
		argv = append(argv, "-bench", file)
	}
	out, err := exec.Command("bash", append([]string{path}, argv...)...).CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run gate: %v\n%s", err, out)
	}
	return string(out), code
}

func wantExit(t *testing.T, want int, code int, out string) {
	t.Helper()
	if code != want {
		t.Fatalf("exit = %d, want %d\noutput:\n%s", code, want, out)
	}
}

// metricLine returns the report line naming a metric, or "" when there is none. The
// verdicts are one line per metric, so an assertion about what a line says cannot be
// satisfied by the same word appearing in another metric's row.
func metricLine(out, metric string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, metric) {
			return line
		}
	}
	return ""
}

func TestWithinBudgetPasses(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	if !strings.Contains(out, "v2-gate-check: PASS") {
		t.Errorf("missing PASS verdict:\n%s", out)
	}
}

func TestOverMeanBudgetFails(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "8", "-p99", "33")
	wantExit(t, 1, code, out)
	if !strings.Contains(out, "mean_ms") || !strings.Contains(out, "FAIL") {
		t.Errorf("mean overrun not reported:\n%s", out)
	}
}

func TestOverP99BudgetFails(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "20")
	wantExit(t, 1, code, out)
	if !strings.Contains(out, "p99_ms") {
		t.Errorf("p99 overrun not reported:\n%s", out)
	}
}

// The zero style and layout pass counters are the cheapest guard against the regression
// class that capped v1, so they have to be able to fail the gate on their own.
func TestScrollFramesWithRestyleFails(t *testing.T) {
	for _, field := range []string{"style_passes", "layout_passes"} {
		artifact := strings.Replace(healthy, `"`+field+`":0`, `"`+field+`":3`, 1)
		out, code := run(t, artifact, "", "-mean", "16", "-p99", "33")
		wantExit(t, 1, code, out)
		if !strings.Contains(out, field) {
			t.Errorf("%s overrun not reported:\n%s", field, out)
		}
	}
}

func TestBadInvocationExitsTwo(t *testing.T) {
	t.Run("no artifact", func(t *testing.T) {
		path, _ := script(t)
		out, err := exec.Command("bash", path).CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
			t.Fatalf("exit = %v, want 2\n%s", err, out)
		}
		if !strings.Contains(string(out), "Usage") {
			t.Errorf("usage not printed:\n%s", out)
		}
	})
	t.Run("artifact missing", func(t *testing.T) {
		out, code := run(t, "", "", "-mean", "16")
		wantExit(t, 2, code, out)
	})
	t.Run("artifact is not JSON", func(t *testing.T) {
		out, code := run(t, "not json at all", "", "-mean", "16")
		wantExit(t, 2, code, out)
	})
	t.Run("unknown argument", func(t *testing.T) {
		out, code := run(t, healthy, "", "-nonsense")
		wantExit(t, 2, code, out)
	})
}

// An empty window would pass every millisecond budget by having nothing in it, so the
// frame count is a precondition rather than a metric.
func TestZeroFramesExitsTwo(t *testing.T) {
	out, code := run(t, strings.Replace(healthy, `"frames":600`, `"frames":0`, 1), "", "-mean", "16")
	wantExit(t, 2, code, out)
}

// present_p99_ms is reported by default and gated only when a budget is supplied.
func TestPresentP99GatedOnlyWhenSupplied(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	if !strings.Contains(out, "reported, not gated") {
		t.Errorf("ungated present_p99_ms should be marked as reported:\n%s", out)
	}
	out, code = run(t, healthy, "", "-mean", "16", "-p99", "33", "-present-p99", "2")
	wantExit(t, 1, code, out)
	if !strings.Contains(out, "present_p99_ms") {
		t.Errorf("present_p99_ms overrun not reported:\n%s", out)
	}
}

// The gate is only worth having if a partial artifact cannot satisfy it by absence. A
// report written by an older or differently-shaped build carries no mean_ms, and reading
// that as 0.000 ms would hand the nightly a PASS for a build that never measured anything.
func TestMissingGatedMetricFailsClosed(t *testing.T) {
	for _, field := range []string{"mean_ms", "p99_ms", "style_passes", "layout_passes"} {
		out, code := run(t, strings.Replace(healthy, `"`+field+`":`, `"`+field+`_omitted":`, 1), "", "-mean", "16", "-p99", "33")
		wantExit(t, 2, code, out)
	}
}

func TestNonNumericMetricFailsClosed(t *testing.T) {
	out, code := run(t, strings.Replace(healthy, `"mean_ms":12.5`, `"mean_ms":"n/a"`, 1), "", "-mean", "16")
	wantExit(t, 2, code, out)
}

// benchLine renders one `go test -bench -benchmem` result in the exact shape go emits,
// which the script's own comment documents: ten whitespace-separated fields, with the
// metric units as their own column so that the value is counted back from the end.
//
//	BenchmarkName-CPUs  iterations  ns-value  ns/op  ms-value  ms/op  B-value  B/op  allocs  allocs/op
func benchLine(name string, cpus int, msPerOp float64, allocs int) string {
	ns := strconv.FormatInt(int64(msPerOp*1_000_000), 10)
	return "Benchmark" + name + "-" + strconv.Itoa(cpus) + "   \t   200   \t" + ns +
		" ns/op   \t" + strconv.FormatFloat(msPerOp, 'f', 3, 64) +
		" ms/op   \t       0 B/op   \t      " + strconv.Itoa(allocs) + " allocs/op\n"
}

// With -count>1 there are several lines per benchmark and the median is the number, so
// that one noisy repetition is a property of the runner rather than a verdict.
func TestBenchMedianIgnoresOneNoisyRepetition(t *testing.T) {
	var bench strings.Builder
	bench.WriteString(benchLine("WarmScroll", 8, 4.0, 12))
	bench.WriteString(benchLine("WarmScroll", 8, 30.0, 12)) // the noisy one
	bench.WriteString(benchLine("WarmScroll", 8, 4.4, 12))
	bench.WriteString(benchLine("ColdScrollBurst", 8, 6.0, 20))
	bench.WriteString(benchLine("ColdScrollBurst", 8, 6.4, 20))
	bench.WriteString(benchLine("ColdScrollBurst", 8, 40.0, 20))

	out, code := run(t, healthy, bench.String(), "-mean", "16", "-p99", "33", "-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 0, code, out)
	if !strings.Contains(out, "4.400") {
		t.Errorf("warm median should be 4.400 ms/op:\n%s", out)
	}
	if !strings.Contains(out, "6.400") {
		t.Errorf("cold median should be 6.400 ms/op:\n%s", out)
	}
}

// An even repetition count has no middle element, so the middle two are averaged. The
// budgets bracket that average and exclude either neighbour, which is what distinguishes
// the mean-of-middle-two from a floor or a ceiling.
func TestBenchMedianOfEvenRepetitionsAveragesMiddleTwo(t *testing.T) {
	var bench strings.Builder
	for _, ms := range []float64{2.0, 4.0, 6.0, 8.0} {
		bench.WriteString(benchLine("WarmScroll", 8, ms, 12))
		bench.WriteString(benchLine("ColdScrollBurst", 8, 1.0, 20))
	}
	out, code := run(t, healthy, bench.String(), "-mean", "16", "-p99", "33", "-warm-ms", "5.1", "-cold-ms", "8")
	wantExit(t, 0, code, out)
	if !strings.Contains(out, "5.000") {
		t.Errorf("warm median should be 5.000 ms/op:\n%s", out)
	}

	// 5.000 is over a 4.9 budget, so the same transcript has to fail.
	out, code = run(t, healthy, bench.String(), "-mean", "16", "-p99", "33", "-warm-ms", "4.9", "-cold-ms", "8")
	wantExit(t, 1, code, out)
}

func TestBenchOverBudgetFails(t *testing.T) {
	bench := benchLine("WarmScroll", 8, 9.0, 12) + benchLine("ColdScrollBurst", 8, 1.0, 20)
	out, code := run(t, healthy, bench, "-mean", "16", "-p99", "33", "-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 1, code, out)
	if !strings.Contains(out, "BenchmarkWarmScroll") {
		t.Errorf("warm overrun not named:\n%s", out)
	}
}

// If a benchmark silently stops running, the gate must say so rather than wave the
// artifact through for want of a number to check.
func TestBenchMissingBenchmarkFailsClosed(t *testing.T) {
	out, code := run(t, healthy, benchLine("SomethingElse", 8, 1.0, 1), "-mean", "16", "-p99", "33",
		"-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 2, code, out)
	if !strings.Contains(out, "BenchmarkWarmScroll") {
		t.Errorf("absent benchmark should be named:\n%s", out)
	}
}

// The median is read by counting columns back from the end of the line. Dropping
// -benchmem from the workflow removes the ms/op column, which shifts that count, so a
// transcript without it has to be a broken parse rather than a pass.
func TestBenchMissingMsColumnFailsClosed(t *testing.T) {
	noMs := "BenchmarkWarmScroll-8   \t   200   \t   4000000 ns/op\n"
	out, code := run(t, healthy, noMs+benchLine("ColdScrollBurst", 8, 1.0, 20), "-mean", "16", "-p99", "33",
		"-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 2, code, out)
}

// go test suffixes the benchmark name with the CPU count, and the gate has to match it.
func TestBenchMatchesCPUCountSuffix(t *testing.T) {
	bench := benchLine("WarmScroll", 10, 4.0, 12) + benchLine("ColdScrollBurst", 10, 1.0, 20)
	out, code := run(t, healthy, bench, "-mean", "16", "-p99", "33", "-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 0, code, out)
}

func TestHelpExitsZero(t *testing.T) {
	path, _ := script(t)
	out, err := exec.Command("bash", path, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help should exit 0: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Usage:") || !strings.Contains(string(out), "-cold-ms") {
		t.Errorf("help should document the flags:\n%s", out)
	}
	if !strings.Contains(string(out), "-startup-ms") {
		t.Errorf("help should document -startup-ms:\n%s", out)
	}
}

// Round two: the same fail-open class, found by attacking the guards themselves rather
// than the fields they read.

// The frame-count guard exists because an empty window passes every millisecond budget by
// having nothing in it. Comparing the field against the string "0" only refuses one
// spelling of it: JSON writes a zero count as 0.0 as readily as 0, and a negative frame
// count is not a window at all.
func TestNonPositiveFrameCountExitsTwo(t *testing.T) {
	for _, frames := range []string{"0.0", "-1", "0.5", "0e0"} {
		out, code := run(t, strings.Replace(healthy, `"frames":600`, `"frames":`+frames, 1), "", "-mean", "16")
		wantExit(t, 2, code, out)
	}
}

// A budget flag whose value is absent or empty is not "this budget is not being gated",
// because the branch that reads it as unsupplied cannot tell the two apart. -present-p99
// with an empty value printed the metric as "reported, not gated" and passed.
func TestNonNumericBudgetExitsTwo(t *testing.T) {
	for _, args := range [][]string{
		{"-present-p99", ""},
		{"-present-p99", "abc"},
		{"-mean", ""},
		{"-p99", "16ms"},
		{"-warm-ms", ""},
		{"-cold-ms", "abc"},
		{"-startup-ms", ""},
		{"-startup-ms", "0.5s"},
		{"-mean"}, // dangling: the flag is the last word
	} {
		bench := ""
		if args[0] == "-warm-ms" || args[0] == "-cold-ms" {
			bench = benchLine("WarmScroll", 8, 1.0, 1) + benchLine("ColdScrollBurst", 8, 1.0, 1)
		}
		out, code := run(t, healthy, bench, args...)
		wantExit(t, 2, code, out)
	}
}

// A negative latency or pass counter is a broken artifact, not a measurement comfortably
// under budget. The first cut of the metric guard allowed a leading minus, so
// style_passes:-5 read as "no restyles happened" and passed the v1 regression check.
func TestNegativeMetricExitsTwo(t *testing.T) {
	for _, pair := range [][2]string{
		{"mean_ms", "12.5"},
		{"p99_ms", "28.0"},
		{"style_passes", "0"},
		{"layout_passes", "0"},
		{"startup_ms", "142.5"},
	} {
		out, code := run(t, strings.Replace(healthy, `"`+pair[0]+`":`+pair[1], `"`+pair[0]+`":-1`, 1), "", "-mean", "16", "-p99", "33")
		wantExit(t, 2, code, out)
	}
}

// Valid JSON is not the expected JSON. `jq -e .` accepts [], "null"-adjacent scalars, and
// {"report":"gone"} alike, after which every read below fails inside jq and the run ends
// with jq's own exit status rather than this script's.
func TestWrongJSONShapeExitsTwo(t *testing.T) {
	for _, artifact := range []string{`[]`, `{"report":"gone"}`, `{"reports":{}}`, `"a string"`} {
		out, code := run(t, artifact, "", "-mean", "16")
		wantExit(t, 2, code, out)
	}
}

// The tile note adds the two counters, so a non-numeric one aborts the run mid-report on
// a bash arithmetic error - a missed budget (1) for what is really a broken artifact (2).
func TestNonNumericTileCountersExitsTwo(t *testing.T) {
	for _, field := range []string{"tiles_rasterized", "tiles_reused"} {
		out, code := run(t, strings.Replace(healthy, `"`+field+`":`, `"`+field+`":"a b", "x":`, 1), "", "-mean", "16", "-p99", "33")
		wantExit(t, 2, code, out)
	}
}

// Round nine: the brief's other headline target, a sub-second cold start, had no number
// anywhere behind it. startup_ms is the process's first presented frame, and the nightly
// is the only runner with a display to pace it, so this is where the budget lives.
func TestStartupGatedOnlyWhenSupplied(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	if !strings.Contains(out, "startup_ms") || !strings.Contains(out, "142.500") {
		t.Errorf("ungated startup_ms should be printed with its value:\n%s", out)
	}
	if strings.Contains(out, "startup_ms") && strings.Contains(out, "FAIL") {
		t.Errorf("an ungated metric cannot fail the run:\n%s", out)
	}

	out, code = run(t, healthy, "", "-mean", "16", "-p99", "33", "-startup-ms", "1000")
	wantExit(t, 0, code, out)

	out, code = run(t, healthy, "", "-mean", "16", "-p99", "33", "-startup-ms", "100")
	wantExit(t, 1, code, out)
	if !strings.Contains(out, "startup_ms") {
		t.Errorf("startup overrun not reported:\n%s", out)
	}
}

// An artifact whose startup was never measured carries null, and the gate has to say so
// rather than print - or worse, compare - a zero. bash's printf reads an empty value as
// 0.000, which is under a sub-second budget and would report the fastest start possible
// for the one run that showed nothing at all.
func TestUnmeasuredStartupFailsClosed(t *testing.T) {
	null := strings.Replace(healthy, `"startup_ms":142.5`, `"startup_ms":null`, 1)
	out, code := run(t, null, "", "-mean", "16", "-p99", "33", "-startup-ms", "1000")
	wantExit(t, 2, code, out)

	// Ungated it is still not a number: the line has to say it measured nothing.
	out, code = run(t, null, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	line := metricLine(out, "startup_ms")
	if line == "" {
		t.Fatalf("no startup_ms line in the report:\n%s", out)
	}
	if !strings.Contains(line, "not measured") {
		t.Errorf("an unmeasured startup should be labelled as such, got %q", line)
	}
	// The whole point of the label: bash's printf reads an empty value as 0.000, which
	// is a startup fast enough to win any budget.
	if strings.Contains(line, "0.000") {
		t.Errorf("an unmeasured startup printed as a zero millisecond figure: %q", line)
	}

	// A build predating the metric has no key at all, which jq reads the same way.
	out, code = run(t, strings.Replace(healthy, `"startup_ms":142.5,`, ``, 1), "", "-startup-ms", "1000")
	wantExit(t, 2, code, out)
}

func TestNonNumericStartupExitsTwo(t *testing.T) {
	out, code := run(t, strings.Replace(healthy, `"startup_ms":142.5`, `"startup_ms":"fast"`, 1), "", "-mean", "16")
	wantExit(t, 2, code, out)
}

// Round 13: the tile cache's own ledger. A nightly run at the gate's geometry holds about
// 750 MiB, which reads as a leak to anyone without the configuration beside it - and the
// paced scene's budget is the whole document by design, so the number alone cannot tell a
// full cache from an escaping one. These cases pin the three-way split the artifact needs:
// bytes held, bytes authorised, and buffers evicted.
func TestTileCacheIsReportedNotGated(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	line := metricLine(out, "tile_mib")
	if line == "" {
		t.Fatalf("no tile_mib line in the report:\n%s", out)
	}
	if !strings.Contains(line, "747") || !strings.Contains(line, "749") {
		t.Errorf("tile_mib should print the bytes held against the bytes authorised, got %q", line)
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("an ungated metric cannot fail the run:\n%s", out)
	}
}

func TestTileBudgetIsGatedOnlyWhenSupplied(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "33", "-tile-mib", "800")
	wantExit(t, 0, code, out)
	line := metricLine(out, "tile_mib")
	if !strings.Contains(line, "budget 800") {
		t.Errorf("a supplied tile budget should appear as the budget, got %q", line)
	}

	out, code = run(t, healthy, "", "-mean", "16", "-p99", "33", "-tile-mib", "700")
	wantExit(t, 1, code, out)
	if !strings.Contains(out, "tile_mib") || !strings.Contains(out, "FAIL") {
		t.Errorf("an over-budget tile cache must be named as the failure:\n%s", out)
	}
}

// A build that stops emitting the fields has to be refused by a supplied budget rather than
// read as an empty cache: bash's printf turns the absence into 0.000 MiB, which is inside
// every budget there is.
func TestMissingTileFieldsFailClosed(t *testing.T) {
	stripped := strings.Replace(healthy, `"tile_bytes":783286272,`, ``, 1)
	out, code := run(t, stripped, "", "-mean", "16", "-p99", "33", "-tile-mib", "800")
	wantExit(t, 2, code, out)

	// Ungated, the absence is still not a zero: the line has to say the run reported none.
	out, code = run(t, stripped, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	line := metricLine(out, "tile_mib")
	if line == "" {
		t.Fatalf("no tile_mib line in the report:\n%s", out)
	}
	if !strings.Contains(line, "not reported") || strings.Contains(line, "0.000") {
		t.Errorf("an absent tile ledger should be labelled, not printed as a zero: %q", line)
	}
}

func TestNonNumericTileBytesExitsTwo(t *testing.T) {
	out, code := run(t, strings.Replace(healthy, `"tile_bytes":783286272`, `"tile_bytes":"lots"`, 1), "", "-tile-mib", "800")
	wantExit(t, 2, code, out)
}

// Evictions are the third leg: a sweep that never evicts over a budget smaller than the
// document is a cache that filled to its configuration, and a run that evicts nothing while
// holding more than it was authorised is the opposite. Reported either way.
func TestEvictionsAreReported(t *testing.T) {
	out, code := run(t, healthy, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	line := metricLine(out, "tile_evictions")
	if line == "" {
		t.Fatalf("no tile_evictions line in the report:\n%s", out)
	}
	if !strings.Contains(line, "0") {
		t.Errorf("evictions should print the counter, got %q", line)
	}
}

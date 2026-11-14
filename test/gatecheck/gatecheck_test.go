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

func TestZeroFramesExitsTwo(t *testing.T) {
	out, code := run(t, strings.Replace(healthy, `"frames":600`, `"frames":0`, 1), "", "-mean", "16")
	wantExit(t, 2, code, out)
}

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

func benchLine(name string, cpus int, msPerOp float64, allocs int) string {
	ns := strconv.FormatInt(int64(msPerOp*1_000_000), 10)
	return "Benchmark" + name + "-" + strconv.Itoa(cpus) + "   \t   200   \t" + ns +
		" ns/op   \t" + strconv.FormatFloat(msPerOp, 'f', 3, 64) +
		" ms/op   \t       0 B/op   \t      " + strconv.Itoa(allocs) + " allocs/op\n"
}

func TestBenchMedianIgnoresOneNoisyRepetition(t *testing.T) {
	var bench strings.Builder
	bench.WriteString(benchLine("WarmScroll", 8, 4.0, 12))
	bench.WriteString(benchLine("WarmScroll", 8, 30.0, 12))
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

func TestBenchMissingBenchmarkFailsClosed(t *testing.T) {
	out, code := run(t, healthy, benchLine("SomethingElse", 8, 1.0, 1), "-mean", "16", "-p99", "33",
		"-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 2, code, out)
	if !strings.Contains(out, "BenchmarkWarmScroll") {
		t.Errorf("absent benchmark should be named:\n%s", out)
	}
}

func TestBenchMissingMsColumnFailsClosed(t *testing.T) {
	noMs := "BenchmarkWarmScroll-8   \t   200   \t   4000000 ns/op\n"
	out, code := run(t, healthy, noMs+benchLine("ColdScrollBurst", 8, 1.0, 20), "-mean", "16", "-p99", "33",
		"-warm-ms", "5", "-cold-ms", "8")
	wantExit(t, 2, code, out)
}

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

func TestNonPositiveFrameCountExitsTwo(t *testing.T) {
	for _, frames := range []string{"0.0", "-1", "0.5", "0e0"} {
		out, code := run(t, strings.Replace(healthy, `"frames":600`, `"frames":`+frames, 1), "", "-mean", "16")
		wantExit(t, 2, code, out)
	}
}

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
		{"-mean"},
	} {
		bench := ""
		if args[0] == "-warm-ms" || args[0] == "-cold-ms" {
			bench = benchLine("WarmScroll", 8, 1.0, 1) + benchLine("ColdScrollBurst", 8, 1.0, 1)
		}
		out, code := run(t, healthy, bench, args...)
		wantExit(t, 2, code, out)
	}
}

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

func TestWrongJSONShapeExitsTwo(t *testing.T) {
	for _, artifact := range []string{`[]`, `{"report":"gone"}`, `{"reports":{}}`, `"a string"`} {
		out, code := run(t, artifact, "", "-mean", "16")
		wantExit(t, 2, code, out)
	}
}

func TestNonNumericTileCountersExitsTwo(t *testing.T) {
	for _, field := range []string{"tiles_rasterized", "tiles_reused"} {
		out, code := run(t, strings.Replace(healthy, `"`+field+`":`, `"`+field+`":"a b", "x":`, 1), "", "-mean", "16", "-p99", "33")
		wantExit(t, 2, code, out)
	}
}

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

func TestUnmeasuredStartupFailsClosed(t *testing.T) {
	null := strings.Replace(healthy, `"startup_ms":142.5`, `"startup_ms":null`, 1)
	out, code := run(t, null, "", "-mean", "16", "-p99", "33", "-startup-ms", "1000")
	wantExit(t, 2, code, out)

	out, code = run(t, null, "", "-mean", "16", "-p99", "33")
	wantExit(t, 0, code, out)
	line := metricLine(out, "startup_ms")
	if line == "" {
		t.Fatalf("no startup_ms line in the report:\n%s", out)
	}
	if !strings.Contains(line, "not measured") {
		t.Errorf("an unmeasured startup should be labelled as such, got %q", line)
	}
	if strings.Contains(line, "0.000") {
		t.Errorf("an unmeasured startup printed as a zero millisecond figure: %q", line)
	}

	out, code = run(t, strings.Replace(healthy, `"startup_ms":142.5,`, ``, 1), "", "-startup-ms", "1000")
	wantExit(t, 2, code, out)
}

func TestNonNumericStartupExitsTwo(t *testing.T) {
	out, code := run(t, strings.Replace(healthy, `"startup_ms":142.5`, `"startup_ms":"fast"`, 1), "", "-mean", "16")
	wantExit(t, 2, code, out)
}

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

func TestMissingTileFieldsFailClosed(t *testing.T) {
	stripped := strings.Replace(healthy, `"tile_bytes":783286272,`, ``, 1)
	out, code := run(t, stripped, "", "-mean", "16", "-p99", "33", "-tile-mib", "800")
	wantExit(t, 2, code, out)

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

package gatecheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

func parityRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
}

func parityPython(t *testing.T) string {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	if out, err := exec.Command(py, "-c", "import numpy, PIL").CombinedOutput(); err != nil {
		t.Skipf("numpy/PIL are not installed for python3: %v: %s", err, out)
	}
	return py
}

func parityScript(root string) string {
	return filepath.Join(root, "testdata", "parity.py")
}

func paritySelftestScript(root string) string {
	return filepath.Join(root, "testdata", "parity_selftest.py")
}

// qc:BR-177
func TestParitySelftestIsGreen(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	cmd := exec.Command(py, paritySelftestScript(root))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("parity_selftest.py failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "selftest green") {
		t.Fatalf("parity_selftest.py did not print its green marker:\n%s", out)
	}
}

// qc:BR-264
func TestParityWorkdirIsAccepted(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	work := filepath.Join(t.TempDir(), "corpus")
	cmd := exec.Command(py, parityScript(root), "--workdir", work)
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("parity.py --workdir reported success on an empty corpus:\n%s", out)
	}
	text := string(out)
	if !strings.Contains(text, "per-run render corpus rooted at "+work) {
		t.Fatalf("parity.py --workdir did not announce the per-run root %q;\n"+
			"the flag is missing or is not published into the render roots\n%s",
			work, text)
	}
}

// qc:BR-265
func TestParityBaselineRecordsItsHost(t *testing.T) {
	root := parityRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "render-known-gaps.json"))
	if err != nil {
		t.Fatalf("read render-known-gaps.json: %v", err)
	}
	var baseline struct {
		MeasuredOn string `json:"measured_on"`
	}
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("decode render-known-gaps.json: %v", err)
	}
	if strings.TrimSpace(baseline.MeasuredOn) == "" {
		t.Fatal("render-known-gaps.json records no measured_on; its blank-band " +
			"counts are then attributed to no host, so a 'known-gap baseline is " +
			"stale' FAIL on a second platform cannot be told from an engine " +
			"regression (GOOSIE-CIPARITY-001)")
	}

	py := parityPython(t)
	cmd := exec.Command(py, "-c", `import importlib.util, sys
spec = importlib.util.spec_from_file_location("parity_under_test", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
sys.stdout.write(str(mod.known_gap_provenance()))`, parityScript(root))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("parity.py known_gap_provenance(): %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != baseline.MeasuredOn {
		t.Errorf("parity.py reports measured_on %q, render-known-gaps.json says %q; "+
			"the accessor and the file no longer read the same key", got, baseline.MeasuredOn)
	}
}

// qc:BR-187
func TestParityDriftMetricIsGreen(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	cmd := exec.Command(py, paritySelftestScript(root), "--drift")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("parity_selftest.py --drift failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "drift selftest green") {
		t.Fatalf("parity_selftest.py --drift did not print its green marker:\n%s", out)
	}
}

// qc:BR-176
func TestParityFixtureFloorMatchesCorpus(t *testing.T) {
	root := parityRepoRoot(t)
	src, err := os.ReadFile(parityScript(root))
	if err != nil {
		t.Fatalf("read parity.py: %v", err)
	}
	re := regexp.MustCompile(`(?m)^MIN_SCORED_FIXTURES[ \t]*=[ \t]*([0-9]+)[ \t]*$`)
	m := re.FindSubmatch(src)
	if m == nil {
		t.Fatal("MIN_SCORED_FIXTURES assignment not found in testdata/parity.py")
	}
	var floor int
	if floor, err = strconv.Atoi(string(m[1])); err != nil {
		t.Fatalf("parse MIN_SCORED_FIXTURES %q: %v", m[1], err)
	}

	fixtures := filepath.Join(root, "testdata", "render")
	count := 0
	walkErr := filepath.WalkDir(fixtures, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".html") {
			count++
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk fixtures: %v", walkErr)
	}
	if count == 0 {
		t.Fatal("no HTML fixtures found under testdata/render")
	}
	if count < floor {
		t.Errorf("testdata/render has %d HTML fixtures, below MIN_SCORED_FIXTURES %d; "+
			"the render corpus shrank, so the parity denominator is no longer honest "+
			"(raising the floor to the new count is how an added fixture locks in)",
			count, floor)
	}
}

// qc:BR-177
func TestParityRejectsNoStrictFlag(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	cmd := exec.Command(py, parityScript(root), "--no-strict")
	cmd.Dir = root
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run parity.py --no-strict: %v", err)
	}
	if code == 0 {
		t.Fatalf("--no-strict was accepted (rc 0); the escape hatch must stay dead\n%s", stdout.String())
	}
	joined := stderr.String() + stdout.String()
	if !strings.Contains(joined, "unrecognized") {
		t.Fatalf("--no-strict did not fail as an unrecognized flag (rc %d); expected an argparse \"unrecognized\" error\n%s", code, joined)
	}
}

package gatecheck

import (
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

// Parity harness guard for GOOSIE-PARITYGATE-001.  The render-parity oracle
// (testdata/parity.py) is the evidence every other PARITY task leans on, so it
// must be impossible to fool.  These tests drive the pure-Python selftest
// (testdata/parity_selftest.py, which builds synthetic images in a throwaway
// temp corpus and needs no browser and no network), pin the corpus floor
// against the real fixture count, and prove the --no-strict escape hatch is
// dead rather than merely hidden.  They skip only when python3 / numpy / PIL are
// absent (CI installs them: .github/workflows/ci.yml).

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

// parityPython returns a python3 that can import numpy and PIL, or skips the
// test.  The selftest and the parity CLI both need those two modules, so a
// machine without them cannot exercise the oracle.
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

// TestParitySelftestIsGreen runs the oracle's own fail-closed self-tests and
// requires a clean exit.  This is the primary gate for this task: it proves the
// harness reports a missing render, rejects an inflated known-gaps baseline, and
// trips the corpus floor — all without a browser.
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

// TestParityFixtureFloorMatchesCorpus reads MIN_SCORED_FIXTURES straight out of
// testdata/parity.py and asserts the real count of HTML fixtures is at least
// that floor. Neither number may be hard-coded twice, so shrinking the corpus
// (deleting fixtures so the denominator falls) is caught here the same way
// parity.py's main() catches it at runtime. The floor only polices downward
// movement: adding a fixture must not fail this test, and locking the gain is
// done by raising MIN_SCORED_FIXTURES to the new count in the same change.
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

// TestParityRejectsNoStrictFlag proves the --no-strict escape hatch is gone: the
// flag is no longer recognised at all, so parity.py exits non-zero with an
// "unrecognized" argparse error instead of silently skipping missing renders.
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

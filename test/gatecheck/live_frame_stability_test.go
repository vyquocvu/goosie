package gatecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// qc:BR-286
func TestLiveFrameStabilitySelftestIsGreen(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	cmd := exec.Command(py, filepath.Join(root, "scripts", "live-frame-stability.py"), "--selftest")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("live-frame-stability.py --selftest failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "selftest green") {
		t.Fatalf("live-frame-stability.py did not print its green marker:\n%s", out)
	}
}

// qc:BR-286
func TestLiveFrameStabilityRefusesAnUnrunnableRequest(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	script := filepath.Join(root, "scripts", "live-frame-stability.py")
	cmd := exec.Command(py, script)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("a run with no --url exited 0; it must refuse:\n%s", out)
	}
	if exit := cmd.ProcessState.ExitCode(); exit != 2 {
		t.Errorf("no --url exit = %d, want 2 (NOT MEASURABLE)", exit)
	}
	if !strings.Contains(strings.ToLower(string(out)), "url") {
		t.Errorf("refusal did not name the missing argument:\n%s", out)
	}
}

// qc:BR-286
func TestLiveFrameStabilityScoresWithTheParityHarnessNumbers(t *testing.T) {
	root := parityRepoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "scripts", "live-frame-stability.py"))
	if err != nil {
		t.Fatalf("read the script: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, `"testdata" / "parity.py"`) {
		t.Error("the script does not import testdata/parity.py, so region scoring has two definitions")
	}
	if !strings.Contains(text, "parity.THRESHOLD") || !strings.Contains(text, "parity.content_bands") {
		t.Error("the script does not score with parity.THRESHOLD and parity.content_bands")
	}
	for _, own := range []string{"THRESHOLD =", "MIN_INK =", "BAND_GAP ="} {
		if strings.Contains(text, own) {
			t.Errorf("the script defines its own %s instead of borrowing the harness's", own)
		}
	}
}

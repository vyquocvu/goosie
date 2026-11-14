package gatecheck

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// qc:BR-258
func TestLiveParitySelftestIsGreen(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	cmd := exec.Command(py, filepath.Join(root, "testdata", "live-parity.py"), "--selftest")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("live-parity.py --selftest failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "selftest green") {
		t.Fatalf("live-parity.py did not print its green marker:\n%s", out)
	}
}

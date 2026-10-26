package gatecheck

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The live sweep is a report, not a gate, but the report decides what the team
// believes about 100 real sites.  Its own selftest is where the fail-closed
// properties live: a 4xx row must be named as not-a-measurement instead of
// reading as an engine failure, and the disclosure must not swallow clean rows.
// GOOSIE-LIVE-005 is the defect that made the section necessary.
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

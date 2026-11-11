package gatecheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunWPTScriptFailsClosedOnSkippedSuite proves make wpt can never again
// report success while TestWPTSuite skips: an empty checkout discovers zero
// tests, the suite skips, and the script must exit non-zero.
func TestRunWPTScriptFailsClosedOnSkippedSuite(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	root := parityRepoRoot(t)
	var cfg struct {
		Commit string `json:"commit"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "wpt-config.json"))
	if err != nil {
		t.Fatalf("read wpt config: %v", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse wpt config: %v", err)
	}

	dir := t.TempDir()
	wptDir := filepath.Join(dir, "wpt")
	if err := os.MkdirAll(wptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The reuse marker keeps Checkout.Ensure from downloading: the empty
	// checkout then discovers zero tests and the suite skips. The marker
	// alone is not reusable (checkoutReusable wants content behind it), so
	// add one inert entry.
	if err := os.WriteFile(filepath.Join(wptDir, ".goosie-wpt-commit"), []byte(cfg.Commit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wptDir, "inert"), 0o755); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "run-wpt.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"WPT_DIR="+wptDir,
		"WPT_OUTPUT_DIR="+outDir,
		"WPT_GOOSIE_BINARY=/usr/bin/true",
		"WPT_DIRS=does-not-exist",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("run-wpt.sh exited 0 on a suite that never ran; output:\n%s", out)
	}
	// Two fail-closed paths are acceptable: the suite errors on the empty
	// checkout (go test exit propagates), or it skips and the script's own
	// results check fires. Neither may report success.
	if strings.Contains(string(out), "completed successfully") {
		t.Fatalf("run-wpt.sh reported success on a suite that never ran; output:\n%s", out)
	}
}

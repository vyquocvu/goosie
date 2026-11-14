package gate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func runArtifact(t *testing.T, args ...string) map[string]any {
	t.Helper()
	root := moduleRoot(t)
	bin := appBinary(t, root)
	out := filepath.Join(t.TempDir(), "gate-artifact.json")
	cmd := exec.Command(bin, append(args, "-out", out)...)
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", filepath.Base(bin), args, err, combined)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the run wrote no artifact: %v", err)
	}
	var doc struct {
		Report map[string]any `json:"report"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if doc.Report == nil {
		t.Fatal("the artifact carries no report object")
	}
	wanted := ""
	for i, a := range args {
		if a == "-frames" && i+1 < len(args) {
			wanted = args[i+1]
		}
	}
	if wanted != "" {
		n, ok := doc.Report["frames"].(float64)
		want, err := strconv.Atoi(wanted)
		if !ok || err != nil || int64(want) != int64(n) {
			t.Fatalf("the artifact reports frames=%v, want the %s that were asked for", doc.Report["frames"], wanted)
		}
	}
	return doc.Report
}

func runBench(t *testing.T) map[string]any {
	t.Helper()
	return runArtifact(t, "-bench", "-backend", "headless", "-scene", "checkerboard", "-frames", "16")
}

func TestGateArtifactCarriesTheTileLedger(t *testing.T) {
	report := runBench(t)
	ledger := map[string]int64{}
	for _, key := range []string{"tile_bytes", "tile_budget", "tile_evictions"} {
		v, ok := report[key]
		if !ok {
			t.Fatalf("the artifact has no %q key; the gate script reads it", key)
		}
		n, isNum := v.(float64)
		if !isNum {
			t.Fatalf("%q holds %v (%T); want a number of bytes, not a null the gate would have to refuse", key, v, v)
		}
		ledger[key] = int64(n)
	}

	if ledger["tile_bytes"] == 0 {
		t.Errorf("tile_bytes = 0 after a run that rasterized %v tiles: the ledger is wired to nothing", report["tiles_rasterized"])
	}
	if ledger["tile_budget"] == 0 {
		t.Errorf("tile_budget = 0: the grid reports the budget its cache was configured with, so this is a missing figure rather than an empty cache")
	}
	if ledger["tile_bytes"] > ledger["tile_budget"] {
		t.Errorf("tile_bytes %d exceeds tile_budget %d: held tile memory passed the budget that authorises it",
			ledger["tile_bytes"], ledger["tile_budget"])
	}
	t.Logf("ledger: held=%d bytes of %d budgeted, %d evictions",
		ledger["tile_bytes"], ledger["tile_budget"], ledger["tile_evictions"])
}

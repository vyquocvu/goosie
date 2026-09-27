package gate

// The gate artifact, read back from the binary that writes it. The unit tests in
// test/frame pin what the recorder encodes and test/gatecheck pins what the script
// accepts; neither can see the wiring between them, and the wiring is where a ledger
// goes missing: the report function is free to hand the recorder the counters it likes,
// and a run that never hands in the grid's accounting produces an artifact whose tile
// fields are null, which is exactly the shape a build that stopped measuring looks like.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// runArtifact builds the shipped binary, runs it headless with args, and returns the
// report object from the artifact it wrote. Headless because CI has no display session and
// the criterion here is the artifact's contents, not the pacing; the caller's -frames is
// short because a run this small is otherwise a second of wall clock.
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
	// The run is the one asked for: an artifact from a different invocation would
	// satisfy every field check a caller makes while saying nothing about this build.
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

// runBench is the gate's own run: the 16-frame checkerboard burst the nightly measures.
func runBench(t *testing.T) map[string]any {
	t.Helper()
	return runArtifact(t, "-bench", "-backend", "headless", "-scene", "checkerboard", "-frames", "16")
}

// TestGateArtifactCarriesTheTileLedger is the round-13 criterion: a run's held tile
// bytes, the budget that authorised them, and the evictions that kept the first inside
// the second, all present as numbers rather than null. The scripts' gate can only refuse
// an absent ledger once the binary is proven to write one.
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

	// A zero held is a cache that never filled, which a 16-frame checkerboard burst at
	// the gate's geometry does not do: it rasterizes hundreds of tiles. Asserting the
	// non-zero half is what makes the null check above non-vacuous, because both are the
	// same absent figure wearing different types.
	if ledger["tile_bytes"] == 0 {
		t.Errorf("tile_bytes = 0 after a run that rasterized %v tiles: the ledger is wired to nothing", report["tiles_rasterized"])
	}
	if ledger["tile_budget"] == 0 {
		t.Errorf("tile_budget = 0: the grid reports the budget its cache was configured with, so this is a missing figure rather than an empty cache")
	}
	// The invariant the whole ledger exists to state, checked where it is produced rather
	// than only where it is read: the grid never lets held bytes pass its budget, so an
	// artifact that says otherwise has mis-wired fields, not a browser that overspent.
	if ledger["tile_bytes"] > ledger["tile_budget"] {
		t.Errorf("tile_bytes %d exceeds tile_budget %d: held tile memory passed the budget that authorises it",
			ledger["tile_bytes"], ledger["tile_budget"])
	}
	t.Logf("ledger: held=%d bytes of %d budgeted, %d evictions",
		ledger["tile_bytes"], ledger["tile_budget"], ledger["tile_evictions"])
}

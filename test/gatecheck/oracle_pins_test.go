package gatecheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type oraclePins struct {
	Render      struct{ Playwright, Chromium string } `json:"render"`
	Interaction struct{ Playwright, Chromium string } `json:"interaction"`
}

func TestOraclePinsAreConsistent(t *testing.T) {
	root := parityRepoRoot(t)
	read := func(rel string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return raw
	}
	var pins oraclePins
	if err := json.Unmarshal(read("testdata/oracle-pins.json"), &pins); err != nil {
		t.Fatalf("parse oracle-pins.json: %v", err)
	}
	if pins.Render.Playwright == "" || pins.Interaction.Playwright == "" {
		t.Fatal("oracle-pins.json must pin both oracles")
	}

	req := string(read("testdata/requirements.txt"))
	if !strings.Contains(req, "playwright=="+pins.Render.Playwright+"\n") {
		t.Errorf("requirements.txt does not pin playwright==%s", pins.Render.Playwright)
	}

	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(read("package.json"), &pkg); err != nil {
		t.Fatalf("parse package.json: %v", err)
	}
	if got := pkg.Dependencies["playwright"]; got != pins.Interaction.Playwright {
		t.Errorf("package.json playwright = %q, want exact %q", got, pins.Interaction.Playwright)
	}

	var lock struct {
		Packages map[string]struct {
			Version      string            `json:"version"`
			Dependencies map[string]string `json:"dependencies"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(read("package-lock.json"), &lock); err != nil {
		t.Fatalf("parse package-lock.json: %v", err)
	}
	if got := lock.Packages[""].Dependencies["playwright"]; got != pins.Interaction.Playwright {
		t.Errorf("package-lock root spec = %q, want %q", got, pins.Interaction.Playwright)
	}
	if got := lock.Packages["node_modules/playwright"].Version; got != pins.Interaction.Playwright {
		t.Errorf("package-lock playwright version = %q, want %q", got, pins.Interaction.Playwright)
	}

	var meta struct {
		PlaywrightVersion string `json:"playwrightVersion"`
		BrowserVersion    string `json:"browserVersion"`
	}
	if err := json.Unmarshal(read("testdata/interaction-parity/reference-meta.json"), &meta); err != nil {
		t.Fatalf("parse reference-meta.json: %v", err)
	}
	if meta.PlaywrightVersion != pins.Interaction.Playwright {
		t.Errorf("reference-meta playwrightVersion = %q, want %q", meta.PlaywrightVersion, pins.Interaction.Playwright)
	}
	if meta.BrowserVersion != pins.Interaction.Chromium {
		t.Errorf("reference-meta browserVersion = %q, want %q", meta.BrowserVersion, pins.Interaction.Chromium)
	}
}

func TestParityOracleCheckLogic(t *testing.T) {
	py := parityPython(t)
	root := parityRepoRoot(t)
	script := `
import importlib.util, pathlib, sys
spec = importlib.util.spec_from_file_location("parity", pathlib.Path(sys.argv[1]))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
pins = m.load_oracle_pins()
assert m.check_oracle_version("9.9.9", pins) is not None, "mismatch accepted"
assert m.check_oracle_version(pins["render"]["playwright"], pins) is None, "pin rejected"
print("ok")
`
	cmd := exec.Command(py, "-c", script, filepath.Join(root, "testdata", "parity.py"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("oracle check logic: %v\n%s", err, out)
	}
}

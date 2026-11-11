# SP-2 V0 Measurability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. User has waived approval gates; execute autonomously.

**Goal:** Make every verification claim falsifiable — pinned+asserted oracles, committed pixel goldens, fail-closed WPT gate, nightly macOS timing gate, honest docs.

**Architecture:** Spec `docs/superpowers/specs/2026-11-12-sp2-v0-measurability-design.md` (commit 19770a4) is the authority. Seven tasks, one commit each. Goldens use `GOOSIE_SYSTEM_FONTS=0` so renders are a function of the binary alone.

**Tech Stack:** Go 1.26.10, Python 3 (parity harness), Node (Playwright scripts), GitHub Actions.

**Verified facts (2026-11-12):** local playwright 1.49.1 / pillow 11.3.0 / numpy 1.26.4 (Python 3.9); node playwright 1.63.0; Chromium builds 131.0.6778.33 (py) and 153.0.8010.12 (node); `v2-gate-check.sh` takes the artifact path as `$1`; `run-wpt.sh` never exports `WPT_RUN_SUITE=1` (silent skip, verified); WPT checkout reuse marker is `.goosie-wpt-commit`; results.json has `total`/`passed`/`failed`/`skipped`/`pass_rate`.

---

### Task 1: `run-wpt.sh` fails closed

**Files:**
- Modify: `scripts/run-wpt.sh` (env export block ~line 113-117; summary block ~line 150-160)
- Test: `test/gatecheck/wpt_script_test.go` (create)

- [ ] **Step 1: Write the failing contract test**

Create `test/gatecheck/wpt_script_test.go`:

```go
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
	root := repoRoot(t)
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
	// checkout then discovers zero tests and the suite skips.
	if err := os.WriteFile(filepath.Join(wptDir, ".goosie-wpt-commit"), []byte(cfg.Commit+"\n"), 0o644); err != nil {
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
		t.Fatalf("run-wpt.sh exited 0 on a skipped suite; output:\n%s", out)
	}
	if !strings.Contains(string(out), "did not run") {
		t.Fatalf("run-wpt.sh failure must say the suite did not run; output:\n%s", out)
	}
}
```

Note: `repoRoot(t)` already exists in `test/gatecheck` (used by parity_harness_test.go); confirm its name with grep and reuse it. If it is named differently (e.g. `parityRepoRoot`), use that helper instead.

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./test/gatecheck/ -run TestRunWPTScriptFailsClosedOnSkippedSuite -count=1 -v`
Expected: FAIL — script exits 0 ("WPT run completed successfully.").

- [ ] **Step 3: Fix the script**

In `scripts/run-wpt.sh`, in the export block, add `WPT_RUN_SUITE` so the suite actually runs:

```bash
# Export environment for the Go test.
export WPT_DIR
export WPT_GOOSIE_BINARY
export WPT_CONFIG
export WPT_OUTPUT_DIR
# Without this the suite skips and the script used to exit 0 anyway.
export WPT_RUN_SUITE=1
```

Then replace the final success/failure block:

```bash
echo ""
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "WPT run completed successfully."
else
  echo "WPT run completed with failures (exit code: $EXIT_CODE)."
fi

exit $EXIT_CODE
```

with:

```bash
echo ""
if [[ $EXIT_CODE -ne 0 ]]; then
  echo "WPT run completed with failures (exit code: $EXIT_CODE)."
  exit "$EXIT_CODE"
fi

# Fail closed: a skipped suite (no results, or zero tests discovered) is not
# a successful run. This is the bug where make wpt printed "completed
# successfully" while TestWPTSuite never executed.
if [[ ! -s "$WPT_OUTPUT_DIR/results.json" ]]; then
  echo "WPT suite did not run: no results.json in $WPT_OUTPUT_DIR" >&2
  exit 1
fi
if command -v python3 &>/dev/null; then
  TOTAL=$(python3 -c "import json;print(json.load(open('$WPT_OUTPUT_DIR/results.json'))['total'])" 2>/dev/null || echo 0)
  if [[ "$TOTAL" == "0" ]]; then
    echo "WPT suite did not run: zero tests discovered" >&2
    exit 1
  fi
fi

echo "WPT run completed successfully."
exit 0
```

- [ ] **Step 4: Run test, verify it passes**

Run: `go test ./test/gatecheck/ -run TestRunWPTScriptFailsClosedOnSkippedSuite -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add scripts/run-wpt.sh test/gatecheck/wpt_script_test.go
git commit -m "fix(wpt): make run-wpt.sh fail closed when the suite skips"
```

---

### Task 2: Pinned, asserted oracles

**Files:**
- Create: `testdata/oracle-pins.json`, `testdata/requirements.txt`, `scripts/oracle.js`
- Modify: `testdata/parity.py` (oracle assert), `package.json`, `package-lock.json` (root spec), `.github/workflows/ci.yml:131`, `scripts/interaction-parity-reference.js`, `scripts/verify-phase*.js` (14 files)
- Test: `test/gatecheck/oracle_pins_test.go` (create)

- [ ] **Step 1: Create the pin files**

`testdata/oracle-pins.json`:

```json
{
  "render": {
    "playwright": "1.49.1",
    "chromium": "131.0.6778.33"
  },
  "interaction": {
    "playwright": "1.63.0",
    "chromium": "153.0.8010.12"
  }
}
```

`testdata/requirements.txt`:

```
# Render-parity oracle and scoring libs. Exact pins: the parity baseline was
# recorded against these versions (see oracle-pins.json). Installable on
# Python 3.9 (local) and 3.12 (CI).
playwright==1.49.1
pillow==11.3.0
numpy==1.26.4
```

- [ ] **Step 2: Assert the oracle in parity.py**

Add near the other path constants (after `KNOWN_GAPS`):

```python
ORACLE_PINS = Path("testdata/oracle-pins.json")
```

Add two functions before `main()`:

```python
def load_oracle_pins() -> dict:
    with ORACLE_PINS.open() as f:
        return json.load(f)


def check_oracle_version(installed: str, pins: dict) -> str | None:
    """Return an error message when the installed playwright is not the
    pinned render oracle, else None. The parity baseline is only meaningful
    against the Chromium build this playwright ships."""
    want = pins["render"]["playwright"]
    if installed != want:
        return (f"oracle mismatch: installed playwright {installed} != pinned "
                f"{want} (testdata/oracle-pins.json); install with "
                f"pip install -r testdata/requirements.txt")
    return None
```

In `main()`, immediately before the `if args.render_chromium:` render block, add:

```python
    if args.render_chromium:
        from importlib.metadata import version as _pkg_version
        err = check_oracle_version(_pkg_version("playwright"), load_oracle_pins())
        if err:
            print(f"ERROR: {err}", file=sys.stderr)
            return 2
```

(Confirm `json` is already imported in parity.py; it is used by write_json_report.)

- [ ] **Step 3: Node pin + assert helper**

`package.json`: change `"playwright": "^1.63.0"` to `"playwright": "1.63.0"`.
`package-lock.json`: in `packages[""].dependencies` change `"playwright": "^1.63.0"` to `"playwright": "1.63.0"` (the `node_modules/playwright` entry already says 1.63.0 — leave it).

Create `scripts/oracle.js`:

```js
// Asserts the installed Node playwright matches the pinned interaction
// oracle in testdata/oracle-pins.json. Interaction references were recorded
// against that exact Chromium build; a different one invalidates them.
const fs = require('fs');
const path = require('path');

function assertOracle(kind) {
  const pinsPath = path.join(__dirname, '..', 'testdata', 'oracle-pins.json');
  const pins = JSON.parse(fs.readFileSync(pinsPath, 'utf8'));
  const pin = pins[kind];
  if (!pin) throw new Error(`oracle: unknown kind '${kind}' in ${pinsPath}`);
  const installed = require('playwright-core/package.json').version;
  if (installed !== pin.playwright) {
    throw new Error(`oracle: ${kind} playwright ${installed} != pinned ${pin.playwright}; run npm ci`);
  }
}

module.exports = { assertOracle };
```

Add as the first require in `scripts/interaction-parity-reference.js` and every `scripts/verify-phase*.js`:

```js
require('./oracle.js').assertOracle('interaction');
```

- [ ] **Step 4: CI installs from the pin file**

`.github/workflows/ci.yml` parity job, replace:

```yaml
          pip install playwright==1.49.1 pillow numpy
```

with:

```yaml
          pip install -r testdata/requirements.txt
```

- [ ] **Step 5: Write the consistency gate test**

Create `test/gatecheck/oracle_pins_test.go`:

```go
package gatecheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type oraclePins struct {
	Render      struct{ Playwright, Chromium string } `json:"render"`
	Interaction struct{ Playwright, Chromium string } `json:"interaction"`
}

// TestOraclePinsAreConsistent keeps every oracle declaration in the repo
// agreeing with testdata/oracle-pins.json: the CI requirements file, the
// Node manifest and lockfile, and the recorded interaction references.
// Fail closed by design: no t.Skip on any path.
func TestOraclePinsAreConsistent(t *testing.T) {
	root := repoRoot(t)
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

// TestParityOracleCheckLogic exercises check_oracle_version without needing
// a wrong playwright installed: a mismatch must produce an error, the pinned
// version must produce none.
func TestParityOracleCheckLogic(t *testing.T) {
	py := parityPython(t)
	root := repoRoot(t)
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
	cmd := execCommand(py, "-c", script, filepath.Join(root, "testdata", "parity.py"))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("oracle check logic: %v\n%s", err, out)
	}
}
```

Note: reuse the existing helpers in `test/gatecheck` — confirm the actual names of `repoRoot`, `parityPython`, and the exec wrapper (parity_harness_test.go uses `exec.Command` directly; if there is no `execCommand` helper, use `exec.Command` and add the `os/exec` import). Importing parity.py executes its module top level — verify it has an `if __name__ == "__main__":` guard so import does not run main(); if it does not, add the guard as part of this task.

- [ ] **Step 6: Run the tests**

Run: `go test ./test/gatecheck/ -run 'TestOraclePins|TestParityOracleCheckLogic' -count=1 -v`
Expected: PASS. Then `python3 testdata/parity.py --render-chromium --binary ./bin/goosie` is NOT run here (full parity is Task 8); instead verify the assert fires:
`python3 -c "import importlib.util,pathlib; s=importlib.util.spec_from_file_location('p', pathlib.Path('testdata/parity.py')); m=importlib.util.module_from_spec(s); s.loader.exec_module(m); print(m.check_oracle_version('0.0.1', m.load_oracle_pins()))"`
Expected: prints the mismatch message.

- [ ] **Step 7: Commit**

```bash
git add testdata/oracle-pins.json testdata/requirements.txt scripts/oracle.js testdata/parity.py package.json package-lock.json .github/workflows/ci.yml scripts/interaction-parity-reference.js scripts/verify-phase*.js test/gatecheck/oracle_pins_test.go
git commit -m "test(oracle): pin both Chromium oracles and assert them at test start"
```

---

### Task 3: Committed PNG goldens

**Files:**
- Create: `test/goldens/golden_test.go`, `testdata/render-goldens/*.png` (7), `testdata/render-goldens/manifest.json`

- [ ] **Step 1: Write the golden test**

Create `test/goldens/golden_test.go`:

```go
// Package goldens_test is the pixel regression ratchet: committed renders of
// one fixture per layout family, produced with GOOSIE_SYSTEM_FONTS=0 so the
// pixels are a function of the binary alone (internal/raster/fonts.go).
// A deliberate engine change re-records in the same commit via
// GOOSIE_RECORD_GOLDENS=1 go test ./test/goldens/ -run TestRecordGoldens.
package goldens_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

type goldenEntry struct {
	Family  string `json:"family"`
	Fixture string `json:"fixture"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
}

type manifest struct {
	Goldens []goldenEntry `json:"goldens"`
	Render  struct {
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		DPR         int    `json:"dpr"`
		SystemFonts string `json:"systemFonts"`
	} `json:"render"`
}

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func headlessBinary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		dir, err := os.MkdirTemp("", "goosie-goldens-bin")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "goosie-headless")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/goosie-headless")
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("build goosie-headless: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return binPath
}

func loadManifest(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "render-goldens", "manifest.json"))
	if err != nil {
		t.Fatalf("read golden manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse golden manifest: %v", err)
	}
	return m
}

func renderFixture(t *testing.T, bin, fixture string, m manifest, out string) {
	t.Helper()
	src := filepath.Join(repoRoot(t), "testdata", "render", fixture)
	cmd := exec.Command(bin, "-in", src, "-out", out,
		"-width", fmt.Sprint(m.Render.Width),
		"-height", fmt.Sprint(m.Render.Height),
		"-dpr", fmt.Sprint(m.Render.DPR))
	cmd.Env = append(os.Environ(), "GOOSIE_SYSTEM_FONTS=0")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render %s: %v\n%s", fixture, err, combined)
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

// comparePixels returns the number of differing pixels and the first
// differing coordinate. Images are compared in RGBA so color-model
// differences in the PNG encoding cannot mask a pixel change.
func comparePixels(a, b image.Image) (int, image.Point) {
	ba, bb := a.Bounds(), b.Bounds()
	if ba != bb {
		return -1, image.Point{}
	}
	diffs := 0
	first := image.Point{}
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb2, ba2 := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb2 || aa != ba2 {
				if diffs == 0 {
					first = image.Point{X: x, Y: y}
				}
				diffs++
			}
		}
	}
	return diffs, first
}

func TestGoldensMatch(t *testing.T) {
	bin := headlessBinary(t)
	m := loadManifest(t)
	if len(m.Goldens) == 0 {
		t.Fatal("golden manifest is empty")
	}
	for _, g := range m.Goldens {
		t.Run(g.Family, func(t *testing.T) {
			goldenPath := filepath.Join(repoRoot(t), "testdata", "render-goldens", g.File)
			raw, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != g.SHA256 {
				t.Fatalf("golden %s sha256 mismatch: file was edited without updating manifest.json", g.File)
			}
			actual := filepath.Join(t.TempDir(), g.File)
			renderFixture(t, bin, g.Fixture, m, actual)
			diffs, first := comparePixels(decodePNG(t, goldenPath), decodePNG(t, actual))
			if diffs != 0 {
				t.Fatalf("%s: %d pixel(s) differ from the committed golden (first at %v); "+
					"if this engine change is deliberate, re-record with "+
					"GOOSIE_RECORD_GOLDENS=1 go test ./test/goldens/ -run TestRecordGoldens",
					g.Fixture, diffs, first)
			}
		})
	}
}

// TestGoldenManifestCoversFamilies keeps the ratchet honest: every layout
// family the spec names must keep a golden, and every fixture must exist.
func TestGoldenManifestCoversFamilies(t *testing.T) {
	m := loadManifest(t)
	want := []string{"block", "inline", "float", "flex", "grid", "table", "positioning"}
	seen := map[string]bool{}
	for _, g := range m.Goldens {
		seen[g.Family] = true
		if _, err := os.Stat(filepath.Join(repoRoot(t), "testdata", "render", g.Fixture)); err != nil {
			t.Errorf("golden fixture missing: %s", g.Fixture)
		}
	}
	for _, family := range want {
		if !seen[family] {
			t.Errorf("layout family %q lost its golden", family)
		}
	}
}

// TestGoldenComparatorDetectsOnePixel is the V0 acceptance: a deliberate
// one-pixel change must be caught. It perturbs a decoded golden in memory;
// no committed file is touched.
func TestGoldenComparatorDetectsOnePixel(t *testing.T) {
	m := loadManifest(t)
	if len(m.Goldens) == 0 {
		t.Fatal("golden manifest is empty")
	}
	g := m.Goldens[0]
	img := decodePNG(t, filepath.Join(repoRoot(t), "testdata", "render-goldens", g.File))
	b := img.Bounds()
	px := image.Point{X: b.Min.X + b.Dx()/2, Y: b.Min.Y + b.Dy()/2}
	r, g2, bl, a := img.At(px.X, px.Y).RGBA()
	perturbed := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			perturbed.Set(x, y, img.At(x, y))
		}
	}
	// Flip one channel by one 16-bit step; keep alpha opaque.
	perturbed.Set(px.X, px.Y, rgba64{r: r ^ 0x100, g: g2, b: bl, a: a})
	diffs, first := comparePixels(img, perturbed)
	if diffs != 1 || first != px {
		t.Fatalf("one-pixel perturbation: diffs=%d first=%v, want 1 at %v", diffs, first, px)
	}
}

type rgba64 struct{ r, g, b, a uint32 }

func (c rgba64) RGBA() (uint32, uint32, uint32, uint32) { return c.r, c.g, c.b, c.a }

// TestRenderIsDeterministic guards the golden premise: two renders of the
// same fixture with system fonts off must be byte-identical.
func TestRenderIsDeterministic(t *testing.T) {
	bin := headlessBinary(t)
	m := loadManifest(t)
	g := m.Goldens[0]
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	renderFixture(t, bin, g.Fixture, m, a)
	renderFixture(t, bin, g.Fixture, m, b)
	ra, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ra, rb) {
		t.Fatal("two embedded-font renders of the same fixture differ; goldens cannot be a binary-only function")
	}
}

// TestRecordGoldens re-records the committed baselines. It only runs when
// explicitly asked, so a normal test run can never overwrite evidence.
func TestRecordGoldens(t *testing.T) {
	if os.Getenv("GOOSIE_RECORD_GOLDENS") != "1" {
		t.Skip("set GOOSIE_RECORD_GOLDENS=1 to re-record the golden baselines")
	}
	bin := headlessBinary(t)
	m := loadManifest(t)
	dir := filepath.Join(repoRoot(t), "testdata", "render-goldens")
	for i := range m.Goldens {
		g := &m.Goldens[i]
		out := filepath.Join(dir, g.File)
		renderFixture(t, bin, g.Fixture, m, out)
		raw, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		g.SHA256 = hex.EncodeToString(sum[:])
		t.Logf("recorded %s (%s)", g.File, g.Fixture)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Create the manifest (hashes filled by recording)**

Create `testdata/render-goldens/manifest.json`:

```json
{
  "goldens": [
    {"family": "block", "fixture": "basic/01-headings-paragraphs.html", "file": "block.png", "sha256": ""},
    {"family": "inline", "fixture": "basic/02-inline-elements.html", "file": "inline.png", "sha256": ""},
    {"family": "float", "fixture": "floats/01-clear-directions.html", "file": "float.png", "sha256": ""},
    {"family": "flex", "fixture": "flex/01-flex-row.html", "file": "flex.png", "sha256": ""},
    {"family": "grid", "fixture": "grid/01-basic-grid.html", "file": "grid.png", "sha256": ""},
    {"family": "table", "fixture": "tables/01-basic-table.html", "file": "table.png", "sha256": ""},
    {"family": "positioning", "fixture": "positioning/02-absolute.html", "file": "positioning.png", "sha256": ""}
  ],
  "render": {"width": 800, "height": 600, "dpr": 1, "systemFonts": "off"}
}
```

- [ ] **Step 3: Verify the comparator tests fail before goldens exist**

Run: `go test ./test/goldens/ -run 'TestGoldenComparatorDetectsOnePixel|TestGoldensMatch' -count=1 -v`
Expected: FAIL (missing PNG files). This is the red state.

- [ ] **Step 4: Record the goldens**

Run: `GOOSIE_RECORD_GOLDENS=1 go test ./test/goldens/ -run TestRecordGoldens -count=1 -v`
Expected: PASS, 7 "recorded" log lines, PNGs + updated manifest on disk.

- [ ] **Step 5: Verify green**

Run: `go test ./test/goldens/ -count=1 -v`
Expected: all PASS (TestRecordGoldens skips).

- [ ] **Step 6: Commit**

```bash
git add test/goldens/ testdata/render-goldens/
git commit -m "test(goldens): commit embedded-font pixel baselines for seven layout families"
```

---

### Task 4: Ratchet the corpus floor

**Files:**
- Modify: `testdata/parity.py:75` (and the ratchet comment log above it)

- [ ] **Step 1: Raise the floor**

Change `MIN_SCORED_FIXTURES = 93` to `MIN_SCORED_FIXTURES = 105` and append to the dated ratchet comment block above it:

```python
# 105: SP-2 (2026-11-12) locks the full corpus; the floor had 12 fixtures of
# un-ratcheted slack (93 vs 105 scored).
```

(Match the existing comment style; the number 105 must equal the actual HTML fixture count — verify with `find testdata/render -name '*.html' | wc -l`.)

- [ ] **Step 2: Verify**

Run: `go test ./test/gatecheck/ -run TestParityFixtureFloorMatchesCorpus -count=1 -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add testdata/parity.py
git commit -m "test(parity): ratchet the fixture floor to the full 105-fixture corpus"
```

---

### Task 5: CI hardening (race cache, darwin coverage)

**Files:**
- Modify: `.github/workflows/ci.yml` (race job line ~47; darwin job lines ~60-84)

- [ ] **Step 1: Race job busts the cache**

Change the race job's run line from `go test -race ./...` to:

```yaml
        run: go test -race -count=1 ./...
```

- [ ] **Step 2: Darwin job covers engine + gatecheck**

In the darwin job, before the test step, add:

```yaml
      - name: Install jq for the gate-check contract tests
        run: brew install jq
```

and extend the test command's package list with `./test/engine/ ./test/gatecheck/` (keep `-race -count=1 -v`). Read the current step first and edit precisely.

- [ ] **Step 3: Validate YAML**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('ok')"` (if pyyaml absent, `actionlint .github/workflows/ci.yml` if installed; else careful inspection and note it).
Expected: ok.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: bust the race cache and cover engine+gatecheck on macOS ARM64"
```

---

### Task 6: Nightly workflow (macOS timing gate + WPT conformance)

**Files:**
- Create: `.github/workflows/nightly.yml`

- [ ] **Step 1: Write the workflow**

```yaml
name: Nightly

# Timing budgets gate here, not on per-PR CI (AGENTS.md): the macOS runner
# exercises scripts/v2-gate-check.sh, and the WPT conformance suite reports
# against its pinned upstream commit with the min_pass_rate ratchet.
on:
  schedule:
    - cron: "0 9 * * *"
  workflow_dispatch:

jobs:
  macos-gate:
    name: macOS pacing gate
    runs-on: macos-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Install jq
        run: brew install jq
      - name: Run the gate scene
        run: go run ./cmd/goosie -gate -scene checkerboard -frames 600 -out /tmp/gate.json
      - name: Assert the pacing budgets
        run: bash scripts/v2-gate-check.sh /tmp/gate.json
      - name: Upload gate artifact
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: gate-json
          path: /tmp/gate.json

  wpt-conformance:
    name: WPT conformance (pinned upstream)
    runs-on: ubuntu-latest
    timeout-minutes: 60
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Cache the pinned WPT checkout
        uses: actions/cache@v4
        with:
          path: /tmp/goosie-wpt
          key: wpt-${{ hashFiles('testdata/wpt-config.json') }}
      - name: Build goosie
        run: go build -o goosie ./cmd/goosie
      - name: Run the conformance suite
        run: >
          WPT_RUN_SUITE=1 WPT_DIR=/tmp/goosie-wpt WPT_GOOSIE_BINARY=./goosie
          go test -timeout 40m -run TestWPTSuite ./test/wpt/
      - name: Upload WPT report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: wpt-report
          path: |
            /tmp/wpt-results/results.json
            /tmp/wpt-results/report.html
```

- [ ] **Step 2: Validate**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/nightly.yml')); print('ok')"` (or actionlint).
Expected: ok.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/nightly.yml
git commit -m "ci(nightly): wire the macOS pacing gate and WPT conformance report"
```

---

### Task 7: Docs authority + stale review corrections

**Files:**
- Modify: `docs/README.md`, `docs/engine-compatibility-review.md` (~lines 344, 354, 393-403)

- [ ] **Step 1: Align docs/README.md with AGENTS.md**

Rewrite the section-1 rule line (currently "📌 **Quy tắc:** Mọi công việc ... BẮT BUỘC ... docs/spec/") to:

```markdown
> 📌 **Quy tắc:** `docs/spec/` là bộ yêu cầu sản phẩm và kiến trúc tham chiếu (SRS, tầm nhìn, blueprint, WBS). Thẩm quyền lập kế hoạch và nghiệm thu hiện hành là **[`roadmap-v2.md`](roadmap-v2.md)** — trạng thái hiện tại và các cổng phát hành theo thứ tự, như quy định trong `AGENTS.md`. Không lập kế hoạch lại những gì roadmap đã ghi là hoàn thành.
```

Add the missing table row after the SYSTEM_ARCHITECTURE_BLUEPRINT row:

```markdown
| **[`SPRINT_PLAN_AND_WBS.md`](spec/SPRINT_PLAN_AND_WBS.md)** | 🗓️ **Delivery Plan** | Phân rã công việc (WBS) và kế hoạch sprint tham chiếu. |
```

Move the `roadmap-v2.md` bullet out of section 2 ("Thực Nghiệm & Lịch Sử") into a new short section before it:

```markdown
## 🧭 Thẩm quyền thực thi (Execution Authority)

- **[`roadmap-v2.md`](roadmap-v2.md)**: đánh giá mức sẵn sàng sản xuất hiện hành và các cổng phát hành theo thứ tự (P0-P3). Đọc trước khi lập kế hoạch tính năng mới; không lên lại phần đã ghi là hoàn thành.
```

and delete the old roadmap bullet from section 2. Keep the rest of section 2 unchanged.

- [ ] **Step 2: Correct the stale review lines**

In `docs/engine-compatibility-review.md`, replace the claim at ~line 344 ("Every testharness.js test is skipped with the reason 'requires JS DOM' (test/wpt/curate.go:370-373)") with:

```markdown
The blanket testharness.js skip is gone: `test/wpt/curate.go` curates ~25
`Type: TestHarness` directories and `test/wpt/testharness.go` executes them
in Goosie's real document (corrected 2026-11-12, SP-2 audit). What remains
open is running the upstream subset in CI — the nightly `wpt-conformance`
job now does this against the pinned commit.
```

Replace the claim at ~line 354 (suite inside `go test ./...`, paid twice) with:

```markdown
`TestWPTSuite` is opt-in (`WPT_RUN_SUITE=1`, test/wpt/wpt_test.go:63-66);
`go test ./...` never starts it. CI runs only the 8-pair local pilot
(corrected 2026-11-12, SP-2 audit).
```

Read the exact current wording first and preserve surrounding markdown structure.

- [ ] **Step 3: Annotate the V0 checklist**

Under the V0 bullets (~lines 395-399), append a status block:

```markdown
**V0 status (2026-11-12, SP-2):** license pin, QSA race timeout, Go 1.26.10,
macOS ARM64 job, testharness unskip (in code), and Chromium-build recording
were already done before SP-2. SP-2 adds: pinned+asserted oracles
(testdata/oracle-pins.json), seven committed embedded-font goldens
(testdata/render-goldens/), fail-closed run-wpt.sh, the nightly macOS pacing
gate and WPT conformance job, the 105-fixture floor ratchet, and the docs
authority alignment. Oracle unification (one Chromium for both corpora) is
deferred with its own re-baseline budget.
```

- [ ] **Step 4: Commit**

```bash
git add docs/README.md docs/engine-compatibility-review.md
git commit -m "docs: align authority with AGENTS.md and correct stale V0 claims"
```

---

### Task 8: Full verification + completion summary

**Files:** none (verification only)

- [ ] **Step 1: Full local matrix**

```bash
go build ./... && go vet -all ./... && go test ./... -count=1
go test -race ./cmd/goosie/ ./internal/engine/ ./test/engine/ ./test/gatecheck/ ./test/goldens/ ./internal/archtest/ -count=1
make parity
```

Expected: all PASS; parity 105/105 at or above the 96.39% SP-1 baseline.

- [ ] **Step 2: WPT baseline evidence**

The full-suite baseline run (`WPT_RUN_SUITE=1 ... go test -run TestWPTSuite`) was started during planning; read `/tmp/wpt-results/results.json`. Record total/passed/pass_rate and confirm at least one testharness test passed (acceptance 2). If pass_rate < 85.0, do NOT lower `min_pass_rate`; record that the nightly job is expected red until engine work lifts it.

- [ ] **Step 3: Limits to record in the summary**

- No push authorized: remote CI/nightly green cannot be observed; local equivalents are the evidence.
- macOS pacing budgets not exercisable in this display-less session; the script contract is covered by test/gatecheck.
- Node `npm ci` was not re-run (lockfile root spec edited by hand); verify `node -e "require('./scripts/oracle.js').assertOracle('interaction')"` passes.

- [ ] **Step 4: Final history check**

```bash
git --no-pager log --oneline 19770a4..HEAD && git status --short
```

Expected: 7 task commits, clean tree.
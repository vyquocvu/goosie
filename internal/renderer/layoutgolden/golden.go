// Package layoutgolden provides golden layout tests for the renderer.
//
// Golden layout tests deterministically serialize the layout tree produced
// by the layout engine for a fixed (HTML, CSS, viewport) fixture and
// compare the result against a checked-in text snapshot. They complement
// the raster golden tests in internal/renderer/frame/golden by catching
// regressions in box-model, flex, inline, and table layout before pixels
// are even drawn.
//
// Design constraints:
//
//   - Inputs must be deterministic: integer/float-strict viewports, inline
//     CSS, and no time-dependent or network-dependent resources.
//   - Serialization must be stable across Go versions. We round all
//     floats to two decimals and emit only structural fields (Box
//     geometry, display type, padding, margin, flex parameters). Volatile
//     fields (NodeID, colors, font metric internals) are intentionally
//     omitted.
//   - Failed comparisons write the new layout snapshot to a sibling
//     directory (testdata/golden-layout-update/<name>.txt) so a reviewer
//     can diff against the committed snapshot before accepting changes
//     with GOOSIE_UPDATE_GOLDEN=1.
package layoutgolden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/renderer"
	"golang.org/x/net/html"
)

// Config controls the comparison behavior for golden layout tests.
type Config struct {
	// GoldenDir is the directory holding committed golden snapshots.
	// Default: "testdata/golden-layout".
	GoldenDir string

	// UpdateDir is the directory where -update writes new golden files
	// when the comparison fails or no committed golden exists.
	// Default: "testdata/golden-layout-update".
	UpdateDir string

	// SnapshotLabel is an optional header line written at the top of
	// each snapshot. Useful for identifying the fixture in diff output.
	SnapshotLabel string
}

// DefaultConfig returns a Config with sensible defaults that mirror the
// layout golden test directories relative to the test package.
func DefaultConfig() Config {
	return Config{
		GoldenDir:     "testdata/golden-layout",
		UpdateDir:     "testdata/golden-layout-update",
		SnapshotLabel: "goosie layout golden snapshot",
	}
}

// AssertGoldenLayout runs the renderer layout engine for the given
// fixture (HTML, CSS, viewport) and asserts the serialized layout tree
// matches the committed golden snapshot. When the environment variable
// GOOSIE_UPDATE_GOLDEN=1 is set, the snapshot is rewritten instead of
// compared.
//
// The harness is intentionally tolerant of layout engine API additions:
// new layout box fields become part of the snapshot only after a
// contributor explicitly regenerates the goldens, so the diff is
// reviewable.
func AssertGoldenLayout(t *testing.T, name string, cfg Config, viewportW, viewportH float32, htmlDoc, cssDoc string) {
	t.Helper()

	if cfg.GoldenDir == "" {
		cfg.GoldenDir = DefaultConfig().GoldenDir
	}
	if cfg.UpdateDir == "" {
		cfg.UpdateDir = DefaultConfig().UpdateDir
	}

	// Render and layout.
	tree, err := parseHTMLToRenderTree(htmlDoc)
	if err != nil {
		t.Fatalf("parseHTMLToRenderTree(%q): %v", name, err)
	}
	stylesheet, err := css.NewParser(cssDoc).Parse()
	if err != nil {
		t.Fatalf("css parse(%q): %v", name, err)
	}
	styleManager := renderer.NewStyleManagerWithViewport(stylesheet, viewportW, viewportH)
	styleManager.ApplyStyles(tree)
	engine := renderer.NewLayoutEngine(viewportW, viewportH)
	layoutRoot := engine.ComputeLayout(tree)

	// Serialize and snapshot.
	tagOf := TagLookup(tree)
	got := SerializeLayoutBox(layoutRoot, tagOf, cfg.SnapshotLabel)
	goldenPath := filepath.Join(cfg.GoldenDir, name+".txt")

	if os.Getenv("GOOSIE_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(cfg.GoldenDir, 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := writeFile(goldenPath, got); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			updatePath := filepath.Join(cfg.UpdateDir, name+".txt")
			_ = os.MkdirAll(cfg.UpdateDir, 0o755)
			_ = writeFile(updatePath, got)
			t.Fatalf("no golden at %s — wrote candidate to %s (set GOOSIE_UPDATE_GOLDEN=1 to accept)",
				goldenPath, updatePath)
		}
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}

	if got != string(want) {
		updatePath := filepath.Join(cfg.UpdateDir, name+".txt")
		_ = os.MkdirAll(cfg.UpdateDir, 0o755)
		_ = writeFile(updatePath, got)
		t.Fatalf("layout mismatch %q\n  committed: %s\n  candidate: %s",
			name, goldenPath, updatePath)
	}
}

// RunDeterminismGuard runs the harness twice for the same fixture and
// asserts both serializations match. This is a regression guard against
// non-determinism creep into the layout engine — for example, accidental
// map-iteration dependence on computed style or layout cache state.
//
// Callers should invoke RunDeterminismGuard once per fixture from the
// same test function that calls AssertGoldenLayout.
func RunDeterminismGuard(t *testing.T, name string, viewportW, viewportH float32, htmlDoc, cssDoc string) {
	t.Helper()

	first := RenderAndSerialize(t, viewportW, viewportH, htmlDoc, cssDoc)
	second := RenderAndSerialize(t, viewportW, viewportH, htmlDoc, cssDoc)
	if first != second {
		t.Fatalf("determinism guard %q: two consecutive runs produced different layouts", name)
	}
}

// RenderAndSerialize is a helper that runs the layout pipeline and
// returns the deterministic text snapshot.
func RenderAndSerialize(t *testing.T, viewportW, viewportH float32, htmlDoc, cssDoc string) string {
	t.Helper()
	tree, err := parseHTMLToRenderTree(htmlDoc)
	if err != nil {
		t.Fatalf("parseHTMLToRenderTree: %v", err)
	}
	stylesheet, err := css.NewParser(cssDoc).Parse()
	if err != nil {
		t.Fatalf("css parse: %v", err)
	}
	styleManager := renderer.NewStyleManagerWithViewport(stylesheet, viewportW, viewportH)
	styleManager.ApplyStyles(tree)
	engine := renderer.NewLayoutEngine(viewportW, viewportH)
	layoutRoot := engine.ComputeLayout(tree)
	return SerializeLayoutBox(layoutRoot, TagLookup(tree), "")
}

// parseHTMLToRenderTree parses an HTML string and returns the renderer
// RenderNode tree rooted at the document element. It uses
// golang.org/x/net/html for tokenization to keep the fixture parsing
// path identical to the engine's real-input path; the rendered tree is
// built via renderer's BuildRenderTree helper exposed via internal
// package use.
func parseHTMLToRenderTree(htmlStr string) (*renderer.RenderNode, error) {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
	}
	var htmlNode *html.Node
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "html" {
			htmlNode = c
			break
		}
	}
	if htmlNode == nil {
		return nil, fmt.Errorf("html element not found")
	}
	return renderer.BuildRenderTree(htmlNode), nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

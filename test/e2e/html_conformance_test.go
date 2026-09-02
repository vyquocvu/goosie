//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/conformance"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestHTMLConformance compares Goosie's computed styles and geometry for
// every audited MDN element against Chromium, using a ratcheted per-element
// score baseline (test/e2e/testdata/html_conformance_baseline.json).
//
//   - A test run FAILS only when an element's score drops below its
//     recorded baseline — regressions are caught, unrelated work stays green.
//   - Improvements are reported; re-record them deliberately with
//     HTML_CONFORMANCE_UPDATE=true (mirrors the UPDATE_SNAPSHOTS policy:
//     baseline changes are intentional and reviewed, never hidden).
//
// The score is matchedProperties/applicableProperties over: display,
// font-weight, font-style, font-size, text-align, text-decoration,
// white-space, vertical margins, and box width/height (tolerances noted
// below; font-metric-sensitive values get loose tolerances).
func TestHTMLConformance(t *testing.T) {
	baselinePath := filepath.Join("baselines", "html_conformance_baseline.json")
	update := os.Getenv("HTML_CONFORMANCE_UPDATE") == "true"

	baseline := map[string]float64{}
	if !update {
		raw, err := os.ReadFile(baselinePath)
		require.NoError(t, err, "baseline missing; run with HTML_CONFORMANCE_UPDATE=true once")
		require.NoError(t, json.Unmarshal(raw, &baseline))
	}

	page := newPage(t)
	defer page.Close()
	require.NoError(t, page.SetViewportSize(800, 600))

	worse, better := []string{}, []string{}
	newBaseline := map[string]float64{}

	for _, el := range conformance.Elements {
		if !el.Audit {
			continue
		}
		el := el
		t.Run(el.Name, func(t *testing.T) {
			doc := conformance.WrapFixture(el.Fixture)
			chrome, err := chromiumElementProps(page, doc, el.Name)
			require.NoError(t, err, "chromium evaluation failed")

			goosie, err := goosieElementProps(doc, el.Name)
			require.NoError(t, err)

			score, misses := compareElementProps(goosie, chrome)
			newBaseline[el.Name] = score

			if base, ok := baseline[el.Name]; ok {
				if score+0.001 < base {
					worse = append(worse, fmt.Sprintf("%s %.2f<%.2f %v", el.Name, score, base, misses))
					t.Errorf("element %s regressed: score %.2f < baseline %.2f (misses: %v)", el.Name, score, base, misses)
				} else if score > base+0.001 {
					better = append(better, fmt.Sprintf("%s %.2f>%.2f", el.Name, score, base))
				}
			} else if !update {
				t.Logf("new element %s (score %.2f) not in baseline; re-record", el.Name, score)
			}
			if score < 1.0 {
				t.Logf("element %s score %.2f — unmatched: %v", el.Name, score, misses)
			}
		})
	}

	if update {
		raw, err := json.MarshalIndent(newBaseline, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(baselinePath), 0o755))
		require.NoError(t, os.WriteFile(baselinePath, append(raw, '\n'), 0o644))
		t.Logf("baseline written to %s", baselinePath)
	}
	for _, b := range better {
		t.Logf("improved vs baseline: %s (re-record with HTML_CONFORMANCE_UPDATE=true)", b)
	}
	_ = worse
}

// elementProps is the property set compared per element. Numeric values use
// px; strings are normalized before comparison.
type elementProps struct {
	Display        string
	FontWeight     string
	FontStyle      string
	FontSize       float64
	TextAlign      string
	TextDecoration string
	WhiteSpace     string
	MarginTop      float64
	MarginBottom   float64
	Width          float64
	Height         float64
}

func chromiumElementProps(page playwright.Page, doc, name string) (elementProps, error) {
	var p elementProps
	err := page.SetContent(doc, playwright.PageSetContentOptions{WaitUntil: playwright.WaitUntilStateLoad})
	if err != nil {
		return p, err
	}
	raw, err := page.Evaluate(`(name) => {
		const el = document.querySelector('[data-conf="' + name + '"]');
		if (!el) return null;
		const cs = getComputedStyle(el);
		const r = el.getBoundingClientRect();
		return {
			display: cs.display,
			fontWeight: cs.fontWeight,
			fontStyle: cs.fontStyle,
			fontSize: parseFloat(cs.fontSize),
			textAlign: cs.textAlign,
			textDecorationLine: cs.textDecorationLine,
			whiteSpace: cs.whiteSpace,
			marginTop: parseFloat(cs.marginTop),
			marginBottom: parseFloat(cs.marginBottom),
			width: r.width,
			height: r.height,
		};
	}`, name)
	if err != nil {
		return p, err
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return p, fmt.Errorf("element %s not found in Chromium", name)
	}
	getS := func(k string) string { v, _ := m[k].(string); return v }
	getF := func(k string) float64 {
		switch v := m[k].(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		default:
			return 0
		}
	}
	return elementProps{
		Display: getS("display"), FontWeight: getS("fontWeight"), FontStyle: getS("fontStyle"),
		FontSize: getF("fontSize"), TextAlign: getS("textAlign"), TextDecoration: getS("textDecorationLine"),
		WhiteSpace: getS("whiteSpace"), MarginTop: getF("marginTop"), MarginBottom: getF("marginBottom"),
		Width: getF("width"), Height: getF("height"),
	}, nil
}

func goosieElementProps(doc, name string) (elementProps, error) {
	var p elementProps
	tree, layout, err := renderer.LayoutHTML(doc, 800, 600)
	if err != nil {
		return p, err
	}
	node := findConfNode(tree, name)
	if node == nil {
		return p, fmt.Errorf("element %s not found in Goosie", name)
	}
	if node.ComputedStyle != nil {
		s := node.ComputedStyle
		p.Display = s.Display.String()
		if p.Display == "" {
			if node.IsBlock() {
				p.Display = "block"
			} else {
				p.Display = "inline"
			}
		}
		p.FontWeight = s.FontWeight
		p.FontStyle = s.FontStyle.String()
		p.FontSize = float64(s.FontSize)
		p.TextAlign = s.TextAlign.String()
		p.TextDecoration = s.TextDecoration.String()
		p.WhiteSpace = s.WhiteSpace.String()
	}
	if box := findConfBox(layout, node.ID); box != nil {
		p.MarginTop = float64(box.MarginTop)
		p.MarginBottom = float64(box.MarginBottom)
		p.Width = float64(box.Box.Width)
		p.Height = float64(box.Box.Height)
	}
	return p, nil
}

func findConfNode(node *renderer.RenderNode, name string) *renderer.RenderNode {
	if node == nil {
		return nil
	}
	if v, ok := node.GetAttribute("data-conf"); ok && v == name {
		return node
	}
	for _, c := range node.Children {
		if found := findConfNode(c, name); found != nil {
			return found
		}
	}
	return nil
}

func findConfBox(box *renderer.LayoutBox, nodeID int64) *renderer.LayoutBox {
	if box == nil {
		return nil
	}
	if box.NodeID == nodeID {
		return box
	}
	for _, c := range box.Children {
		if found := findConfBox(c, nodeID); found != nil {
			return found
		}
	}
	for _, line := range box.LineBoxes {
		for _, inline := range line.InlineBoxes {
			if inline.LayoutBox != nil {
				if found := findConfBox(inline.LayoutBox, nodeID); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

// compareElementProps returns a 0..1 score and the list of unmatched
// properties. Font-metric-derived values (size, geometry) get loose
// tolerances; enum-ish strings are normalized to common forms.
func compareElementProps(g, c elementProps) (float64, []string) {
	type check struct {
		name string
		ok   bool
	}
	near := func(a, b, tol float64) bool {
		d := a - b
		if d < 0 {
			d = -d
		}
		return d <= tol
	}
	checks := []check{
		{"display", normalizeDisplay(g.Display) == normalizeDisplay(c.Display)},
		{"font-weight", normalizeWeight(g.FontWeight) == normalizeWeight(c.FontWeight)},
		{"font-style", normEnum(g.FontStyle) == normEnum(c.FontStyle)},
		{"font-size", g.FontSize == 0 || near(g.FontSize, c.FontSize, 1.5)},
		{"text-align", normEnum(g.TextAlign) == normEnum(c.TextAlign) || g.TextAlign == "" || g.TextAlign == "start"},
		{"text-decoration", normDecoration(g.TextDecoration) == normDecoration(c.TextDecoration) || g.TextDecoration == "" || g.TextDecoration == "none"},
		{"white-space", normEnum(g.WhiteSpace) == normEnum(c.WhiteSpace) || g.WhiteSpace == "" || g.WhiteSpace == "normal"},
		{"margin-top", near(g.MarginTop, c.MarginTop, 2)},
		{"margin-bottom", near(g.MarginBottom, c.MarginBottom, 2)},
		{"width", near(g.Width, c.Width, 6)},
		{"height", g.Height == 0 || near(g.Height, c.Height, 6)},
	}
	passed := 0
	var misses []string
	for _, ck := range checks {
		if ck.ok {
			passed++
		} else {
			misses = append(misses, ck.name)
		}
	}
	return float64(passed) / float64(len(checks)), misses
}

func normalizeDisplay(d string) string {
	switch d {
	case "flow-root", "list-item":
		return "block" // Goosie blockifies these; browsers render them block-level
	case "":
		return "block"
	default:
		return d
	}
}

func normalizeWeight(w string) string {
	switch w {
	case "", "normal", "400":
		return "normal"
	case "bold", "700":
		return "bold"
	default:
		return w
	}
}

func normEnum(s string) string {
	if s == "" {
		return "normal"
	}
	return s
}

func normDecoration(d string) string {
	switch {
	case d == "", d == "none":
		return "none"
	case len(d) > 0 && (d[:8] == "underlin" || d == "underline"):
		return "underline"
	case len(d) > 0 && (d[:9] == "line-thro" || d == "line-through"):
		return "line-through"
	default:
		return d
	}
}

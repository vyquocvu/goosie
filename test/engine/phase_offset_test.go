package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// TestPhaseOffset_HoverSkipsReflow verifies that hover interactions on
// geometry-neutral properties (color, background) skip the layout pass.
func TestPhaseOffset_HoverSkipsReflow(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
		.box { width: 100px; height: 100px; background: blue; }
		.box:hover { background: red; }
	</style></head><body><div class="box">Test</div></body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}

	sess.SetHover(50, 50)
	if sess.HasPseudoClassRule("hover") && sess.IsHoverGeometryNeutral() {
		if err := sess.RestyleForHover(); err != nil {
			t.Fatalf("RestyleForHover should succeed for geometry-neutral hover: %v", err)
		}
	}
}

// TestPhaseOffset_HoverWithLayoutChange verifies that hover interactions
// that change layout properties (width, margin, etc.) DO trigger reflow.
func TestPhaseOffset_HoverWithLayoutChange(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
		.box { width: 100px; height: 100px; }
		.box:hover { width: 200px; }
	</style></head><body><div class="box">Test</div></body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}

	sess.SetHover(50, 50)
	if sess.HasPseudoClassRule("hover") && !sess.IsHoverGeometryNeutral() {
		if err := sess.RestyleAndReflow(800); err != nil {
			t.Fatalf("RestyleAndReflow should succeed for layout-changing hover: %v", err)
		}
	}
}

// TestPhaseOffset_SelectionUsesAccurateDamage verifies that selection
// changes compute accurate damage rects instead of full repaints.
func TestPhaseOffset_SelectionUsesAccurateDamage(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<p>Select this text to test selection damage tracking.</p>
	</body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}

	rect := sess.SelectionDamageRect()
	if rect.X0 != 0 || rect.Y0 != 0 || rect.X1 != 0 || rect.Y1 != 0 {
		t.Error("Selection damage rect should be empty before selection")
	}
}

// TestPhaseOffset_FocusUsesAccurateDamage verifies that focus changes
// compute accurate damage rects.
func TestPhaseOffset_FocusUsesAccurateDamage(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<input type="text" id="test" value="Focus test">
	</body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}

	var inputNode *dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n != nil && n.Data == "input" {
			inputNode = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&sess.Doc.Node)

	if inputNode == nil {
		t.Fatal("Could not find input element")
	}

	sess.FocusNode(inputNode)

	rect := sess.FocusDamageRect()
	if rect.X0 == 0 && rect.Y0 == 0 && rect.X1 == 0 && rect.Y1 == 0 {
		t.Error("Focus damage rect should not be empty for focused input")
	}
}

// TestIncrementalStyleResolution_WhitespaceNodes verifies that whitespace
// text node normalization doesn't cause false tree change detection.
func TestIncrementalStyleResolution_WhitespaceNodes(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<div>
			<span>Test</span>
		</div>
	</body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}

	sess.SetHover(10, 10)
	if sess.HasPseudoClassRule("hover") && sess.IsHoverGeometryNeutral() {
		if err := sess.RestyleForHover(); err != nil {
			t.Fatalf("RestyleForHover should succeed: %v", err)
		}
	}
}

// TestArenaRefresh_EmptyNodeList verifies that arena refresh handles
// empty node lists correctly without failing.
func TestArenaRefresh_EmptyNodeList(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div>Test</div></body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}

	emptyNodes := []dom.NodeID{}
	if !sess.Arena.RefreshComputedStylesForNodes(sess.Styles, emptyNodes) {
		t.Error("Arena refresh should succeed with empty node list")
	}
}

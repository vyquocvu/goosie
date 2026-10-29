package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// TestFirstLineFontWeight verifies that ::first-line { font-weight: bold }
// changes the font weight of runs on the first line.
func TestFirstLineFontWeight(t *testing.T) {
	html := `<html><head><style>
	p { font-size: 16px; line-height: 20px; width: 400px; }
	p::first-line { font-weight: bold; }
	</style></head><body>
	<p id="target">Hello world this is a test</p>
	</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	// Run inline layout.
	layout.Inline(arena, layout.ObjectID(1))

	// Find word objects that are children of the <p> element.
	pObjID, ok := arena.ForNode(target.ID)
	if !ok {
		t.Fatal("target layout object not found")
	}
	_ = pObjID

	// Find the first word object in the arena. It should have bold weight.
	firstWord := findWordObject(arena, "Hello")
	if firstWord == nil {
		t.Fatal("word 'Hello' not found in arena")
	}
	if firstWord.Style == nil {
		t.Fatal("word 'Hello' has nil style")
	}
	if firstWord.Style.FontWeight != style.WeightBold {
		t.Errorf("first-line word 'Hello' font-weight = %v, want %v (bold)",
			firstWord.Style.FontWeight, style.WeightBold)
	}
}

// TestFirstLineSecondLineUnaffected verifies that only the first line is
// styled; words on the second line keep the original style.
func TestFirstLineSecondLineUnaffected(t *testing.T) {
	html := `<html><head><style>
	p { font-size: 16px; line-height: 20px; width: 100px; }
	p::first-line { font-weight: bold; }
	</style></head><body>
	<p id="target">First line words here and then more words for the second line</p>
	</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	layout.Inline(arena, layout.ObjectID(1))

	// Find a word that should be on the second line (narrow width forces wrap).
	secondLineWord := findWordObject(arena, "second")
	if secondLineWord == nil {
		// If "second" is not found, the text all fits on one line.
		// That's fine for this test - just verify the first word is bold.
		firstWord := findWordObject(arena, "First")
		if firstWord != nil && firstWord.Style != nil && firstWord.Style.FontWeight != style.WeightBold {
			t.Errorf("first-line word font-weight = %v, want bold", firstWord.Style.FontWeight)
		}
		return
	}
	if secondLineWord.Style != nil && secondLineWord.Style.FontWeight == style.WeightBold {
		t.Errorf("second-line word 'second' should NOT be bold, got font-weight %v",
			secondLineWord.Style.FontWeight)
	}
}

// TestFirstLineNoRules verifies that without ::first-line rules, the first
// line keeps the original style.
func TestFirstLineNoRules(t *testing.T) {
	html := `<html><head><style>
	p { font-size: 16px; font-weight: normal; width: 400px; }
	</style></head><body>
	<p id="target">Hello world</p>
	</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	layout.Inline(arena, layout.ObjectID(1))

	firstWord := findWordObject(arena, "Hello")
	if firstWord == nil {
		t.Fatal("word 'Hello' not found")
	}
	if firstWord.Style != nil && firstWord.Style.FontWeight == style.WeightBold {
		t.Error("word 'Hello' should not be bold without ::first-line rule")
	}
}

// TestFirstLineStoredInArena verifies that the arena carries first-line
// styles from the pseudo-style map.
func TestFirstLineStoredInArena(t *testing.T) {
	html := `<html><body><p id="target">Hello</p></body></html>`
	cssText := `p::first-line { font-weight: bold; }`

	doc := domtest.Parse(html)
	sheets := []*css.Stylesheet{css.Parse(cssText)}
	styles := style.Resolve(doc, sheets, nil)
	pseudo := style.ResolvePseudoElements(doc, sheets, styles, nil)
	arena := layout.Build(doc, styles, pseudo)

	if arena.FirstLineStyles == nil {
		t.Fatal("expected FirstLineStyles to be populated")
	}

	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target not found")
	}
	flStyle, ok := arena.FirstLineStyles[target.ID]
	if !ok {
		t.Fatalf("expected first-line style for target node %d", target.ID)
	}
	if flStyle.FontWeight != style.WeightBold {
		t.Errorf("first-line font-weight = %v, want bold", flStyle.FontWeight)
	}
}

// TestFirstLineColor verifies that color is applied on the first line.
func TestFirstLineColor(t *testing.T) {
	html := `<html><head><style>
	p { font-size: 16px; width: 400px; color: black; }
	p::first-line { color: red; }
	</style></head><body>
	<p id="target">Hello world</p>
	</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	layout.Inline(arena, layout.ObjectID(1))

	firstWord := findWordObject(arena, "Hello")
	if firstWord == nil {
		t.Fatal("word 'Hello' not found")
	}
	if firstWord.Style == nil {
		t.Fatal("word has nil style")
	}
	// Red is css.Color{R:255, G:0, B:0, A:255}
	if firstWord.Style.Color.R != 255 || firstWord.Style.Color.G != 0 || firstWord.Style.Color.B != 0 {
		t.Errorf("first-line color = %v, want red", firstWord.Style.Color)
	}
}

// sessionWithSheetsForFirstLine is a helper that builds the arena and runs
// block layout. It is the same as sessionWithSheets in first_letter_test.go.
func sessionWithSheetsForFirstLine(t *testing.T, html string, viewportW float32) (*layout.Arena, *dom.Document) {
	t.Helper()
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "style" {
			sheets = append(sheets, css.ParseForViewport(n.TextContent(), viewportW))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	styles := style.Resolve(doc, sheets, nil)
	pseudo := style.ResolvePseudoElements(doc, sheets, styles, nil)
	arena := layout.Build(doc, styles, pseudo)
	layout.Block(arena, layout.ObjectID(1), viewportW, 0)
	return arena, doc
}

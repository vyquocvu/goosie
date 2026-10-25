package layout_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func sessionWithSheets(t *testing.T, html string, viewportW float32) (*layout.Arena, *dom.Document) {
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

// qc:BR-075
func TestFirstLetterSplitBox(t *testing.T) {
	html := `<html><head><style>
p::first-letter { font-size: 48px; }
</style></head><body>
<p id="target">Hello</p>
</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	pObjID, ok := arena.ForNode(target.ID)
	if !ok {
		t.Fatal("target layout object not found")
	}

	pObj := arena.Get(pObjID)
	var textContents []string
	var firstLetterObj *layout.Object
	var remainderObj *layout.Object

	for kid := pObj.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
		k := arena.Get(kid)
		if k.Node != nil && k.Node.Text() {
			textContents = append(textContents, k.Node.DataContent)
			if k.Node.DataContent == "H" {
				firstLetterObj = k
			} else {
				remainderObj = k
			}
		}
	}

	if firstLetterObj == nil {
		t.Fatalf("first-letter box containing 'H' not found; children texts: %v", textContents)
	}
	if remainderObj == nil || remainderObj.Node.DataContent != "ello" {
		t.Fatalf("remainder box containing 'ello' not found; children texts: %v", textContents)
	}

	totalGlyphs := 0
	for _, tc := range textContents {
		totalGlyphs += len(tc)
	}
	if totalGlyphs != 5 {
		t.Fatalf("total source glyphs = %d, want 5 (text must not be duplicated); texts: %v", totalGlyphs, textContents)
	}
	if len(textContents) != 2 {
		t.Fatalf("expected exactly 2 text boxes, got %d: %v", len(textContents), textContents)
	}
}

// qc:BR-075
func TestFirstLetterLeadingWhitespace(t *testing.T) {
	html := `<html><head><style>
p::first-letter { font-size: 48px; }
</style></head><body>
<p id="target">   Hello</p>
</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	pObjID, ok := arena.ForNode(target.ID)
	if !ok {
		t.Fatal("target layout object not found")
	}

	pObj := arena.Get(pObjID)
	var firstLetterObj *layout.Object
	for kid := pObj.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
		k := arena.Get(kid)
		if k.Node != nil && k.Node.Text() {
			if strings.TrimSpace(k.Node.DataContent) != "" {
				firstLetterObj = k
				break
			}
		}
	}

	if firstLetterObj == nil {
		t.Fatal("first-letter box not found")
	}
	if firstLetterObj.Node.DataContent != "H" {
		t.Fatalf("first-letter extracted %q, want 'H' (leading whitespace must be skipped)", firstLetterObj.Node.DataContent)
	}
}

// qc:BR-075
func TestFirstLetterFloatLeft(t *testing.T) {
	html := `<html><head><style>
p { width: 300px; font-size: 16px; line-height: 20px; margin: 0; }
p::first-letter { font-size: 48px; float: left; margin-right: 15px; }
</style></head><body>
<p id="target">This is a long paragraph that should wrap around the drop cap first letter. The first letter is floated to the left with a large font size and margin. The lines beside the float must be indented to the right of the float box, and subsequent lines below the float should return to the left margin.</p>
</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	pObjID, ok := arena.ForNode(target.ID)
	if !ok {
		t.Fatal("target layout object not found")
	}
	pObj := arena.Get(pObjID)

	var floatObj *layout.Object
	for kid := pObj.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
		k := arena.Get(kid)
		if k.Style != nil && k.Style.Float == style.FloatLeft {
			floatObj = k
			break
		}
	}

	if floatObj == nil {
		t.Fatal("first-letter float box not found")
	}
	if floatObj.W <= 0 || floatObj.H <= 0 {
		t.Fatalf("first-letter float box has empty dimensions: W=%v H=%v", floatObj.W, floatObj.H)
	}

	// Find the first word object in the arena following the drop cap
	firstWord := findWordObject(arena, "This")
	if firstWord == nil {
		firstWord = findWordObject(arena, "his")
	}
	if firstWord == nil {
		t.Fatal("word 'his' or 'This' not found")
	}

	minX := floatObj.X + floatObj.W + 15
	if firstWord.X < minX-1 {
		t.Fatalf("first line word X = %v, want >= %v (must be to the right of the float and its margin)", firstWord.X, minX-1)
	}
}

// qc:BR-075
func TestFirstLetterEmptyContent(t *testing.T) {
	html := `<html><head><style>
p::first-letter { content: ""; font-size: 48px; }
</style></head><body>
<p id="target">Hello</p>
</body></html>`

	arena, doc := sessionWithSheets(t, html, 800)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}

	pObjID, ok := arena.ForNode(target.ID)
	if !ok {
		t.Fatal("target layout object not found")
	}
	pObj := arena.Get(pObjID)

	count := 0
	for kid := pObj.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
		k := arena.Get(kid)
		if k.Node != nil && k.Node.Text() {
			count++
			if k.Node.DataContent != "Hello" {
				t.Fatalf("unexpected text %q, want 'Hello'", k.Node.DataContent)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 text box for content: \"\", got %d", count)
	}
}

// qc:BR-075
func TestFirstLetterEmptyAndElementOnly(t *testing.T) {
	cases := []string{
		`<p id="target"></p>`,
		`<p id="target">   </p>`,
		`<p id="target"><span><b></b></span></p>`,
	}

	for _, c := range cases {
		html := `<html><head><style>
p::first-letter { font-size: 48px; }
</style></head><body>` + c + `</body></html>`

		arena, doc := sessionWithSheets(t, html, 800)
		target := doc.ElementByID("target")
		if target == nil {
			t.Fatal("target not found")
		}
		pObjID, ok := arena.ForNode(target.ID)
		if !ok {
			t.Fatal("target layout object not found")
		}
		pObj := arena.Get(pObjID)
		for kid := pObj.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
			k := arena.Get(kid)
			if k.Style != nil && k.Style.FontSize == 48 {
				t.Fatalf("expected no first-letter box for empty/element-only case: %s", c)
			}
		}
	}
}

// Untagged on purpose: it proves a rebuild happened, never what the rebuilt ::first-letter was - GOOSIE-PSEUDO-002.
func TestFirstLetterIncremental(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)
	sess, err := engine.NewSession(`<html><head><style>
		#target { font-size: 16px; }
		#target::first-letter { font-size: 32px; color: black; }
		#target:hover::first-letter { font-size: 64px; }
		#unrelated { background-color: black; }
		#unrelated:hover { background-color: red; }
	</style></head><body>
		<p id="target">Hello</p>
		<div id="unrelated">other</div>
	</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)

	unrelated := sess.Doc.ElementByID("unrelated")
	target := sess.Doc.ElementByID("target")
	if unrelated == nil || target == nil {
		t.Fatal("elements not found")
	}

	// 1. Paint-only change on unrelated element: arena reused, no layout rebuild (BR-185 invariant)
	before := sess.WorkCounts()
	for current := unrelated; current != nil && current.Element(); current = current.Parent {
		css.SetHover(current, true)
	}
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	after := sess.WorkCounts()
	if after.LayoutBuilds != before.LayoutBuilds {
		t.Fatalf("paint-only hover rebuilt layout: %d -> %d", before.LayoutBuilds, after.LayoutBuilds)
	}

	// 2. Hover on target modifying ::first-letter: layout rebuild must occur via style_diff
	css.ClearInteractiveState()
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	before = sess.WorkCounts()
	for current := target; current != nil && current.Element(); current = current.Parent {
		css.SetHover(current, true)
	}
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	if sess.WorkCounts().LayoutBuilds != before.LayoutBuilds+1 {
		t.Fatalf("first-letter style change did not rebuild layout: work %+v, before %+v", sess.WorkCounts(), before)
	}
}

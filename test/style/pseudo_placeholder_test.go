package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func findByID(doc *dom.Document, id string) *dom.Node {
	var out *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil || out != nil {
			return
		}
		if n.Element() && n.GetAttribute("id") == id {
			out = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	return out
}

// qc:BR-321
func TestPlaceholderPseudoElementCascade(t *testing.T) {
	html := `<html><body>
		<input id="a1" placeholder="AAA">
		<input id="a2" placeholder="RED">
		<input id="a3" class="red" placeholder="SPECIFIC">
		<textarea id="b1" placeholder="TA"></textarea>
		<input id="d1" placeholder="BG">
		<input id="e1" placeholder="OP">
		<input id="f" placeholder="FO">
		<input id="g" placeholder="GO">
		<input id="h" placeholder="HO">
	</body></html>`

	cssText := `
		#g { color: rgb(0, 0, 255); }
		#h { color: rgb(200, 0, 200); }
		#a2::placeholder { color: rgb(200, 0, 0); }
		#a3.red::placeholder { color: rgb(0, 100, 0); }
		.red::placeholder { color: rgb(0, 0, 100); }
		#b1::placeholder { color: rgb(123, 45, 67); }
		#d1::placeholder { background-color: rgb(255, 255, 0); color: rgb(1, 2, 3); }
		#e1::placeholder { opacity: 0.25; color: rgb(250, 250, 250); }
		#f::placeholder { opacity: 0.3; }
		#g::placeholder { text-decoration: underline; }
		#h::placeholder { color: inherit; }
	`

	doc := domtest.Parse(html)
	sheets := []*css.Stylesheet{css.Parse(cssText)}
	styles := style.Resolve(doc, sheets, nil)
	pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)

	type want struct {
		r, g, b uint8
		opacity float32
	}
	cases := map[string]want{
		"a2": {200, 0, 0, 1},
		"a3": {0, 100, 0, 1},
		"b1": {123, 45, 67, 1},
		"d1": {1, 2, 3, 1},
		"e1": {250, 250, 250, 0.25},
		"f":  {117, 117, 117, 0.3},
		"g":  {117, 117, 117, 1},
		"h":  {200, 0, 200, 1},
	}
	for id, w := range cases {
		n := findByID(doc, id)
		if n == nil {
			t.Fatalf("node %s not found", id)
		}
		cs, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "placeholder"}]
		if !ok {
			t.Errorf("%s: expected ::placeholder style, missing", id)
			continue
		}
		if cs.Color.R != w.r || cs.Color.G != w.g || cs.Color.B != w.b {
			t.Errorf("%s: ::placeholder color = rgb(%d,%d,%d), want rgb(%d,%d,%d)", id, cs.Color.R, cs.Color.G, cs.Color.B, w.r, w.g, w.b)
		}
		if cs.Opacity < w.opacity-0.001 || cs.Opacity > w.opacity+0.001 {
			t.Errorf("%s: ::placeholder opacity = %v, want %v", id, cs.Opacity, w.opacity)
		}
	}

	n1 := findByID(doc, "a1")
	if _, ok := pseudoStyles[style.PseudoKey{NodeID: n1.ID, Pseudo: "placeholder"}]; ok {
		t.Errorf("a1: expected NO ::placeholder style for an element with no author rule (paint falls back to UA rgb(117,117,117))")
	}
}

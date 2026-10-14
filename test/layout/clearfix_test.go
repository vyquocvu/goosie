package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func findByID(arena *layout.Arena, id string) *layout.Object {
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Element() && obj.Node.GetAttribute("id") == id {
			return obj
		}
	}
	return nil
}

// sessionSheets resolves with the document's own <style> blocks (the plain
// session helper ignores them, so CSS-dependent tests must collect sheets).
func sessionSheets(t *testing.T, html string, viewportW float32) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	var walk func(n *dom.Node)
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
	return arena
}

// The float-containment clearfix (.row:after{content:"";display:table;
// clear:both}) is how float grids keep their height. content:"" is generated
// content per CSS (an empty box), not no content: without the clearing box
// the row collapses and everything after slides up over the floats.
func TestClearfixContainsFloats(t *testing.T) {
	arena := sessionSheets(t, `<html><head><style>
.w3-row:after{content:"";display:table;clear:both}
.w3-col{float:left;width:50%}
</style></head><body>
<div class="w3-row" id="row"><div class="w3-col" style="height:100px">a</div><div class="w3-col" style="height:100px">b</div></div>
<div id="after">x</div>
</body></html>`, 800)
	row := findByID(arena, "row")
	if row == nil {
		t.Fatal("row not found")
	}
	if row.H < 100 {
		t.Errorf("row H = %v, want >= 100 (clearfix must contain the floats)", row.H)
	}
	after := findByID(arena, "after")
	if after == nil {
		t.Fatal("after not found")
	}
	if after.Y < 100 {
		t.Errorf("after Y = %v, want >= 100 (below the contained floats)", after.Y)
	}
}

func TestClearfixEmptyStringGeneratesBox(t *testing.T) {
	arena := sessionSheets(t, `<html><head><style>
.box:after{content:"";display:block;clear:both}
</style></head><body>
<div class="box" id="box"><div style="float:left;height:40px">a</div></div>
</body></html>`, 800)
	box := findByID(arena, "box")
	if box == nil {
		t.Fatal("box not found")
	}
	if box.H < 40 {
		t.Errorf("box H = %v, want >= 40", box.H)
	}
}

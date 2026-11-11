package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// buildBijectionArena lays a fragment out through the full pipeline (Build,
// Block, Inline, Positioning) so generated word and anonymous boxes exist. The
// css argument is injected as a <style> sheet; pass "" for none.
func buildBijectionArena(t *testing.T, html, cssText string) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(`<html><head><style>` + cssText + `</style></head><body>` + html + `</body></html>`)
	var sheets []*css.Stylesheet
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "style" {
			sheets = append(sheets, css.ParseForViewport(n.TextContent(), 800))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	styles := style.Resolve(doc, sheets, nil)
	pseudo := style.ResolvePseudoElements(doc, sheets, styles, nil)
	arena := layout.Build(doc, styles, pseudo)
	layout.Block(arena, layout.ObjectID(1), 800, 600)
	layout.Inline(arena, layout.ObjectID(1))
	layout.Positioning(arena, layout.ObjectID(1), 800, 600)
	return arena
}

// keyedBoxes walks the arena and returns every object that carries a non-zero Key.
func keyedBoxes(a *layout.Arena) []layout.ObjectID {
	var ids []layout.ObjectID
	for i := 1; i < len(a.Objects); i++ {
		if a.Objects[i].Key != 0 {
			ids = append(ids, layout.ObjectID(i))
		}
	}
	return ids
}

func TestBoxKeyBijectionElementsAndPseudo(t *testing.T) {
	a := buildBijectionArena(t, `<div id="x"><p>hello</p></div>`, `#x::before{content:"B"}`)
	ids := keyedBoxes(a)
	if len(ids) == 0 {
		t.Fatal("no keyed boxes")
	}
	seen := map[layout.BoxKey]layout.ObjectID{}
	for _, id := range ids {
		k := a.Get(id).Key
		if prev, dup := seen[k]; dup {
			t.Fatalf("key %v shared by objects %d and %d", k, prev, id)
		}
		seen[k] = id
		got, ok := a.ForKey(k)
		if !ok || got != id {
			t.Fatalf("ForKey(%v) = (%d,%v), want (%d,true)", k, got, ok, id)
		}
	}
}

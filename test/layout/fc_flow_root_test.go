package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// arenaObjectByID walks the arena for the object whose element carries id.
func arenaObjectByID(a *layout.Arena, id string) (layout.ObjectID, bool) {
	for i := 1; i < len(a.Objects); i++ {
		n := a.Objects[i].Node
		if n != nil && n.Element() && n.GetAttribute("id") == id {
			return layout.ObjectID(i), true
		}
	}
	return 0, false
}

// A flow-root block contains its float: its height grows to the float bottom.
func TestFlowRootContainsFloat(t *testing.T) {
	a := buildBijectionArena(t,
		`<div id="fr" style="display:flow-root;width:300px"><div style="float:left;width:100px;height:80px"></div>text</div>`,
		"")
	id, ok := arenaObjectByID(a, "fr")
	if !ok {
		t.Fatal("no layout object for #fr")
	}
	_, y0, _, y1 := a.Get(id).BorderRect()
	if h := y1 - y0; h < 80 {
		t.Errorf("flow-root height = %v, want >= 80 (must contain its float)", h)
	}
}

// A plain block does NOT contain its float (control).
func TestPlainBlockDoesNotContainFloat(t *testing.T) {
	a := buildBijectionArena(t,
		`<div id="pl" style="width:300px"><div style="float:left;width:100px;height:80px"></div>text</div>`,
		"")
	id, ok := arenaObjectByID(a, "pl")
	if !ok {
		t.Fatal("no layout object for #pl")
	}
	_, y0, _, y1 := a.Get(id).BorderRect()
	if h := y1 - y0; h >= 80 {
		t.Errorf("plain block height = %v, want < 80 (float must escape)", h)
	}
}

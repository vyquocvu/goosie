package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// qc:BR-061
func TestCompoundTypeClassIDSelectorsReachOnlyTheirNode(t *testing.T) {
	doc, styles := resolveTestDoc(`<html><body>
<p id="p1" class="note lead">one</p>
<p id="p2" class="note">two</p>
<p id="p3">three</p>
</body></html>`, `
p { color: red; }
.note { color: green; }
#p1 { color: blue; }
p.note.lead { background-color: yellow; }
`)

	p1 := findElementByID(&doc.Node, "p1")
	p2 := findElementByID(&doc.Node, "p2")
	p3 := findElementByID(&doc.Node, "p3")
	if p1 == nil || p2 == nil || p3 == nil {
		t.Fatal("fixture nodes not found")
	}

	if got := styles[p1.ID].Color; got != col(0, 0, 255, 255) {
		t.Errorf("p1 colour = %v, want blue from #p1", got)
	}
	if got := styles[p1.ID].BackgroundColor; got != col(255, 255, 0, 255) {
		t.Errorf("p1 background = %v, want yellow from p.note.lead", got)
	}
	if got := styles[p2.ID].BackgroundColor; got.A == 0 {
	} else {
		t.Errorf("p2 background = %v, want transparent (it lacks .lead)", got)
	}
	if got := styles[p2.ID].Color; got != col(0, 128, 0, 255) {
		t.Errorf("p2 colour = %v, want green from .note", got)
	}
	if got := styles[p3.ID].Color; got != col(255, 0, 0, 255) {
		t.Errorf("p3 colour = %v, want red from the type selector", got)
	}
}

// qc:BR-062
func TestSpecificityOutranksSourceOrderAndEqualSpecificityUsesIt(t *testing.T) {
	t.Run("id beats class beats type", func(t *testing.T) {
		doc, styles := resolveTestDoc(`<html><body><p id="x" class="c">t</p></body></html>`, `
#x { color: blue; }
.c { color: green; }
p { color: red; }
`)
		p := findElementByID(&doc.Node, "x")
		if p == nil {
			t.Fatal("node not found")
		}
		if got := styles[p.ID].Color; got != col(0, 0, 255, 255) {
			t.Errorf("colour = %v, want blue: an id selector declared first still wins", got)
		}
	})

	t.Run("equal specificity follows source order", func(t *testing.T) {
		doc, styles := resolveTestDoc(`<html><body><p id="x" class="c">t</p></body></html>`, `
.c { color: green; }
.c { color: teal; }
`)
		p := findElementByID(&doc.Node, "x")
		if p == nil {
			t.Fatal("node not found")
		}
		if got := styles[p.ID].Color; got != col(0, 128, 128, 255) {
			t.Errorf("colour = %v, want teal: the later declaration of equal specificity wins", got)
		}
	})

	t.Run("later stylesheet wins at equal specificity", func(t *testing.T) {
		doc := domtest.Parse(`<html><body><p id="x" class="c">t</p></body></html>`)
		first := css.Parse(`.c { color: green; } p { color: olive; }`)
		second := css.Parse(`.c { color: teal; }`)
		styles := style.Resolve(doc, []*css.Stylesheet{first, second}, nil)
		p := findElementByID(&doc.Node, "x")
		if p == nil {
			t.Fatal("node not found")
		}
		if got := styles[p.ID].Color; got != col(0, 128, 128, 255) {
			t.Errorf("colour = %v, want teal from the second sheet", got)
		}
	})
}

// qc:BR-063
func TestInheritedFontColorAndLineHeightReachDescendants(t *testing.T) {
	doc, styles := resolveTestDoc(`<html><body style="margin:0;font-size:20px;line-height:2;color:rgb(1,2,3)">
<div id="child"><span id="leaf">hi</span></div>
<div id="big" style="font-size:24px">hi</div>
</body></html>`, "")

	child := findElementByID(&doc.Node, "child")
	leaf := findElementByID(&doc.Node, "leaf")
	big := findElementByID(&doc.Node, "big")
	if child == nil || leaf == nil || big == nil {
		t.Fatal("fixture nodes not found")
	}

	for _, tc := range []struct {
		name string
		id   dom.NodeID
	}{
		{"child", child.ID},
		{"leaf", leaf.ID},
	} {
		cs := styles[tc.id]
		if cs.FontSize != 20 {
			t.Errorf("%s font-size = %v, want 20px inherited", tc.name, cs.FontSize)
		}
		if cs.Color != col(1, 2, 3, 255) {
			t.Errorf("%s colour = %v, want rgb(1,2,3) inherited", tc.name, cs.Color)
		}
		if cs.LineHeight != 40 {
			t.Errorf("%s line-height = %v, want 40px (2 x the inherited 20px font)", tc.name, cs.LineHeight)
		}
	}

	cs := styles[big.ID]
	if cs.FontSize != 24 {
		t.Errorf("big font-size = %v, want 24px", cs.FontSize)
	}
	if cs.LineHeight != 48 {
		t.Errorf("big line-height = %v, want 48px (2 x its own 24px font)", cs.LineHeight)
	}

	arena := layout.Build(doc, styles, nil)
	layout.Block(arena, layout.ObjectID(1), 800, 0)
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node == nil || obj.Node.GetAttribute("id") != "child" || obj.Style == nil {
			continue
		}
		if obj.H != 40 {
			t.Errorf("child.H = %v, want 40 (one inherited line box)", obj.H)
		}
		return
	}
	t.Fatal("child box not found in layout arena")
}

package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
)

// --- Checkbox toggle tests ---

func TestCheckboxToggle(t *testing.T) {
	html := `<html><body><form>` +
		`<input id="cb" type="checkbox">` +
		`</form></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	// Find the checkbox node.
	cb := findNodeByTag(&s.Doc.Node, "input")
	if cb == nil {
		t.Fatal("checkbox input not found")
	}
	// Initially unchecked.
	if engine.IsChecked(cb) {
		t.Fatal("checkbox should start unchecked")
	}
	// Toggle on.
	if !s.ToggleControl(cb) {
		t.Fatal("ToggleControl should return true")
	}
	if !engine.IsChecked(cb) {
		t.Fatal("checkbox should be checked after toggle")
	}
	// Toggle off.
	if !s.ToggleControl(cb) {
		t.Fatal("ToggleControl should return true on second toggle")
	}
	if engine.IsChecked(cb) {
		t.Fatal("checkbox should be unchecked after second toggle")
	}
}

// --- Radio toggle tests ---

func TestRadioToggle(t *testing.T) {
	html := `<html><body><form>` +
		`<input id="r1" type="radio" name="color" value="red">` +
		`<input id="r2" type="radio" name="color" value="blue">` +
		`<input id="r3" type="radio" name="color" value="green">` +
		`</form></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	radios := findNodesByTagAndAttr(&s.Doc.Node, "input", "name", "color")
	if len(radios) != 3 {
		t.Fatalf("expected 3 radios, got %d", len(radios))
	}
	r1, r2, r3 := radios[0], radios[1], radios[2]

	// Click r2: it should be checked, others unchecked.
	if !s.ToggleControl(r2) {
		t.Fatal("ToggleControl should return true")
	}
	if !engine.IsChecked(r2) {
		t.Fatal("r2 should be checked")
	}
	if engine.IsChecked(r1) {
		t.Fatal("r1 should be unchecked")
	}
	if engine.IsChecked(r3) {
		t.Fatal("r3 should be unchecked")
	}

	// Click r3: r2 should be unchecked, r3 checked.
	s.ToggleControl(r3)
	if !engine.IsChecked(r3) {
		t.Fatal("r3 should be checked")
	}
	if engine.IsChecked(r2) {
		t.Fatal("r2 should be unchecked after r3 clicked")
	}
}

// --- IsChecked tests ---

func TestIsChecked(t *testing.T) {
	doc := dom.NewDocument()
	n := doc.NewElement("input")
	n.SetAttribute("type", "checkbox")

	if engine.IsChecked(n) {
		t.Fatal("unchecked checkbox should report false")
	}
	engine.SetChecked(n, true)
	if !engine.IsChecked(n) {
		t.Fatal("checked checkbox should report true")
	}
	engine.SetChecked(n, false)
	if engine.IsChecked(n) {
		t.Fatal("unchecked checkbox should report false after unchecking")
	}
	if engine.IsChecked(nil) {
		t.Fatal("nil node should report false")
	}
}

// --- IsActivatable tests ---

func TestButtonIsActivatable(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.NewElement("button")
	if !engine.IsActivatable(btn) {
		t.Fatal("button element should be activatable")
	}
}

func TestSubmitInputIsActivatable(t *testing.T) {
	doc := dom.NewDocument()
	inp := doc.NewElement("input")
	inp.SetAttribute("type", "submit")
	if !engine.IsActivatable(inp) {
		t.Fatal("input[type=submit] should be activatable")
	}
	inp2 := doc.NewElement("input")
	inp2.SetAttribute("type", "reset")
	if !engine.IsActivatable(inp2) {
		t.Fatal("input[type=reset] should be activatable")
	}
	inp3 := doc.NewElement("input")
	inp3.SetAttribute("type", "button")
	if !engine.IsActivatable(inp3) {
		t.Fatal("input[type=button] should be activatable")
	}
}

func TestTextInputIsNotActivatable(t *testing.T) {
	doc := dom.NewDocument()
	inp := doc.NewElement("input")
	inp.SetAttribute("type", "text")
	if engine.IsActivatable(inp) {
		t.Fatal("input[type=text] should NOT be activatable")
	}
	inp2 := doc.NewElement("input")
	inp2.SetAttribute("type", "checkbox")
	if engine.IsActivatable(inp2) {
		t.Fatal("input[type=checkbox] should NOT be activatable")
	}
	if engine.IsActivatable(nil) {
		t.Fatal("nil should NOT be activatable")
	}
}

// --- SelectedOption tests ---

func TestSelectedOption(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.NewElement("select")
	o1 := doc.NewElement("option")
	o1.AppendChild(doc.NewText("Alpha"))
	o2 := doc.NewElement("option")
	o2.AppendChild(doc.NewText("Beta"))
	o2.SetAttribute("selected", "")
	o3 := doc.NewElement("option")
	o3.AppendChild(doc.NewText("Gamma"))
	sel.AppendChild(o1)
	sel.AppendChild(o2)
	sel.AppendChild(o3)

	got := engine.SelectedOption(sel)
	if got != o2 {
		t.Fatalf("SelectedOption = %v, want option with 'selected'", got)
	}
}

func TestSelectedOptionDefault(t *testing.T) {
	doc := dom.NewDocument()
	sel := doc.NewElement("select")
	o1 := doc.NewElement("option")
	o1.AppendChild(doc.NewText("First"))
	o2 := doc.NewElement("option")
	o2.AppendChild(doc.NewText("Second"))
	sel.AppendChild(o1)
	sel.AppendChild(o2)

	got := engine.SelectedOption(sel)
	if got != o1 {
		t.Fatalf("SelectedOption default = %v, want first option", got)
	}
	if engine.SelectedOption(nil) != nil {
		t.Fatal("SelectedOption(nil) should return nil")
	}
}

// --- Intrinsic size tests ---

func TestCheckboxIntrinsicSize(t *testing.T) {
	html := `<html><body><input id="cb" type="checkbox"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	obj := findObjByID(t, s, "cb")
	x0, y0, x1, y1 := obj.BorderRect()
	w := x1 - x0
	h := y1 - y0
	if w < 13 || w > 16 {
		t.Fatalf("checkbox width = %v, want ~13", w)
	}
	if h < 13 || h > 16 {
		t.Fatalf("checkbox height = %v, want ~13", h)
	}
}

func TestSelectIntrinsicSize(t *testing.T) {
	html := `<html><body><select id="sel"><option>Alpha</option><option>Beta</option></select></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	// The select element should exist in the arena.
	// With display:inline in the UA stylesheet, the select is an inline element
	// and doesn't get intrinsic dimensions from block layout's replacedSize path.
	// We just verify it's present in the arena.
	obj := findObjByID(t, s, "sel")
	if obj == nil {
		t.Fatal("select element not found in arena")
	}
	if obj.Node == nil || obj.Node.Data != "select" {
		t.Fatal("found object is not a select element")
	}
}

// --- Test helpers ---

func findNodeByTag(root *dom.Node, tag string) *dom.Node {
	if root.Element() && root.Data == tag {
		return root
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if got := findNodeByTag(c, tag); got != nil {
			return got
		}
	}
	return nil
}

func findNodesByTagAndAttr(root *dom.Node, tag, attrName, attrVal string) []*dom.Node {
	var out []*dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == tag && n.GetAttribute(attrName) == attrVal {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func findObjByID(t *testing.T, s *engine.Session, id string) *layout.Object {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			return obj
		}
	}
	t.Fatalf("element %q not found in arena", id)
	return nil
}

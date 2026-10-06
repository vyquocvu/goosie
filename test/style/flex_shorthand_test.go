package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// flexOf resolves one styled div and returns its flex triple.
func flexOf(t *testing.T, declared string) (grow, shrink, basis float32) {
	t.Helper()
	doc := domtest.Parse(`<html><body><div style="flex: ` + declared + `;">x</div></body></html>`)
	styles := style.Resolve(doc, nil, nil)
	var div *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n.FirstChild; c != nil && div == nil; c = c.NextSibling {
			if c.Element() && c.Data == "div" {
				div = c
				return
			}
			walk(c)
		}
	}
	walk(&doc.Node)
	if div == nil {
		t.Fatal("div not parsed")
	}
	cs := styles[div.ID]
	return cs.FlexGrow, cs.FlexShrink, cs.FlexBasis
}

// TestFlexShorthandTwoPartBasis pins `flex: <grow> <basis>`: a length second
// half is the basis with shrink defaulting to 1, not the shrink. Reading
// `flex: 0 10px` as shrink 10 collapsed two-part basis items to content size
// in every flex direction.
func TestFlexShorthandTwoPartBasis(t *testing.T) {
	grow, shrink, basis := flexOf(t, "0 10px")
	if grow != 0 || shrink != 1 || basis != 10 {
		t.Errorf("flex: 0 10px = (%v, %v, %v), want (0, 1, 10)", grow, shrink, basis)
	}
	grow, shrink, basis = flexOf(t, "0 30%")
	if grow != 0 || shrink != 1 || basis != -32 {
		t.Errorf("flex: 0 30%% = (%v, %v, %v), want (0, 1, -32%% sentinel)", grow, shrink, basis)
	}
	// Two bare numbers stay grow+shrink.
	grow, shrink, basis = flexOf(t, "2 3")
	if grow != 2 || shrink != 3 || basis != 0 {
		t.Errorf("flex: 2 3 = (%v, %v, %v), want (2, 3, 0)", grow, shrink, basis)
	}
	// A lone length is the basis, not the grow.
	grow, shrink, basis = flexOf(t, "10px")
	if grow != 1 || shrink != 1 || basis != 10 {
		t.Errorf("flex: 10px = (%v, %v, %v), want (1, 1, 10)", grow, shrink, basis)
	}
}

// TestFlexShorthandInvalidUnitlessBasis pins declaration validity: a
// unitless nonzero basis makes the whole shorthand drop, so the previous
// value stands instead of a 4px basis appearing from nowhere.
func TestFlexShorthandInvalidUnitlessBasis(t *testing.T) {
	doc := domtest.Parse(`<html><body><div style="flex: 1 1 200px; flex: 0 0 4;">x</div></body></html>`)
	styles := style.Resolve(doc, nil, nil)
	var div *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n.FirstChild; c != nil && div == nil; c = c.NextSibling {
			if c.Element() && c.Data == "div" {
				div = c
				return
			}
			walk(c)
		}
	}
	walk(&doc.Node)
	if div == nil {
		t.Fatal("div not parsed")
	}
	cs := styles[div.ID]
	if cs.FlexGrow != 1 || cs.FlexShrink != 1 || cs.FlexBasis != 200 {
		t.Errorf("after invalid flex: 0 0 4 = (%v, %v, %v), want previous (1, 1, 200)",
			cs.FlexGrow, cs.FlexShrink, cs.FlexBasis)
	}
}

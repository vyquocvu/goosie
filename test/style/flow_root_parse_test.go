package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestFlowRootParsesToBlockWithFlag(t *testing.T) {
	doc, styles := resolveTestDoc(`<html><body><div id="x"></div></body></html>`, `#x{display:flow-root}`)
	n := findElementByID(&doc.Node, "x")
	cs := styles[n.ID]
	if cs == nil {
		t.Fatal("no computed style for #x")
	}
	if !cs.FlowRoot {
		t.Error("FlowRoot = false, want true for display:flow-root")
	}
	if cs.Display != style.DisplayBlock {
		t.Errorf("Display = %v, want DisplayBlock for flow-root", cs.Display)
	}
}

func TestPlainBlockHasNoFlowRootFlag(t *testing.T) {
	doc, styles := resolveTestDoc(`<html><body><div id="x"></div></body></html>`, `#x{display:block}`)
	n := findElementByID(&doc.Node, "x")
	cs := styles[n.ID]
	if cs == nil {
		t.Fatal("no computed style for #x")
	}
	if cs.FlowRoot {
		t.Error("FlowRoot = true for plain display:block")
	}
}

func TestFlowRootDoesNotInheritToChild(t *testing.T) {
	doc, styles := resolveTestDoc(
		`<html><body><div id="p"><span id="c">text</span></div></body></html>`,
		`#p{display:flow-root}`)
	p := findElementByID(&doc.Node, "p")
	c := findElementByID(&doc.Node, "c")
	if styles[p.ID] == nil || !styles[p.ID].FlowRoot {
		t.Fatal("parent #p should carry FlowRoot")
	}
	cs := styles[c.ID]
	if cs == nil {
		t.Fatal("no computed style for child #c")
	}
	if cs.FlowRoot {
		t.Error("FlowRoot leaked to child #c; display is non-inherited")
	}
}

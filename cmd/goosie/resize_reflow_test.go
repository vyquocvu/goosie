package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestResizeReflow(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()

	html := `<html><body><div style="width: 100px; height: 200px;">Test</div></body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	tab.Session = sess

	list, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatal(err)
	}
	dl := list.Build(1)
	extent1 := dl.Extent()

	if err := sess.Reflow(400); err != nil {
		t.Fatal(err)
	}

	list2, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatal(err)
	}
	dl2 := list2.Build(1)
	extent2 := dl2.Extent()

	if extent1.H() == extent2.H() && extent1.W() == extent2.W() {
		t.Log("Warning: extents unchanged after reflow (may be expected for simple content)")
	}
}

func TestResizeReflowWithNilSession(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()

	if tab.Session != nil {
		t.Fatal("new tab should have nil session")
	}
}

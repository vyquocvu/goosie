package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

func TestRepaintTabPublishesChangedSessionBackground(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<!doctype html><html><body style="margin:0;background:rgb(10,20,30)"><p>stable</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	f.repaintTab(tab, frame.Rect{})
	layer := tab.Layer()
	if layer == nil {
		t.Fatal("repaintTab created no layer")
	}
	first, _ := f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
	if first == nil || first.Background != frame.RGB(10, 20, 30) {
		t.Fatalf("first plan=%v, want the session background", first)
	}

	sess.Doc.Body.SetAttribute("style", "margin:0;background:rgb(40,50,60)")
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	f.repaintTab(tab, frame.Rect{})
	if tab.Layer() != layer {
		t.Fatal("a background change discarded the reusable layer")
	}
	second, _ := f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
	if second == nil || second.Background != frame.RGB(40, 50, 60) {
		t.Fatalf("second plan=%v, want the changed session background", second)
	}
}

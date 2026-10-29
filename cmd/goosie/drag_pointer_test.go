package main

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func newPointerDragFrame(t *testing.T, html string) (*framePath, *engine.Session) {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	mgr := tabs.NewManager(nil)
	mgr.NewTab().SetSession(sess)
	return &framePath{tabMgr: mgr, config: config{width: 800, dpr: 1}, zoom: 1}, sess
}

func pointerDragPoint(t *testing.T, sess *engine.Session, id string) (int32, int32) {
	t.Helper()
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return int32((x0 + x1) / 2), int32((y0 + y1) / 2)
		}
	}
	t.Fatalf("layout object %q not found", id)
	return 0, 0
}

func TestPointerDragStartsAfterThresholdAndDropsSharedData(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:50px;height:50px">source</div>
		<div id="dst" style="width:50px;height:50px">target</div>
	</body></html>`
	f, sess := newPointerDragFrame(t, html)
	var events []string
	var droppedData string
	src, dst := sess.Doc.ElementByID("src"), sess.Doc.ElementByID("dst")
	dom.AddEventListener(src, "dragstart", func(e *dom.Event) {
		if !e.IsTrusted() {
			t.Error("dragstart is not trusted")
		}
		events = append(events, "dragstart")
		e.AsDrag().DataTransfer.SetData("text/plain", "payload")
	}, false)
	dom.AddEventListener(src, "drag", func(*dom.Event) { events = append(events, "drag") }, false)
	dom.AddEventListener(src, "dragend", func(*dom.Event) { events = append(events, "dragend") }, false)
	dom.AddEventListener(src, "dragleave", func(e *dom.Event) {
		events = append(events, "dragleave")
		if e.AsDrag().RelatedTarget != dst {
			t.Errorf("dragleave.relatedTarget = %v, want destination", e.AsDrag().RelatedTarget)
		}
	}, false)
	dom.AddEventListener(dst, "dragenter", func(e *dom.Event) {
		events = append(events, "dragenter")
		if e.AsDrag().RelatedTarget != nil {
			t.Errorf("first dragenter.relatedTarget = %v, want null", e.AsDrag().RelatedTarget)
		}
	}, false)
	dom.AddEventListener(dst, "dragover", func(e *dom.Event) {
		events = append(events, "dragover")
		e.PreventDefault()
	}, false)
	dom.AddEventListener(dst, "drop", func(e *dom.Event) {
		events = append(events, "drop")
		droppedData = e.AsDrag().DataTransfer.GetData("text/plain")
	}, false)

	sx, sy := pointerDragPoint(t, sess, "src")
	dx, dy := pointerDragPoint(t, sess, "dst")
	if !f.pointerDrag(surface.PointerPress, sx, sy) {
		t.Fatal("draggable pointer press was not claimed")
	}
	if !f.pointerDrag(surface.PointerMotion, sx+2, sy) {
		t.Fatal("pending pointer motion was not claimed")
	}
	if sess.Dragging() || len(events) != 0 {
		t.Fatalf("drag started below threshold: dragging=%v events=%v", sess.Dragging(), events)
	}
	if !f.pointerDrag(surface.PointerMotion, dx, dy) {
		t.Fatal("threshold-crossing pointer motion was not claimed")
	}
	if !sess.Dragging() {
		t.Fatal("drag did not start after threshold")
	}
	if !f.pointerDrag(surface.PointerRelease, dx, dy) {
		t.Fatal("pointer release did not finish drag")
	}
	want := []string{"dragstart", "dragenter", "dragover", "drag", "drop", "dragend"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("drag event sequence = %v, want %v", events, want)
	}
	if droppedData != "payload" {
		t.Fatalf("drop data = %q, want shared DataTransfer payload", droppedData)
	}
	if sess.Dragging() || f.contentDrag != nil {
		t.Fatal("drag state survived pointer release")
	}
}

func TestDraggablePointerPressReleaseFallsBackToTrustedClick(t *testing.T) {
	f, sess := newPointerDragFrame(t, `<html><body style="margin:0"><div id="src" draggable="true" style="width:60px;height:60px">source</div></body></html>`)
	var clicks int
	dom.AddEventListener(sess.Doc.ElementByID("src"), "click", func(e *dom.Event) {
		clicks++
		if !e.IsTrusted() {
			t.Error("pointer click event is not trusted")
		}
	}, false)
	x, y := pointerDragPoint(t, sess, "src")
	if !f.pointerDrag(surface.PointerPress, x, y) || !f.pointerDrag(surface.PointerRelease, x, y) {
		t.Fatal("draggable click gesture was not handled")
	}
	if clicks != 1 {
		t.Fatalf("click events = %d, want exactly one", clicks)
	}
	if sess.Dragging() {
		t.Fatal("click gesture left a drag active")
	}
}

func TestEscapeCancelsPointerDragWithoutDrop(t *testing.T) {
	f, sess := newPointerDragFrame(t, `<html><body style="margin:0"><div id="src" draggable="true" style="width:50px;height:50px">source</div><div id="dst" style="width:50px;height:50px">target</div></body></html>`)
	var dropped, ended bool
	src, dst := sess.Doc.ElementByID("src"), sess.Doc.ElementByID("dst")
	dom.AddEventListener(src, "dragend", func(*dom.Event) { ended = true }, false)
	dom.AddEventListener(dst, "dragover", func(e *dom.Event) { e.PreventDefault() }, false)
	dom.AddEventListener(dst, "drop", func(*dom.Event) { dropped = true }, false)
	sx, sy := pointerDragPoint(t, sess, "src")
	dx, dy := pointerDragPoint(t, sess, "dst")
	f.pointerDrag(surface.PointerPress, sx, sy)
	f.pointerDrag(surface.PointerMotion, dx, dy)
	if !f.cancelPointerDrag() {
		t.Fatal("Escape did not claim active pointer drag")
	}
	if sess.Dragging() || !ended || dropped {
		t.Fatalf("cancel result: dragging=%v ended=%v dropped=%v", sess.Dragging(), ended, dropped)
	}
}

func TestChromePointerRoutesDraggableGestureBeforeLinkActivation(t *testing.T) {
	cw := &chromeWindow{toolbar: &toolbar.State{}, tabMgr: tabs.NewManager(nil), events: make(chan surface.Event, 4)}
	var actions []surface.PointerAction
	linkHits := 0
	cw.onContentDrag = func(action surface.PointerAction, _, _ int32) bool {
		actions = append(actions, action)
		return true
	}
	cw.hitTestLink = func(_, _ int32) string {
		linkHits++
		return "https://example.test/"
	}
	cw.onLinkClick = func(string) { t.Fatal("draggable press activated a link before drag resolution") }
	press := surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft,
		Pos: frame.Point{X: 20, Y: int32(totalChromeHeight) + 20}}
	if !cw.intercept(press) {
		t.Fatal("draggable press was not consumed")
	}
	if linkHits != 0 {
		t.Fatal("link hit testing ran before pending drag resolution")
	}
	release := press
	release.Action = surface.PointerRelease
	if !cw.intercept(release) {
		t.Fatal("draggable release was not consumed")
	}
	if !reflect.DeepEqual(actions, []surface.PointerAction{surface.PointerPress, surface.PointerRelease}) {
		t.Fatalf("drag callback actions = %v", actions)
	}
	if linkHits != 0 {
		t.Fatal("drag callback did not suppress immediate link activation")
	}
}

func TestEscapeIsOfferedToContentDragBeforePageKeys(t *testing.T) {
	cw := &chromeWindow{toolbar: &toolbar.State{}, events: make(chan surface.Event, 2)}
	cancelled := false
	pageKey := false
	cw.onCancelDrag = func() bool { cancelled = true; return true }
	cw.onContentKey = func(rune, surface.KeyMod) bool { pageKey = true; return true }
	if !cw.intercept(surface.Event{Kind: surface.EvKey, Key: 0x1B}) {
		t.Fatal("Escape was not consumed by drag cancellation")
	}
	if !cancelled || pageKey {
		t.Fatalf("Escape routing: cancelled=%v pageKey=%v", cancelled, pageKey)
	}
}

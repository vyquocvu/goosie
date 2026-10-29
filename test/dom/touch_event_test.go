package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// The touch lists one gesture action produces, measured against Chromium with
// test/wpt/resources/touch_probe.py: while a contact is held it is in `touches`,
// and touchend arrives with `touches` empty but one entry in `changedTouches`.
// The caller owns the held set because removing the released contact is the
// host's decision, not the event's.
func TestTrustedTouchEventCarriesItsTouchLists(t *testing.T) {
	target := &dom.Node{}
	held := &dom.Touch{Identifier: 1, ClientX: 50, ClientY: 50, Target: target}

	start := dom.NewTrustedTouchEvent("touchstart", target, []*dom.Touch{held}, []*dom.Touch{held})
	te := start.AsTouch()
	if te == nil {
		t.Fatal("touchstart has no TouchEvent payload")
	}
	if got := start.InterfaceName(); got != "TouchEvent" {
		t.Errorf("InterfaceName() = %q, want %q", got, "TouchEvent")
	}
	if len(te.Touches) != 1 || len(te.ChangedTouches) != 1 {
		t.Errorf("touchstart lists: touches=%d changedTouches=%d, want 1 and 1", len(te.Touches), len(te.ChangedTouches))
	}
	if te.Touches[0].Identifier != 1 || te.Touches[0].ClientX != 50 || te.Touches[0].ClientY != 50 {
		t.Errorf("touchstart touch = %+v, want identifier 1 at (50, 50)", te.Touches[0])
	}

	// A second finger joins the held set for both lists of its own touchstart,
	// and the first contact keeps its identifier.
	second := &dom.Touch{Identifier: 2, ClientX: 80, ClientY: 90, Target: target}
	move := dom.NewTrustedTouchEvent("touchmove", target, []*dom.Touch{held, second}, []*dom.Touch{second})
	if me := move.AsTouch(); me == nil || len(me.Touches) != 2 || len(me.ChangedTouches) != 1 {
		t.Fatalf("two-finger touchmove: %+v, want 2 touches and 1 changed", me)
	}

	end := dom.NewTrustedTouchEvent("touchend", target, nil, []*dom.Touch{held})
	if te := end.AsTouch(); te == nil || len(te.Touches) != 0 || len(te.ChangedTouches) != 1 {
		t.Fatalf("touchend: %+v, want 0 touches and 1 changed", te)
	}
}

// `targetTouches` is the held set limited to contacts whose target is the
// event target; a touch aimed at a different node leaves it empty.
func TestTouchEventTargetTouchesFollowsTheTouchTarget(t *testing.T) {
	other := &dom.Node{}
	inDoc := &dom.Node{}
	held := &dom.Touch{Identifier: 1, Target: inDoc}

	same := dom.NewTrustedTouchEvent("touchstart", inDoc, []*dom.Touch{held}, []*dom.Touch{held})
	if te := same.AsTouch(); te == nil || len(te.TargetTouches) != 1 {
		t.Fatalf("touch whose target is the event target: %+v, want 1 targetTouches", te)
	}

	elsewhere := dom.NewTrustedTouchEvent("touchstart", other, []*dom.Touch{held}, []*dom.Touch{held})
	if te := elsewhere.AsTouch(); te == nil || len(te.TargetTouches) != 0 {
		t.Fatalf("touch aimed at another node: %+v, want 0 targetTouches", te)
	}
}

// touchstart and touchmove cancel the scroll they cause, so a generated one
// reports the dispatch-time answer like a wheel gesture does. touchend and
// touchcancel block no scroll and keep the flag they were built with.
func TestGeneratedTouchCancelabilityCoversOnlyTheScrollBlockingTypes(t *testing.T) {
	for _, typ := range []string{"touchstart", "touchmove"} {
		e := dom.NewTrustedTouchEvent(typ, nil, nil, nil)
		if !dom.IsScrollBlockingEventType(typ) {
			t.Errorf("%s is not a scroll-blocking type, so its cancelable flag would not derive", typ)
		}
		if e.Cancelable() {
			t.Errorf("%s before the dispatcher answers: Cancelable() = true, want false", typ)
		}
		e.SetScrollCancelableByListener(true)
		if !e.Cancelable() {
			t.Errorf("%s with a non-passive listener: Cancelable() = false, want true", typ)
		}
	}
	for _, typ := range []string{"touchend", "touchcancel"} {
		e := dom.NewTrustedTouchEvent(typ, nil, nil, nil)
		if dom.IsScrollBlockingEventType(typ) {
			t.Errorf("%s is treated as scroll-blocking", typ)
		}
		if !e.Cancelable() {
			t.Errorf("%s: Cancelable() = false, want true", typ)
		}
		e.SetScrollCancelableByListener(false)
		if !e.Cancelable() {
			t.Errorf("%s lost its flag to listener state: Cancelable() = false, want true", typ)
		}
	}
}

// A generated touch event bubbles: the corpus listens on window, document, the
// root and body while the gesture lands on a div.
func TestGeneratedTouchEventBubbles(t *testing.T) {
	for _, typ := range []string{"touchstart", "touchmove", "touchend", "touchcancel"} {
		e := dom.NewTrustedTouchEvent(typ, nil, nil, nil)
		if !e.Bubbles() {
			t.Errorf("%s does not bubble", typ)
		}
		if !e.IsTrusted() {
			t.Errorf("%s is not trusted", typ)
		}
	}
}

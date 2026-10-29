package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// joinCollected joins the entries a page pushed into a global array during
// dispatch. The array only fills while the gesture runs, so the join has to be a
// second evaluation rather than part of the probe script.
func joinCollected(t *testing.T, r *js.Runtime, name string) string {
	t.Helper()
	if err := r.Run("var __joined = "+name+".join('|');", "join collected"); err != nil {
		t.Fatal(err)
	}
	v, ok := r.Global("__joined")
	if !ok {
		t.Fatalf("%s is missing", name)
	}
	return v.Str
}

// WPT dom/events/non-cancelable-when-passive/synthetic-events-cancelable.html
// asserts `TouchEvent in self` for touchstart, touchmove, touchend and
// touchcancel, so the interface has to be reachable as a global constructor.
func TestTouchInterfacesAreExposed(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var report = ["TouchEvent", "Touch", "TouchList"]
			.map(function (name) {
				return name + '=' + (typeof self[name] === "function") + (name in self);
			}).join("|");
	`, "touch interfaces"); err != nil {
		t.Fatal(err)
	}
	v, ok := r.Global("report")
	if !ok {
		t.Fatal("report is missing")
	}
	if want := "TouchEvent=truetrue|Touch=truetrue|TouchList=truetrue"; v.Str != want {
		t.Errorf("report = %q, want %q", v.Str, want)
	}
}

// The same file constructs `new TouchEvent(type)` and asserts the event is
// non-cancelable, that its preventDefault() is ignored, and that dispatchEvent
// returns true. A constructed touch event also carries all three lists, empty.
func TestConstructedTouchEventIsNonCancelableWithEmptyLists(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var prevented = [];
		window.addEventListener("touchstart", function (event) { prevented.push(event.defaultPrevented); });
		var event = new TouchEvent("touchstart");
		var report = [
			"cancelable=" + event.cancelable,
			"bubbles=" + event.bubbles,
			"touches=" + event.touches.length,
			"targetTouches=" + event.targetTouches.length,
			"changedTouches=" + event.changedTouches.length,
			"listIsTouchList=" + (event.touches instanceof TouchList),
			"returns=" + document.dispatchEvent(event),
			"prevented=" + prevented.join(","),
		].join("|");
	`, "constructed touch event"); err != nil {
		t.Fatal(err)
	}
	v, ok := r.Global("report")
	if !ok {
		t.Fatal("report is missing")
	}
	want := "cancelable=false|bubbles=false|touches=0|targetTouches=0|changedTouches=0|listIsTouchList=true|returns=true|prevented=false"
	if v.Str != want {
		t.Errorf("report = %q, want %q", v.Str, want)
	}
}

// TouchEventInit carries the three lists and TouchInit the contact itself, so a
// page can build a gesture and read it back through item(), indexing, the
// coordinates and the target.
func TestConstructedTouchEventReadsBackItsTouchLists(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var div = document.getElementById("target");
		var first = new Touch({identifier: 7, target: div, clientX: 50, clientY: 60});
		var second = new Touch({identifier: 8, target: div, clientX: 80, clientY: 90});
		var event = new TouchEvent("touchmove", {
			bubbles: true, cancelable: true,
			touches: [first, second], targetTouches: [second], changedTouches: [second],
		});
		var list = event.touches;
		var report = [
			"length=" + list.length,
			"id=" + list.item(0).identifier,
			"coords=" + list[1].clientX + "," + list[1].clientY,
			"pageCoords=" + list[0].pageX + "," + list[0].pageY,
			"target=" + (list.item(0).target === div),
			"changed=" + event.changedTouches.length,
			"targetTouches=" + event.targetTouches.length,
			"bubbles=" + event.bubbles,
			"cancelable=" + event.cancelable,
			"isTrusted=" + event.isTrusted,
			"tag=" + Object.prototype.toString.call(event),
		].join("|");
	`, "touch lists"); err != nil {
		t.Fatal(err)
	}
	v, ok := r.Global("report")
	if !ok {
		t.Fatal("report is missing")
	}
	want := "length=2|id=7|coords=80,90|pageCoords=50,60|target=true|changed=1|targetTouches=1|bubbles=true|cancelable=true|isTrusted=false|tag=[object TouchEvent]"
	if v.Str != want {
		t.Errorf("report = %q, want %q", v.Str, want)
	}
}

// A generated touch event reports the dispatch-time answer derived from the
// listeners of the type being dispatched: a listener registered passive leaves
// the gesture non-cancelable for every listener of that dispatch, a non-passive
// one makes it cancelable for all of them, and the default-passive globals get
// the same treatment as any other node.
func TestGeneratedTouchEventCancelableFollowsTheListenerOfItsOwnType(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options string
		want    string
	}{
		{"passive", "{passive: true}", "false"},
		{"non-passive", "{passive: false}", "true"},
		{"unspecified", "{}", "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := eventDOM()
			r := newEventRuntime(t, doc)
			if err := r.Run(`
				var seen = [];
				window.addEventListener("touchstart", function (event) {
					seen.push([
						"cancelable=" + event.cancelable,
						"trusted=" + event.isTrusted,
						"instanceOf=" + (event instanceof TouchEvent) + (event.touches instanceof TouchList),
						"touches=" + event.touches.length,
						"changed=" + event.changedTouches.length,
						"client=" + event.touches.item(0).clientX + "," + event.touches.item(0).clientY,
					].join("|"));
				}, `+tc.options+`);
			`, "install touch probe"); err != nil {
				t.Fatal(err)
			}
			div := doc.ElementByID("target")
			touch := &dom.Touch{Identifier: 1, ClientX: 40, ClientY: 45, Target: div}
			event := dom.NewTrustedTouchEvent("touchstart", div, []*dom.Touch{touch}, []*dom.Touch{touch})
			// Nothing called preventDefault, so the gesture survives either way:
			// the return value is whether the default action may run, not whether
			// the page could have stopped it.
			if !r.DispatchTrustedTouch(div, event) {
				t.Errorf("gesture reported cancelled with no listener cancelling it (%s listener)", tc.name)
			}
			report := joinCollected(t, r, "seen")
			want := "cancelable=" + tc.want + "|trusted=true|instanceOf=truetrue|touches=1|changed=1|client=40,45"
			if report != want {
				t.Errorf("listener saw %q, want %q", report, want)
			}
		})
	}
}

// The return value flips only when a listener that may cancel the gesture
// actually does: a non-passive touchstart that calls preventDefault stops the
// scroll, which is what keeps a host from scrolling a page that asked not to be.
func TestGeneratedTouchEventPreventDefaultCancelsTheGesture(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var cancelled = [];
		div = document.getElementById("target");
		div.addEventListener("touchstart", function (event) {
			cancelled.push(event.cancelable);
			event.preventDefault();
		}, {passive: false});
	`, "cancelling touch listener"); err != nil {
		t.Fatal(err)
	}
	div := doc.ElementByID("target")
	touch := &dom.Touch{Identifier: 1, ClientX: 10, ClientY: 10, Target: div}
	event := dom.NewTrustedTouchEvent("touchstart", div, []*dom.Touch{touch}, []*dom.Touch{touch})
	if r.DispatchTrustedTouch(div, event) {
		t.Error("DispatchTrustedTouch = true after a non-passive listener cancelled the gesture, want false")
	}
	report := joinCollected(t, r, "cancelled")
	if report != "true" {
		t.Errorf("listener saw cancelable %q, want true", report)
	}
}

// The cancelable flag is one value per dispatch: a non-passive listener anywhere
// in the path makes the gesture read cancelable even to a passive listener, and
// that passive listener's preventDefault() is still ignored.
func TestGeneratedTouchEventCancelableIsSharedAndPassivePreventIsIgnored(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var report = [];
		div = document.getElementById("target");
		div.addEventListener("touchstart", function (event) {
			event.preventDefault();
			report.push("divCancelable=" + event.cancelable + ",prevented=" + event.defaultPrevented);
		}, {passive: true});
		window.addEventListener("touchstart", function (event) {
			report.push("windowCancelable=" + event.cancelable);
		}, {passive: false});
	`, "passive touch listener"); err != nil {
		t.Fatal(err)
	}
	div := doc.ElementByID("target")
	touch := &dom.Touch{Identifier: 1, ClientX: 10, ClientY: 10, Target: div}
	event := dom.NewTrustedTouchEvent("touchstart", div, []*dom.Touch{touch}, []*dom.Touch{touch})
	if !r.DispatchTrustedTouch(div, event) {
		t.Fatal("gesture reported cancelled, want the passive listener's preventDefault ignored")
	}
	got := joinCollected(t, r, "report")
	// The div is the event target and Window is the last item in the path, so the
	// target's listener runs first.
	want := "divCancelable=true,prevented=false|windowCancelable=true"
	if got != want {
		t.Errorf("report = %q, want %q", got, want)
	}
}

// touchend blocks no scroll, so a generated one keeps its own flag no matter how
// passive the listeners are - and it reports the released contact in
// changedTouches only.
func TestGeneratedTouchEndStaysCancelableAndListsOnlyTheReleasedContact(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var seen = [];
		window.addEventListener("touchend", function (event) {
			seen.push("cancelable=" + event.cancelable + "|touches=" + event.touches.length +
				"|changed=" + event.changedTouches.length);
		}, {passive: true});
	`, "touch end probe"); err != nil {
		t.Fatal(err)
	}
	div := doc.ElementByID("target")
	touch := &dom.Touch{Identifier: 1, ClientX: 10, ClientY: 10, Target: div}
	event := dom.NewTrustedTouchEvent("touchend", div, nil, []*dom.Touch{touch})
	if !r.DispatchTrustedTouch(div, event) {
		t.Error("touchend with only a passive listener: DispatchTrustedTouch = false, want the event to keep its flag")
	}
	got := joinCollected(t, r, "seen")
	if want := "cancelable=true|touches=0|changed=1"; got != want {
		t.Errorf("listener saw %q, want %q", got, want)
	}
}

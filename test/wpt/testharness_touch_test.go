package wpt

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// touchSequence is the input WPT's resources/touching.js injects: one touch
// pointer that lands on #box, presses, moves to viewport (30, 30), and releases.
// The move is absolute, not pointer-relative - testdriver-actions.js defaults
// pointerMove's origin to "viewport" - and it stays inside the 100x100 box the
// corpus gives #touchDiv, which is what lets the touchmove files observe a move
// on the node that received the press. The serialized source carries
// parameters.pointerType, which is how the runner tells a touch gesture from a
// mouse drag.
const touchSequence = `test_driver.action_sequence([{type:"pointer", id:"touch_pointer", parameters:{pointerType:"touch"}, actions:[
	{type:"pointerMove", x:0, y:0, origin:document.getElementById("box")},
	{type:"pointerDown", button:0},
	{type:"pointerMove", x:30, y:30},
	{type:"pointerUp", button:0}
]}])`

// touchPage runs a WPT touch sequence against a document with a 100x100 #box and
// reports what the page's listeners saw. The expectations come from measurements
// of Chromium's own behaviour, taken with test/wpt/resources/touch_probe.py:
// touchstart and touchmove report the same cancelable value to every listener of
// that dispatch, derived from whether a non-passive listener of the type being
// dispatched can still be reached; touchend blocks no scroll and keeps its flag.
func touchPage(t *testing.T, setup, sequence string) string {
	t.Helper()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="box" style="width:100px;height:100px"></div></body></html>`, nil, 800, engine.WithViewportH(600), engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	prologue := `
		var report = [];
		function probe(label, target, options, type) {
			target.addEventListener(type, function (event) {
				report.push(label + '/' + type + ':' + event.cancelable + ':' + event.isTrusted + ':' +
					event.touches.length + ':' + event.changedTouches.length);
			}, options);
		}
		var div = document.getElementById('box');
		`
	if _, err := s.EvaluateJS(prologue + setup); err != nil {
		t.Fatalf("install touch probes: %v", err)
	}
	if _, err := s.EvaluateJS(sequence); err != nil {
		t.Fatalf("run WPT touch sequence: %v", err)
	}
	report, err := s.EvaluateJS(`report.join("|")`)
	if err != nil {
		t.Fatalf("read touch report: %v", err)
	}
	return report
}

// A non-passive listener on the element the gesture lands on may cancel the
// scroll, so touchstart and touchmove both read cancelable, and the moving
// pointer is what makes the corpus's touchmove files observable at all.
func TestWPTTestDriverTouchSequenceDispatchesCancelableTrustedTouches(t *testing.T) {
	report := touchPage(t, `
		probe('div', div, {passive: false}, 'touchstart');
		probe('div', div, {passive: false}, 'touchmove');
	`, touchSequence)
	want := "div/touchstart:true:true:1:1|div/touchmove:true:true:1:1"
	if report != want {
		t.Errorf("dispatch = %q, want %q", report, want)
	}
}

// Every corpus file registers one listener on one of the default-passive globals
// or on a div. With no option, a window listener cannot cancel the gesture, so
// the generated touchstart reports itself non-cancelable - the same rule the wheel
// gesture follows.
func TestWPTTestDriverTouchSequenceStaysNonCancelableForPassiveListeners(t *testing.T) {
	for _, target := range []struct{ label, expression string }{
		{"window", "window"},
		{"document", "document"},
		{"root", "document.documentElement"},
		{"body", "document.body"},
	} {
		t.Run(target.label, func(t *testing.T) {
			report := touchPage(t, "probe('"+target.label+"', "+target.expression+", {}, 'touchstart');", touchSequence)
			want := target.label + "/touchstart:false:true:1:1"
			if report != want {
				t.Errorf("dispatch = %q, want %q", report, want)
			}
		})
	}
}

// A listener registered explicitly passive reads the same non-cancelable flag as
// an unregistered one, and a touchend on a plain element keeps its own flag
// because releasing a contact cancels no scroll.
func TestWPTTestDriverTouchSequencePassiveAndTouchEnd(t *testing.T) {
	report := touchPage(t, `
		probe('div', div, {passive: true}, 'touchstart');
		probe('div', div, {}, 'touchend');
	`, touchSequence)
	want := "div/touchstart:false:true:1:1|div/touchend:true:true:0:1"
	if report != want {
		t.Errorf("dispatch = %q, want %q", report, want)
	}
}

// The move that positions a gesture puts no contact on the screen, and a release
// with nothing held lifts nothing, so a sequence that never presses raises no
// touch event at all. Only a held contact can move or be released.
func TestWPTTestDriverTouchSequenceWithoutPressDispatchesNothing(t *testing.T) {
	report := touchPage(t, `
		probe('window', window, {passive: false}, 'touchstart');
		probe('window', window, {passive: false}, 'touchmove');
		probe('window', window, {}, 'touchend');
	`, `test_driver.action_sequence([{type:"pointer", id:"touch_pointer", parameters:{pointerType:"touch"}, actions:[
		{type:"pointerMove", x:0, y:0, origin:document.getElementById("box")},
		{type:"pointerUp", button:0}
	]}])`)
	if report != "" {
		t.Errorf("dispatch without a press = %q, want no touch event at all", report)
	}
}

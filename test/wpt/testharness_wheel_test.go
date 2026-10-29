package wpt

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// wheelScrollPage runs one WPT scroll action against a document tall enough to
// scroll and reports what the page's listeners saw. The expectations come from
// measurements of Chromium's own behaviour, taken with
// test/wpt/resources/wheel_probe.py:
//
//   - a generated wheel event is cancelable only while the dispatch can reach a
//     non-passive wheel or mousewheel listener, and every listener of that
//     dispatch reads the same value;
//   - the legacy mousewheel event is dispatched only when the path has no wheel
//     listener at all;
//   - the scroll itself happens unless a listener cancels the gesture.
func wheelScrollPage(t *testing.T, setup, action string) (report string, scrollY string) {
	t.Helper()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="box" style="height:2000px"></div></body></html>`, nil, 800, engine.WithViewportH(600), engine.WithJS(rt))
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
				if (window.cancelIn) event.preventDefault();
				report.push(label + '/' + type + ':' + event.isTrusted + ':' + event.deltaX + ':' +
					event.deltaY + ':' + event.clientX + ':' + event.clientY + ':' + event.cancelable);
			}, options);
		}
		var div = document.getElementById('box');
		`
	if _, err := s.EvaluateJS(prologue + setup); err != nil {
		t.Fatalf("install wheel probes: %v", err)
	}
	if _, err := s.EvaluateJS(action); err != nil {
		t.Fatalf("run WPT scroll action: %v", err)
	}
	report, err = s.EvaluateJS(`report.join("|")`)
	if err != nil {
		t.Fatalf("read wheel report: %v", err)
	}
	scrollY, err = s.EvaluateJS(`String(window.scrollY)`)
	if err != nil {
		t.Fatalf("read window.scrollY: %v", err)
	}
	return report, scrollY
}

const wheelScrollAction = `test_driver.action_sequence([{type:"wheel",actions:[{type:"scroll",x:400,y:300,deltaX:0,deltaY:100}]}])`

// The five default-passive targets plus a plain element, all registered with no
// passive option. The element listener is the only non-passive one, so the
// gesture is cancelable for every listener of the dispatch and the legacy
// mousewheel event is skipped.
func TestWPTTestDriverWheelScrollActionDispatchesCancelableTrustedWheel(t *testing.T) {
	report, scrollY := wheelScrollPage(t, `
		probe('window', window, {}, 'wheel');
		probe('document', document, {}, 'wheel');
		probe('root', document.documentElement, {}, 'wheel');
		probe('body', document.body, {}, 'wheel');
		probe('div', div, {}, 'wheel');
	`, wheelScrollAction)
	want := "div/wheel:true:0:100:400:300:true|" +
		"body/wheel:true:0:100:400:300:true|" +
		"root/wheel:true:0:100:400:300:true|" +
		"document/wheel:true:0:100:400:300:true|" +
		"window/wheel:true:0:100:400:300:true"
	if report != want {
		t.Errorf("dispatch = %q, want %q", report, want)
	}
	if scrollY != "100" {
		t.Errorf("window.scrollY after an uncancelled scroll = %s, want 100", scrollY)
	}
}

// Only default-passive targets listen, so nothing may cancel the scroll: the
// generated wheel reports itself non-cancelable. Because a wheel listener is in
// the path, the legacy mousewheel event is not dispatched as well.
func TestWPTTestDriverWheelScrollActionStaysNonCancelableForPassiveListeners(t *testing.T) {
	report, scrollY := wheelScrollPage(t, `
		probe('window', window, {}, 'wheel');
		probe('window', window, {}, 'mousewheel');
	`, wheelScrollAction)
	want := "window/wheel:true:0:100:400:300:false"
	if report != want {
		t.Errorf("dispatch = %q, want %q", report, want)
	}
	if scrollY != "100" {
		t.Errorf("window.scrollY after an uncancelled scroll = %s, want 100", scrollY)
	}
}

// A page that only listens for the legacy type still gets it: the gesture has no
// wheel listener to hand to, so mousewheel is dispatched, and its own listener
// decides whether the event can be prevented.
func TestWPTTestDriverWheelScrollActionFallsBackToLegacyMousewheel(t *testing.T) {
	report, scrollY := wheelScrollPage(t, `
		probe('div', div, {passive: false}, 'mousewheel');
	`, wheelScrollAction)
	want := "div/mousewheel:true:0:100:400:300:true"
	if report != want {
		t.Errorf("dispatch = %q, want %q", report, want)
	}
	if scrollY != "100" {
		t.Errorf("window.scrollY after an uncancelled scroll = %s, want 100", scrollY)
	}
}

// The page's other assertions in this corpus rely on the cancel path: a
// non-passive listener that calls preventDefault must stop the scroll.
func TestWPTTestDriverWheelScrollActionHonorsPreventDefault(t *testing.T) {
	report, scrollY := wheelScrollPage(t, `
		window.cancelIn = true;
		probe('div', div, {passive: false}, 'wheel');
	`, wheelScrollAction)
	want := "div/wheel:true:0:100:400:300:true"
	if report != want {
		t.Errorf("dispatch = %q, want %q", report, want)
	}
	if scrollY != "0" {
		t.Errorf("window.scrollY after a cancelled scroll = %s, want 0", scrollY)
	}
}

package wpt

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

const touchSequence = `test_driver.action_sequence([{type:"pointer", id:"touch_pointer", parameters:{pointerType:"touch"}, actions:[
	{type:"pointerMove", x:0, y:0, origin:document.getElementById("box")},
	{type:"pointerDown", button:0},
	{type:"pointerMove", x:30, y:30},
	{type:"pointerUp", button:0}
]}])`

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

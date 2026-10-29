package js_test

import (
	"testing"
)

// An event a page constructed keeps the cancelable flag it was given, even where
// the listener cannot use it: Chromium reports cancelable=true to a passive
// listener on a scroll-blocking type and simply ignores preventDefault(). Only
// browser-generated scroll-blocking events hide the flag, and that is measured in
// test/wpt where the engine can synthesize one.
func TestSyntheticScrollCancelableStaysVisibleToPassiveListeners(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	script := `
		var report = [];
		function probe(target, label, options, type) {
			var merged = Object.assign({ once: true }, options);
			target.addEventListener(type, function (event) {
				event.preventDefault();
				report.push(label + "=" + event.cancelable + "," + event.defaultPrevented);
			}, merged);
			report.push(label + "!ret=" + target.dispatchEvent(new Event(type, { cancelable: true })));
		}
		var div = document.getElementById('target');
		for (const type of ["wheel", "mousewheel", "touchstart", "touchmove"]) {
			probe(div, "div-passive-" + type, { passive: true }, type);
			probe(div, "div-active-" + type, { passive: false }, type);
			probe(window, "window-default-" + type, {}, type);
		}
		probe(div, "div-passive-test", { passive: true }, "test");
		probe(window, "window-default-touchend", {}, "touchend");
	`
	if err := r.Run(script, "synthetic-scroll-cancelable"); err != nil {
		t.Fatal(err)
	}
	report, err := r.Evaluate(`report.join("|")`)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	want := ""
	for _, typ := range []string{"wheel", "mousewheel", "touchstart", "touchmove"} {
		// A passive listener sees the flag but cannot use it, so the dispatch was
		// not canceled and dispatchEvent() returns true.
		want += "div-passive-" + typ + "=true,false|div-passive-" + typ + "!ret=true|"
		want += "div-active-" + typ + "=true,true|div-active-" + typ + "!ret=false|"
		want += "window-default-" + typ + "=true,false|window-default-" + typ + "!ret=true|"
	}
	want += "div-passive-test=true,false|div-passive-test!ret=true|"
	want += "window-default-touchend=true,true|window-default-touchend!ret=false"
	if report != want {
		t.Errorf("report =\n  %s\nwant\n  %s", report, want)
	}
}

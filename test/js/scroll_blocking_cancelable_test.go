package js_test

import (
	"testing"
)

// The WPT dom/events/non-cancelable-when-passive corpus asserts what a listener
// sees on event.cancelable: a scroll-blocking type reports false for a passive
// listener and true for a non-passive one, a type that is not scroll-blocking
// reports true for a passive listener, and the default-passive targets (window,
// document, body, documentElement) report false without an explicit option.
func TestScrollBlockingCancelableSeenByListeners(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	script := `
		var report = [];
		function probe(target, label, options, type) {
			var merged = Object.assign({ once: true }, options);
			target.addEventListener(type, function (event) {
				report.push(label + "=" + event.cancelable);
			}, merged);
			target.dispatchEvent(new Event(type, { cancelable: true }));
		}
		var div = document.getElementById('target');
		for (const type of ["wheel", "mousewheel", "touchstart", "touchmove"]) {
			probe(div, "div-passive-" + type, { passive: true }, type);
			probe(div, "div-active-" + type, { passive: false }, type);
			probe(div, "div-plain-" + type, {}, type);
			probe(window, "window-default-" + type, {}, type);
		}
		probe(div, "div-passive-test", { passive: true }, "test");
		probe(window, "window-default-touchend", {}, "touchend");
	`
	if err := r.Run(script, "scroll-blocking-cancelable"); err != nil {
		t.Fatal(err)
	}
	report, err := r.Evaluate(`report.join(",")`)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	want := []string{
		"div-passive-wheel=false",
		"div-active-wheel=true",
		"div-plain-wheel=true",
		"window-default-wheel=false",
		"div-passive-mousewheel=false",
		"div-active-mousewheel=true",
		"div-plain-mousewheel=true",
		"window-default-mousewheel=false",
		"div-passive-touchstart=false",
		"div-active-touchstart=true",
		"div-plain-touchstart=true",
		"window-default-touchstart=false",
		"div-passive-touchmove=false",
		"div-active-touchmove=true",
		"div-plain-touchmove=true",
		"window-default-touchmove=false",
		"div-passive-test=true",
		"window-default-touchend=true",
	}
	got := splitReport(report)
	if len(got) != len(want) {
		t.Fatalf("report has %d probes, want %d: %v", len(got), len(want), report)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("probe %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func splitReport(report string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(report); i++ {
		if i == len(report) || report[i] == ',' {
			if i > start {
				out = append(out, report[start:i])
			}
			start = i + 1
		}
	}
	return out
}

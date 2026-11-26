package js_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func viewportRuntime(t *testing.T, opts js.Options) *js.Runtime {
	t.Helper()
	doc := domtest.Parse("<html><body></body></html>")
	opts.DOM = doc
	r, err := js.New(opts)
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	r.SetViewportRect(0, 0, 320, 240)
	r.SetDevicePixelRatio(1)
	return r
}

func TestMatchMediaMatchesChromiumViewportMeasurements(t *testing.T) {
	r := viewportRuntime(t, js.Options{URL: "https://example.test/viewport"})
	cases := []struct {
		query string
		want  bool
	}{
		{"(min-width: 320px)", true},
		{"(width: 320px)", true},
		{"(width: 321px)", false},
		{"(max-width: 319px)", false},
		{"(min-height: 240px)", true},
		{"(max-height: 239px)", false},
		{"(orientation: landscape)", true},
		{"(orientation: portrait)", false},
		{"(resolution: 1dppx)", true},
		{"(resolution: 96dpi)", true},
		{"(min-resolution: 2dppx)", false},
		{"(min-device-pixel-ratio: 1)", false},
		{"(-webkit-min-device-pixel-ratio: 1)", true},
		{"(prefers-color-scheme: light)", true},
		{"(prefers-color-scheme: dark)", false},
		{"(hover: hover)", true},
		{"(hover: none)", false},
		{"(pointer: fine)", true},
		{"(pointer: coarse)", false},
		{"(any-hover: hover)", true},
		{"(any-pointer: fine)", true},
		{"(prefers-reduced-motion: no-preference)", true},
		{"(prefers-reduced-motion: reduce)", false},
		{"(unknown-feature: yes)", false},
		{"(min-width: bananas)", false},
		{"and", false},
		{"not (min-width: 321px)", true},
		{"screen and (min-width: 300px)", true},
		{"print", false},
		{"(min-width: 300px), print", true},
	}
	for _, tc := range cases {
		got, err := r.Evaluate(fmt.Sprintf("matchMedia(%q).matches", tc.query))
		if err != nil {
			t.Fatalf("matchMedia(%q): %v", tc.query, err)
		}
		if got != fmt.Sprint(tc.want) {
			t.Errorf("matchMedia(%q).matches = %s, want %v", tc.query, got, tc.want)
		}
	}
}

func TestMatchMediaUsesConfiguredPreferencesAndPointerInputs(t *testing.T) {
	r := viewportRuntime(t, js.Options{
		URL:           "https://example.test/viewport",
		DarkMode:      true,
		HighContrast:  true,
		ReducedMotion: true,
	})
	r.SetPointerCapabilities(false, false)
	got, err := r.Evaluate(`[
		matchMedia('(prefers-color-scheme: dark)').matches,
		matchMedia('(prefers-color-scheme: light)').matches,
		matchMedia('(prefers-contrast: more)').matches,
		matchMedia('(prefers-reduced-motion: reduce)').matches,
		matchMedia('(hover: none)').matches,
		matchMedia('(pointer: coarse)').matches
	].join(',')`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "true,false,true,true,true,true"; got != want {
		t.Fatalf("configured media features = %q, want %q", got, want)
	}
}

func TestMatchMediaChangeEventsTrackTheCurrentViewport(t *testing.T) {
	r := viewportRuntime(t, js.Options{URL: "https://example.test/viewport"})
	if err := r.Run(`
		globalThis.mediaEvents = [];
		globalThis.narrow = matchMedia('(max-width: 350px)');
		globalThis.narrow.addEventListener('change', globalThis.modernListener = function(event) {
			mediaEvents.push('modern:' + event.matches + ':' + innerWidth + 'x' + innerHeight + ':' +
				(event.target === narrow) + ':' + (event.currentTarget === narrow));
		});
		globalThis.narrow.addListener(function(event) {
			mediaEvents.push('legacy:' + event.matches + ':' + event.media);
		});
	`, "media-events.js"); err != nil {
		t.Fatal(err)
	}
	r.SetViewportRect(0, 0, 400, 300)
	r.SetViewportRect(0, 0, 500, 300)
	r.Tick()
	got, err := r.Evaluate(`mediaEvents.join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	want := "modern:false:500x300:true:true|legacy:false:(max-width: 350px)"
	if got != want {
		t.Fatalf("coalesced media events = %q, want %q", got, want)
	}
	if err := r.Run(`narrow.removeEventListener('change', modernListener);`, "remove.js"); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(`
		globalThis.narrow.addEventListener('change', function(event) {
			mediaEvents.push('once:' + event.matches);
		}, { once: true });
	`, "once.js"); err != nil {
		t.Fatal(err)
	}
	r.SetViewportRect(0, 0, 320, 240)
	r.Tick()
	got, err = r.Evaluate(`mediaEvents.join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	want += "|legacy:true:(max-width: 350px)|once:true"
	if got != want {
		t.Fatalf("media events after crossing back = %q, want %q", got, want)
	}
	r.SetViewportRect(0, 0, 500, 300)
	r.Tick()
	got, err = r.Evaluate(`mediaEvents.join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	want += "|legacy:false:(max-width: 350px)"
	if got != want {
		t.Fatalf("once listener fired again: %q, want %q", got, want)
	}
}

func TestMatchMediaBoundsQueriesAndListeners(t *testing.T) {
	r := viewportRuntime(t, js.Options{URL: "https://example.test/viewport"})
	got, err := r.Evaluate(`
		var queryFailures = 0;
		var list = matchMedia('all');
		for (var i = 0; i < 300; i++) {
			try { matchMedia('(min-width: 1px)'); } catch (error) { queryFailures++; }
		}
		var listenerFailures = 0;
		for (var i = 0; i < 600; i++) {
			try { list.addEventListener('change', function() {}); } catch (error) { listenerFailures++; }
		}
		queryFailures + ':' + listenerFailures;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got == "0:0" {
		t.Fatal("matchMedia admitted unbounded queries and listeners")
	}
}

func TestWindowResizeAndMediaChangesUseWindowEventPath(t *testing.T) {
	doc := domtest.Parse("<html><body></body></html>")
	r, err := js.New(js.Options{URL: "https://example.test/viewport", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	r.SetViewportRect(0, 0, 320, 240)
	if err := r.Run(`
		globalThis.resizeEvents = [];
		matchMedia('(max-width: 350px)').addEventListener('change', function(event) {
			resizeEvents.push('change:' + event.matches);
		});
		addEventListener('resize', function(event) {
			resizeEvents.push('resize:' + innerWidth + 'x' + innerHeight + ':' +
				(event.target === window) + ':' + (event.currentTarget === window) + ':' + event.isTrusted);
		});
	`, "resize-events.js"); err != nil {
		t.Fatal(err)
	}
	r.SetViewportRect(0, 0, 400, 300)
	r.DispatchWindowResize()
	got, err := r.Evaluate(`resizeEvents.join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "resize:400x300:true:true:true|change:false"; got != want {
		t.Fatalf("window viewport events = %q, want %q", got, want)
	}
}

func TestDevicePixelRatioUpdatesResolutionQueries(t *testing.T) {
	r := viewportRuntime(t, js.Options{URL: "https://example.test/viewport"})
	if err := r.Run(`
		globalThis.resolutionEvents = [];
		matchMedia('(min-resolution: 2dppx)').addEventListener('change', function(event) {
			resolutionEvents.push(event.matches);
		});
	`, "resolution.js"); err != nil {
		t.Fatal(err)
	}
	r.SetDevicePixelRatio(2)
	r.Tick()
	got, err := r.Evaluate(`matchMedia('(min-resolution: 2dppx)').matches + ':' + resolutionEvents.join(',')`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "true:true"; got != want {
		t.Fatalf("resolution query after DPR change = %q, want %q", got, want)
	}
}

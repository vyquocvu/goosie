package js_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// IntersectionObserver checks and the document load path.
//
// GOOSIE-IOEXPORT-001: `CheckIntersections` used to recover the observed
// element's node ID by exporting the target wrapper while holding the runtime
// mutex. A node wrapper carries enumerable `parentNode`/`shadowRoot` getters
// (`internal/js/js.go:1806-1820`, `internal/js/shadow_dom.go:22-33`) that call
// back into the runtime, and `jsGetShadowRoot` locks the same mutex. Go mutexes
// are not reentrant, so on the integrator's 2026-10-26 measurement the minimal
// page with one observer never exited in 20 s while its no-script control
// finished in 0.18 s, and `https://getbootstrap.com/docs/5.3/getting-started/`
// plus `https://vietnamnet.vn/` both timed out at 120.01 s (rc 124) in sweep #9
// after rendering in 0.54 s and 2.21 s in sweep #8. The hog process was idle,
// not slow: 0.36 s of CPU in 60 s. These tests are the liveness and payload
// oracle for the fix.

// intersectionWatchdog is how long a call gets before the test declares it
// deadlocked. It is a liveness watchdog, not a performance budget: a correct
// build returns in single-digit milliseconds (measured: the whole fixture below
// loads in ~1 ms on this host), and 10 s is three orders of magnitude of head
// room so that a loaded CI machine or a race-detector build cannot flip it. It
// is deliberately not tuned to the observed time, because a number close to the
// measurement would grade speed on machines it was never measured on. The
// invariant under test is "returns at all", and a deadlock never returns.
const intersectionWatchdog = 10 * time.Second

// ioPage holds one element inside the viewport and one far below it. The
// off-screen one is not `display:none` and not `visibility:hidden`: it has a box
// and a rect, it simply does not intersect, so "does not fire" is decided by the
// intersection ratio rather than by missing geometry.
//
// The observer script is inline in the page so the engine's load path runs it
// exactly as a real page's would; the unit oracles run the same text through
// rt.Run against the same parsed document, so both paths pin one payload.
const ioPage = `<!DOCTYPE html><html><head><title>io</title></head>
<body style="margin:0">
<div id="seen" style="width:100px;height:100px;background:#04f"></div>
<div id="off" style="position:absolute;top:400px;left:0;width:100px;height:100px;background:#f40"></div>
<script>` + ioObserverJS + `</script>
</body></html>`

// ioObserverJS registers the observer the page would register and records every
// callback as JSON. `target === window.__seen` is the identity check: the entry
// must carry the very object the page passed to observe(), not a copy and not
// a fresh wrapper for the same node.
const ioObserverJS = `
window.__seen = document.getElementById("seen");
window.__off = document.getElementById("off");
window.__log = [];
window.__observer = new IntersectionObserver(function (entries) {
	window.__log.push({
		count: entries.length,
		seen: entries[0].target === window.__seen,
		off: entries[0].target === window.__off,
		intersecting: entries[0].isIntersecting
	});
	document.title = "io " + entries.length;
});
window.__observer.observe(window.__seen);
window.__observer.observe(window.__off);
`

// runUnderWatchdog runs fn on its own goroutine and fails the test if it has not
// returned within intersectionWatchdog. Everything that touches the runtime has
// to happen inside fn: a deadlocked runtime holds its mutex forever, so a
// follow-up call from the test goroutine (Close, Evaluate, even Global) would
// hang the audit instead of reporting it, and the package would die on `go
// test`'s own timeout with no test named. That is the failure mode the watchdog
// exists to prevent, so fn owns the runtime end to end and hands back plain data.
func runUnderWatchdog(t *testing.T, what string, fn func() (string, error)) string {
	t.Helper()

	type outcome struct {
		value string
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		value, err := fn()
		done <- outcome{value: value, err: err}
	}()

	select {
	case o := <-done:
		if o.err != nil {
			t.Fatalf("%s: %v", what, o.err)
		}
		return o.value
	case <-time.After(intersectionWatchdog):
		t.Fatalf("%s did not return within %v: it is deadlocked, not slow (GOOSIE-IOEXPORT-001). "+
			"The check must not call Export() or any property getter on a node wrapper while "+
			"holding the runtime mutex.", what, intersectionWatchdog)
		return ""
	}
}

// TestIntersectionObserverCheckReturns is the primary oracle: checking an
// observer whose target is a real element must return, and must deliver exactly
// one entry for the visible element, carrying the observed object itself.
//
// Measured on this host before the fix: the check never returned (SIGQUIT dump
// showed CheckIntersections waiting in jsGetShadowRoot through Object.Export);
// after the fix it returns in microseconds and the log below is exact. A second
// check must not fire again - the entry's state did not change - which also
// stops the test from passing on a build that fires unconditionally.
// qc:BR-234
func TestIntersectionObserverCheckReturns(t *testing.T) {
	out := runUnderWatchdog(t, "CheckIntersections on a page with one observer", func() (string, error) {
		doc := domtest.Parse(ioPage)
		seen, off := doc.ElementByID("seen"), doc.ElementByID("off")
		if seen == nil || off == nil {
			return "", fmt.Errorf("fixture did not parse the two targets")
		}

		rt, err := js.New(js.Options{URL: "https://example.test/io", DOM: doc})
		if err != nil {
			return "", fmt.Errorf("js.New: %w", err)
		}
		if err := rt.Run(ioObserverJS, "io-page.js"); err != nil {
			return "", fmt.Errorf("Run: %w", err)
		}

		// The engine's own units: 200x150 CSS px viewport, #seen at the origin,
		// #off at y=400. Rects are keyed by dom.Node ID, which is what
		// SetElementRect takes and what the engine passes after layout.
		rt.SetViewportRect(0, 0, 200, 150)
		rt.SetElementRect(int(seen.ID), 0, 0, 100, 100)
		rt.SetElementRect(int(off.ID), 0, 400, 100, 100)
		rt.CheckIntersections()
		rt.CheckIntersections()

		got, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		if err := rt.Close(); err != nil {
			return "", fmt.Errorf("Close: %w", err)
		}
		return got, nil
	})

	const want = `[{"count":1,"seen":true,"off":false,"intersecting":true}]`
	if out != want {
		t.Fatalf("observer log = %s, want %s\n"+
			"one callback with one entry for the visible element, the entry's target identical to "+
			"the object passed to observe(), isIntersecting true, and no second event on the re-check", out, want)
	}
}

// TestIntersectionObserverDoesNotFireForOffscreenTarget is the negative half:
// an element with a real box below the fold must not fire. Without this, an
// implementation that fired every observed entry on every check would satisfy
// the positive test above and pass.
// qc:BR-234
func TestIntersectionObserverDoesNotFireForOffscreenTarget(t *testing.T) {
	out := runUnderWatchdog(t, "CheckIntersections on an off-screen target", func() (string, error) {
		doc := domtest.Parse(ioPage)
		off := doc.ElementByID("off")
		if off == nil {
			return "", fmt.Errorf("fixture did not parse #off")
		}

		rt, err := js.New(js.Options{URL: "https://example.test/io", DOM: doc})
		if err != nil {
			return "", fmt.Errorf("js.New: %w", err)
		}
		if err := rt.Run(`
			window.__off = document.getElementById("off");
			window.__log = [];
			new IntersectionObserver(function (entries) {
				window.__log.push(entries.length);
			}).observe(window.__off);
		`, "io-page.js"); err != nil {
			return "", fmt.Errorf("Run: %w", err)
		}

		rt.SetViewportRect(0, 0, 200, 150)
		rt.SetElementRect(int(off.ID), 0, 400, 100, 100)
		rt.CheckIntersections()

		got, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		if err := rt.Close(); err != nil {
			return "", fmt.Errorf("Close: %w", err)
		}
		return got, nil
	})

	if want := `[]`; out != want {
		t.Fatalf("observer log = %s, want %s: an element entirely below the viewport must not intersect", out, want)
	}
}

// TestIntersectionObserverObserveOfPlainObjectNeverFires pins the decision the
// fix made about a non-node argument. Mutation and Resize observers throw a
// TypeError there; this one has always treated observe(plainObject) as a no-op,
// and test/js/observer_raf_test.go asserts that shape ("Should not throw"), so
// the fix records "no node id" instead of throwing. The callback must still
// never run - there is no geometry to intersect.
// qc:BR-234
func TestIntersectionObserverObserveOfPlainObjectNeverFires(t *testing.T) {
	out := runUnderWatchdog(t, "CheckIntersections after observe(plainObject)", func() (string, error) {
		rt, err := js.New(js.Options{URL: "https://example.test/io"})
		if err != nil {
			return "", fmt.Errorf("js.New: %w", err)
		}
		if err := rt.Run(`
			window.__log = [];
			var observer = new IntersectionObserver(function (entries) {
				window.__log.push(entries.length);
			});
			observer.observe({});
			observer.observe({ width: 100, height: 100 });
		`, "io-page.js"); err != nil {
			return "", fmt.Errorf("Run: %w", err)
		}

		rt.SetViewportRect(0, 0, 200, 150)
		rt.CheckIntersections()

		got, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		if err := rt.Close(); err != nil {
			return "", fmt.Errorf("Close: %w", err)
		}
		return got, nil
	})

	if want := `[]`; out != want {
		t.Fatalf("observer log = %s, want %s: a plain object is not a node and has no geometry", out, want)
	}
}

// TestNewSessionWithIntersectionObserverReturns is the integration oracle: the
// deadlock was reached through Session.Reflow, so the real load path - engine
// parse, layout, and the observer check the reflow runs - must complete and must
// have delivered the callback that the page's own script relied on.
//
// Measured through this path before the fix: NewSession never returned (the
// integrator's bin/t50-iohang.html never exited in 20 s; a SIGQUIT dump put
// goroutine 1 in js.(*Runtime).jsGetShadowRoot under Object.Export under
// CheckIntersections under engine.(*Session).Reflow under engine.NewSession).
// After the fix it returns and the page's title is the one the callback set.
// qc:BR-234 BR-235
func TestNewSessionWithIntersectionObserverReturns(t *testing.T) {
	out := runUnderWatchdog(t, "engine.NewSession with an IntersectionObserver", func() (string, error) {
		rt, err := js.New(js.Options{URL: "https://example.test/io", Title: "io"})
		if err != nil {
			return "", fmt.Errorf("js.New: %w", err)
		}
		sess, err := engine.NewSession(ioPage, nil, 200, engine.WithViewportH(150), engine.WithJS(rt))
		if err != nil {
			if sess != nil {
				sess.Close()
			}
			return "", fmt.Errorf("NewSession: %w", err)
		}
		log, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			sess.Close()
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		// The page's callback sets the title, so a load that swallowed the
		// callback instead of running it cannot look healthy here.
		title := sess.JSTitle()
		sess.Close()
		return title + "|" + log, nil
	})

	y := strings.SplitN(out, "|", 2)
	if len(y) != 2 {
		t.Fatalf("load = %q, want a title and a log joined by %q", out, "|")
	}
	// The title is the one the callback wrote. Everything else about the payload
	// is the same as the unit oracle, which proves the reflow path delivered it.
	if want := "io 1"; y[0] != want {
		t.Fatalf("document title after load = %q, want %q: the observer callback must have run "+
			"during the reflow NewSession performs", y[0], want)
	}
	if want := `[{"count":1,"seen":true,"off":false,"intersecting":true}]`; y[1] != want {
		t.Fatalf("observer log = %s, want %s: the load must deliver one entry for the visible "+
			"element, carrying the object the page observed", y[1], want)
	}
}

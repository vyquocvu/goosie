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

const intersectionWatchdog = 10 * time.Second

const ioPage = `<!DOCTYPE html><html><head><title>io</title></head>
<body style="margin:0">
<div id="seen" style="width:100px;height:100px;background:#04f"></div>
<div id="off" style="position:absolute;top:400px;left:0;width:100px;height:100px;background:#f40"></div>
<script>` + ioObserverJS + `</script>
</body></html>`

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

		rt.SetViewportRect(0, 0, 200, 150)
		rt.SetElementRect(int(seen.ID), 0, 0, 100, 100)
		rt.SetElementRect(int(off.ID), 0, 400, 100, 100)
		rt.CheckIntersections()
		rt.CheckIntersections()
		rt.Tick()

		got, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		if err := rt.Close(); err != nil {
			return "", fmt.Errorf("Close: %w", err)
		}
		return got, nil
	})

	const want = `[{"count":2,"seen":true,"off":false,"intersecting":true}]`
	if out != want {
		t.Fatalf("observer log = %s, want %s\n"+
			"one callback with one entry for the visible element, the entry's target identical to "+
			"the object passed to observe(), isIntersecting true, and no second event on the re-check", out, want)
	}
}

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
		rt.Tick()

		got, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		if err := rt.Close(); err != nil {
			return "", fmt.Errorf("Close: %w", err)
		}
		return got, nil
	})

	if want := `[1]`; out != want {
		t.Fatalf("observer log = %s, want %s: an element entirely below the viewport must receive an initial non-intersecting entry", out, want)
	}
}

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
		sess.PumpJS(200)
		log, err := rt.Evaluate("JSON.stringify(window.__log)")
		if err != nil {
			sess.Close()
			return "", fmt.Errorf("Evaluate: %w", err)
		}
		title := sess.JSTitle()
		sess.Close()
		return title + "|" + log, nil
	})

	y := strings.SplitN(out, "|", 2)
	if len(y) != 2 {
		t.Fatalf("load = %q, want a title and a log joined by %q", out, "|")
	}
	if want := "io 2"; y[0] != want {
		t.Fatalf("document title after load = %q, want %q: the observer callback must have run "+
			"during the reflow NewSession performs", y[0], want)
	}
	if want := `[{"count":2,"seen":true,"off":false,"intersecting":true}]`; y[1] != want {
		t.Fatalf("observer log = %s, want %s: the load must deliver one entry for the visible "+
			"element, carrying the object the page observed", y[1], want)
	}
}

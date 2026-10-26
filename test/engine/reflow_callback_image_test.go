package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// An IntersectionObserver callback runs synchronously from inside Reflow, and
// Reflow used to hold imgMu for its whole body. A callback that assigns
// img.src therefore re-entered the loader on the same goroutine and blocked
// forever on the mutex it already held: https://vneconomy.vn/ never returned
// from NewSession in sweep #13 (SIGQUIT: beginScriptImage in sync.Mutex.Lock,
// called from Reflow at engine.go).
//
// The invariant is "a Reflow must not dispatch JS while holding a lock the
// JS can re-enter". The oracle is liveness under a watchdog, plus the payload:
// the image must actually decode, so a fix that drops the load instead of
// releasing the lock does not pass.
//
// The observer is registered inline so the real load path (parse -> scripts ->
// RestyleAndReflow -> CheckIntersections) runs exactly as on a live page.
const reflowCallbackWatchdog = 10 * time.Second

// qc:BR-256
func TestIntersectionCallbackMayStartAnImageLoad(t *testing.T) {
	png := pngDataURL(palettedPNG(t, 4, 4))
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="seen" style="width:100px;height:100px;background:#04f"></div>
<img id="hero" style="width:4px;height:4px">
<script>
window.__fired = 0;
new IntersectionObserver(function (entries) {
  if (!entries[0].isIntersecting) { return; }
  window.__fired += 1;
  document.getElementById('hero').src = '` + png + `';
}).observe(document.getElementById('seen'));
</script>
</body></html>`

	rt, err := js.New(js.Options{URL: "https://example.test/io-image"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}

	type outcome struct {
		sess *engine.Session
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		sess, err := engine.NewSession(html, nil, 800,
			engine.WithJS(rt), engine.WithViewportH(600),
			engine.WithMetrics(&fixedMetrics{advance: 7}))
		done <- outcome{sess, err}
	}()

	var got outcome
	select {
	case got = <-done:
	case <-time.After(reflowCallbackWatchdog):
		t.Fatalf("NewSession did not return within %v with an observer callback that sets img.src: "+
			"the Reflow goroutine is deadlocked on a lock the loader re-enters. "+
			"Reflow must release imgMu before CheckIntersections dispatches page JS.", reflowCallbackWatchdog)
	}
	if got.err != nil {
		t.Fatalf("NewSession: %v", got.err)
	}
	defer got.sess.Close()

	if fired := engineComputed(t, rt, "window.__fired"); fired != "1" {
		t.Errorf("observer fired %s times, want 1", fired)
	}
	if src := engineComputed(t, rt, "document.getElementById('hero').getAttribute('src')"); src != png {
		t.Errorf("hero src was not assigned by the callback")
	}

	// The data: source loads inline, but js settles the element in a microtask,
	// so the result becomes visible on the next pump. This is the positive half:
	// the load must complete, not merely be attempted.
	got.sess.PumpJS(800)
	if got := engineComputed(t, rt, "document.getElementById('hero').naturalWidth + 'x' + document.getElementById('hero').naturalHeight"); got != "4x4" {
		t.Errorf("hero natural size after the callback's src assignment = %s, want 4x4", got)
	}
}

package engine_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func startupPage(body string) string {
	return `<!doctype html><html><body><div id="d" style="display:none;width:200px;height:100px;background:#c80000"></div><script>` + body + `</script></body></html>`
}

func revealSession(t *testing.T, body string) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	sess, err := engine.NewSession(startupPage(body), nil, 800, engine.WithJS(rt), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess
}

func startupInt(t *testing.T, sess *engine.Session, expr string) int {
	t.Helper()
	got, err := sess.EvaluateJS(expr)
	if err != nil {
		t.Fatalf("EvaluateJS(%s): %v", expr, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(got))
	if err != nil {
		t.Fatalf("EvaluateJS(%s) = %q, want an integer", expr, got)
	}
	return n
}

// qc:BR-283
func TestStartupTimerIsPendingLoadWork(t *testing.T) {
	sess := revealSession(t, "setTimeout(function(){document.getElementById('d').style.display='block'}, 250);")
	due, ok := sess.PendingLoadTimerDelay()
	if !ok {
		t.Fatal("no pending load timer, want one queued 250ms out")
	}
	if due <= 0 || due > 250*time.Millisecond {
		t.Fatalf("pending startup timer due in %v, want (0, 250ms]", due)
	}
}

// qc:BR-283
func TestZeroDelayStartupTaskIsDueNow(t *testing.T) {
	sess := revealSession(t, "setTimeout(function(){document.getElementById('d').style.display='block'}, 0);")
	due, ok := sess.PendingLoadTimerDelay()
	if !ok {
		t.Fatal("a zero-delay startup task reported nothing pending")
	}
	if due > 5*time.Millisecond {
		t.Fatalf("zero-delay startup task due in %v, want already due", due)
	}
}

// qc:BR-283
func TestTimerQueuedAfterLoadIsNotStartupWork(t *testing.T) {
	sess := revealSession(t, "var x=1;")
	if _, err := sess.EvaluateJS("setTimeout(function(){}, 40)"); err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	if _, ok := sess.PendingLoadTimerDelay(); ok {
		t.Fatal("a timer queued after the load step reported startup work")
	}
}

// qc:BR-283
func TestStartupIntervalCountsOnceBeforeItsFirstFire(t *testing.T) {
	sess := revealSession(t, "setInterval(function(){document.getElementById('d').style.display='block';}, 10);")
	if _, ok := sess.PendingLoadTimerDelay(); !ok {
		t.Fatal("a startup interval reported no pending load work before its first fire")
	}
	time.Sleep(20 * time.Millisecond)
	sess.PumpJS(800)
	if _, ok := sess.PendingLoadTimerDelay(); ok {
		t.Fatal("the interval still counted as startup work after its first fire")
	}
}

// qc:BR-283
func TestSessionWithoutJSOwesNothing(t *testing.T) {
	sess, err := engine.NewSession(startupPage(""), nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	if _, ok := sess.PendingLoadTimerDelay(); ok {
		t.Fatal("a session with no runtime owed pending load work")
	}
}

// qc:BR-283
func TestPendingStartupWorkIsTheRevealThePagePaintsAfter(t *testing.T) {
	sess := revealSession(t, "window.n=0; setTimeout(function(){window.n=1; document.getElementById('d').style.display='block';}, 20);")
	if _, ok := sess.PendingLoadTimerDelay(); !ok {
		t.Fatal("nothing pending at load")
	}
	if got := startupInt(t, sess, "window.n"); got != 0 {
		t.Fatalf("window.n = %d before the startup timer ran, want 0", got)
	}
	time.Sleep(30 * time.Millisecond)
	sess.PumpJS(800)
	if _, ok := sess.PendingLoadTimerDelay(); ok {
		t.Fatal("the fired timer still counted as startup work")
	}
	if got := startupInt(t, sess, "window.n"); got != 1 {
		t.Fatalf("window.n = %d after the pump, want 1", got)
	}
}

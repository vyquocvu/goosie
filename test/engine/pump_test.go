package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func pumpSession(t *testing.T, html string, withJS bool) (*engine.Session, *js.Runtime, *engine.Invalidation) {
	t.Helper()
	var rt *js.Runtime
	var inv *engine.Invalidation
	opts := []engine.Option{engine.WithMetrics(&fixedMetrics{advance: 7})}
	if withJS {
		var err error
		rt, err = js.New(js.Options{URL: "https://example.com/page"})
		if err != nil {
			t.Fatalf("js.New: %v", err)
		}
		inv = &engine.Invalidation{}
		opts = append(opts, engine.WithJS(rt), engine.WithInvalidation(inv))
	}
	sess, err := engine.NewSession(html, nil, 800, opts...)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, rt, inv
}

// TestPumpJSFiresTimers pins the event-loop turn: a 0ms timer scheduled by
// a classic script must not run during load, and must have run (with its DOM
// mutation visible) after one pump.
func TestPumpJSFiresTimers(t *testing.T) {
	html := `<html><body><p id="t">waiting</p><script>setTimeout(function(){document.getElementById('t').textContent='fired';},0);</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	if got := sess.Find("fired"); len(got) != 0 {
		t.Fatal("timer callback ran during load without any pump")
	}
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work for a due timer")
	}
	if got := sess.Find("fired"); len(got) == 0 {
		t.Fatal("timer mutation not visible after PumpJS")
	}
	if sess.PumpJS(800) {
		t.Fatal("second PumpJS reported work on a quiescent document")
	}
}

func TestPumpJSSameValueDOMWritesSkipPipeline(t *testing.T) {
	html := `<html><body>
<div id="same" class="stable">same</div>
<input id="check" type="checkbox" value="fixed">
<script>setTimeout(function() {
  var node = document.getElementById('same');
  node.setAttribute('class', 'stable');
  node.classList.remove('absent');
  var check = document.getElementById('check');
  check.checked = false;
  check.value = 'fixed';
}, 0);</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	before := sess.WorkCounts()
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported visual work for same-value DOM writes")
	}
	if got := sess.WorkCounts(); got != before {
		t.Fatalf("work counts after no-op mutations = %+v, want unchanged %+v", got, before)
	}
}

func TestPumpJSChangedAttributeStillRebuildsStylesAndLayout(t *testing.T) {
	html := `<html><head><style>.active { color: red; width: 200px; }</style></head><body>
<p id="target">text</p>
<script>setTimeout(function() { document.getElementById('target').setAttribute('class', 'active'); }, 0);</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	before := sess.WorkCounts()
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS missed a changed class attribute")
	}
	got := sess.WorkCounts()
	if got.StyleResolves != before.StyleResolves+1 || got.LayoutBuilds != before.LayoutBuilds+1 {
		t.Fatalf("work counts after changed attribute = %+v, want one style/layout pass after %+v", got, before)
	}
	target := sess.Doc.ElementByID("target")
	if target == nil || sess.Styles[target.ID].Color.R != 255 {
		t.Fatal("changed class did not update computed style")
	}
}

// TestPumpJSRunsRAF pins the animation-frame half of the turn.
func TestPumpJSRunsRAF(t *testing.T) {
	html := `<html><body><p id="t">waiting</p><script>requestAnimationFrame(function(){document.getElementById('t').textContent='painted';});</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work for a pending RAF callback")
	}
	if got := sess.Find("painted"); len(got) == 0 {
		t.Fatal("RAF mutation not visible after PumpJS")
	}
}

// Canvas drawing is visible output even when its callback does not mutate the
// DOM, so PumpJS must request one repaint for the new recorded operations.
func TestPumpJSCanvasRAFRepaintsOnlyWhenDrawingChanges(t *testing.T) {
	html := `<html><body><canvas id="c" width="10" height="10"></canvas><script>
		var ctx = document.getElementById('c').getContext('2d');
		function tick(){requestAnimationFrame(tick);}
		requestAnimationFrame(function(){ctx.fillRect(0, 0, 5, 5); tick();});
	</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work for a canvas drawing callback")
	}
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported repaint work for a subsequent no-op RAF callback")
	}
}

// TestPumpJSWithoutRuntime is the headless/no-JS contract: no runtime, no
// work, no panic.
func TestPumpJSWithoutRuntime(t *testing.T) {
	sess, _, _ := pumpSession(t, `<html><body><p>x</p></body></html>`, false)
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported work with no runtime")
	}
}

// TestPumpJSClosedSession: pumping after Close must not panic or hang.
func TestPumpJSClosedSession(t *testing.T) {
	html := `<html><body><script>setTimeout(function(){},0);</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	sess.Close()
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported work on a closed session")
	}
}

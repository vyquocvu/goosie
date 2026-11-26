package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func observersRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	return rt
}

// qc:BR-117
func TestResizeObserverFollowsLayoutChanges(t *testing.T) {
	html := `<html><head><style>
		body { margin: 0; }
		#box { width: 100px; height: 40px; }
	</style></head><body>
		<div id="box"></div>
		<script>
			globalThis.sizes = [];
			var box = document.getElementById('box');
			var ro = new ResizeObserver(function(entries) {
				for (var i = 0; i < entries.length; i++) {
					globalThis.sizes.push(entries[i].contentRect.width + 'x' + entries[i].contentRect.height);
				}
			});
			ro.observe(box);
		</script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(observersRuntime(t)), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)

	sizes := func() string {
		t.Helper()
		sess.PumpJS(800)
		got, err := sess.EvaluateJS(`globalThis.sizes.join(',')`)
		if err != nil {
			t.Fatalf("read sizes: %v", err)
		}
		return got
	}

	if got, want := sizes(), "100x40"; got != want {
		t.Fatalf("initial ResizeObserver deliveries = %q, want %q", got, want)
	}

	if _, err := sess.EvaluateJS(`document.getElementById('box').style.width = '240px'`); err != nil {
		t.Fatalf("resize box: %v", err)
	}
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatalf("RestyleAndReflow: %v", err)
	}
	if got, want := sizes(), "100x40,240x40"; got != want {
		t.Fatalf("deliveries after width change = %q, want %q", got, want)
	}

	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatalf("second RestyleAndReflow: %v", err)
	}
	if got, want := sizes(), "100x40,240x40"; got != want {
		t.Fatalf("unchanged reflow added deliveries: %q, want %q", got, want)
	}
}

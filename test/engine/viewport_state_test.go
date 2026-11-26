package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestSessionResizeUpdatesViewportThenDispatchesWindowEvents(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/resize"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	html := `<!doctype html><html><body><script>
		globalThis.viewportEvents = [];
		globalThis.narrow = matchMedia('(max-width: 350px)');
		narrow.addEventListener('change', function(event) {
			viewportEvents.push('change:' + event.matches);
		});
		addEventListener('resize', function(event) {
			viewportEvents.push('listener:' + innerWidth + 'x' + innerHeight + ':' + (event.target === window));
		});
		onresize = function(event) {
			viewportEvents.push('handler:' + innerWidth + 'x' + innerHeight);
		};
	</script></body></html>`
	sess, err := engine.NewSession(html, nil, 320, engine.WithViewportH(240), engine.WithJS(r))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	sess.SetViewportSize(400, 300)
	if err := sess.Reflow(400); err != nil {
		t.Fatal(err)
	}
	sess.FireResizeEvent()
	sess.PumpJS(400)
	got, err := sess.EvaluateJS(`viewportEvents.join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "listener:400x300:true|handler:400x300|change:false"; got != want {
		t.Fatalf("session viewport events = %q, want %q", got, want)
	}
}

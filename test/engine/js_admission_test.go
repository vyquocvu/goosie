package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// jsFetchFetcher is a js.HTTPFetcher double for the engine-level fetch test. It
// returns one canned response so the test can assert the shared subresource
// budget is charged through the real Session wiring.
type jsFetchFetcher struct {
	resp *js.FetchResponse
}

func (f *jsFetchFetcher) Fetch(url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	return f.resp, nil
}

// TestEngineJSDeepNestingStoppedAtBoundary proves the engine wires the document
// mutation budget into a caller-supplied runtime BEFORE scripts run. A script
// that builds a chain deeper than MaxDocumentDepth must be stopped at the
// boundary with a catchable QuotaExceededError: the offending appendChild
// throws, the tree stays within the cap, and NewSession still succeeds. Without
// admission the flood is only caught later by walkDocument, which fails the
// whole document after the nodes are already allocated.
func TestEngineJSDeepNestingStoppedAtBoundary(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<html><body><script>` +
		`var cur = document.body; var reached = 0; var caught = '';` +
		`try {` +
		`  for (var i = 0; i < 400; i++) {` +
		`    var d = document.createElement('div');` +
		`    cur.appendChild(d); cur = d; reached++;` +
		`  }` +
		`} catch (e) { caught = (e && e.name) ? e.name : String(e); }` +
		`</script></body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatalf("NewSession failed: %v (admission must stop the flood at the boundary, not fail the document)", err)
	}
	defer sess.Close()

	if v, ok := rt.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("caught = %+v, want QuotaExceededError", v)
	}
	if d := jsAdmissionTreeDepth(sess.Doc); d > engine.MaxDocumentDepth {
		t.Errorf("tree depth = %d, want <= %d", d, engine.MaxDocumentDepth)
	}
	if n := sess.JSLiveNodeCount(); n <= 0 || n > engine.MaxDocumentNodes {
		t.Errorf("JSLiveNodeCount = %d, want in (0, %d]", n, engine.MaxDocumentNodes)
	}
	// The session must remain paintable: admission stops the flood, it does not
	// corrupt the document.
	if _, err := sess.PaintChecked(1); err != nil {
		t.Errorf("PaintChecked after admission: %v", err)
	}
}

// TestEngineJSFetchChargesSharedBudget proves the engine wires the JS network
// APIs onto the same subresource pool CSS, fonts and images use. A small fetch
// must succeed and charge its response body to Session.SubresourceBytesUsed.
func TestEngineJSFetchChargesSharedBudget(t *testing.T) {
	body := []byte("0123456789abcde") // 15 bytes
	rt, err := js.New(js.Options{
		URL:        "https://example.test/",
		HTTPClient: &jsFetchFetcher{resp: &js.FetchResponse{Status: 200, StatusText: "OK", Body: body, OK: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<html><body><script>` +
		`var st = 0;` +
		`fetch('/data').then(function(r){ st = r.status; }).catch(function(e){ st = -1; });` +
		`</script></body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	// The fetch runs on a goroutine; give it a moment, then drain callbacks on
	// the VM thread so the .then (and the budget charge that precedes it) land.
	time.Sleep(50 * time.Millisecond)
	rt.Tick()

	if v, ok := rt.Global("st"); !ok || v.Num != 200 {
		t.Errorf("fetch status = %+v, want 200 (a small fetch within budget must succeed)", v)
	}
	if used := sess.SubresourceBytesUsed(); used < int64(len(body)) {
		t.Errorf("SubresourceBytesUsed = %d, want >= %d (fetch body must be charged to the shared pool)", used, len(body))
	}
}

// jsAdmissionTreeDepth measures the deepest node under the document node (0).
func jsAdmissionTreeDepth(doc *dom.Document) int {
	if doc == nil {
		return 0
	}
	maxD := 0
	type ent struct {
		n *dom.Node
		d int
	}
	stack := []ent{{&doc.Node, 0}}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if e.d > maxD {
			maxD = e.d
		}
		if e.d > 1<<16 {
			return maxD
		}
		for c := e.n.FirstChild; c != nil; c = c.NextSibling {
			stack = append(stack, ent{c, e.d + 1})
		}
	}
	return maxD
}

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
// qc:BR-210
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
// qc:BR-210
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

// TestEngineJSCloneOfDeepDetachedSubtreeIsCheckedAtInsertion pins the route
// TASK-047 opened: `cloneNode(true)` is now reachable from script, so a page can
// build a chain that no gate ever saw. Reserve's contract is that a detached
// parent is neither charged nor depth-checked (the nodes are not reachable from
// the document yet - the same rule `createElement` follows), so the 140-step
// chain below is built for free and only the *insertion* of the copy is
// measured: the whole cloned subtree's height is charged against
// MaxDocumentDepth, the append throws a catchable QuotaExceededError, the
// document stays inside the cap, and the session still paints. A shallow clone
// is the control: one node, admitted immediately.
//
// Without the subtree depth check in the insert path this test would let a
// 140-deep copy into the live tree and the failure would surface later as
// walkDocument rejecting the document - after the allocation, which is exactly
// what the admission design in internal/engine/jsadmission.go refuses to allow.
// qc:BR-210
func TestEngineJSCloneOfDeepDetachedSubtreeIsCheckedAtInsertion(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<html><body><script>` +
		`var host = document.createElement('div'); var cur = host; var built = 0;` +
		`for (var i = 0; i < 140; i++) { var d = document.createElement('div'); cur.appendChild(d); cur = d; built++; }` +
		`var clone = host.cloneNode(true);` +
		`var cloneDepth = 0; var n = clone; while (n.firstChild) { n = n.firstChild; cloneDepth++; }` +
		`var caught = ''; var kidsBefore = document.body.childNodes.length;` +
		`try { document.body.appendChild(clone); } catch (e) { caught = (e && e.name) ? e.name : String(e); }` +
		`var rejectedClean = (document.body.childNodes.length === kidsBefore);` +
		`var shallowCaught = '';` +
		`try { document.body.appendChild(host.cloneNode(false)); } catch (e) { shallowCaught = (e && e.name) ? e.name : String(e); }` +
		`var grewByOne = (document.body.childNodes.length === kidsBefore + 1);` +
		// Phase 2 isolates the *second* gate: a subtree that is legal on its own
		// (99 levels, under the 128 cap) but does not fit at the depth it is
		// being inserted at. Only `pd+1+height > MaxDocumentDepth` can refuse
		// this one, so removing that comparison shows up here.
		`var cur2 = document.body;` +
		`for (var j = 0; j < 40; j++) { var pd2 = document.createElement('div'); cur2.appendChild(pd2); cur2 = pd2; }` +
		`var host2 = document.createElement('div'); var c2 = host2;` +
		`for (var k = 0; k < 99; k++) { var e2 = document.createElement('div'); c2.appendChild(e2); c2 = e2; }` +
		`var clone2 = host2.cloneNode(true); var kids2 = cur2.childNodes.length;` +
		`var caught2 = '';` +
		`try { cur2.appendChild(clone2); } catch (e) { caught2 = (e && e.name) ? e.name : String(e); }` +
		`var phase2Clean = (cur2.childNodes.length === kids2);` +
		`</script></body></html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatalf("NewSession failed: %v (the cloned subtree must be refused at insertion, not after it lands)", err)
	}
	defer sess.Close()

	if v, ok := rt.Global("built"); !ok || v.Num != 140 {
		t.Errorf("detached chain built = %+v, want 140 (a detached parent is not depth-checked)", v)
	}
	if v, ok := rt.Global("cloneDepth"); !ok || v.Num != 140 {
		t.Errorf("cloneDepth = %+v, want 140", v)
	}
	if v, ok := rt.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("deep clone insert: caught = %+v, want QuotaExceededError", v)
	}
	if v, ok := rt.Global("rejectedClean"); !ok || !v.Bool {
		t.Errorf("body child count changed by the rejected insert (%+v), want an untouched tree", v)
	}
	if v, ok := rt.Global("shallowCaught"); !ok || v.Str != "" {
		t.Errorf("shallow clone insert: caught = %q, want no error", v.Str)
	}
	if v, ok := rt.Global("grewByOne"); !ok || !v.Bool {
		t.Errorf("body child count after the shallow insert = %+v, want exactly one more", v)
	}
	if v, ok := rt.Global("caught2"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("clone inserted 40 levels down: caught = %+v, want QuotaExceededError "+
			"(the subtree is legal alone but not at this depth)", v)
	}
	if v, ok := rt.Global("phase2Clean"); !ok || !v.Bool {
		t.Errorf("the depth-rejected insert changed the parent's child count (%+v), want an untouched tree", v)
	}
	if d := jsAdmissionTreeDepth(sess.Doc); d > engine.MaxDocumentDepth {
		t.Errorf("tree depth = %d, want <= %d", d, engine.MaxDocumentDepth)
	}
	if _, err := sess.PaintChecked(1); err != nil {
		t.Errorf("PaintChecked after admission: %v", err)
	}
}

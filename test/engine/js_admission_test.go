package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

type jsFetchFetcher struct {
	resp *js.FetchResponse
}

func (f *jsFetchFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	return f.resp, nil
}

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
	if _, err := sess.PaintChecked(1); err != nil {
		t.Errorf("PaintChecked after admission: %v", err)
	}
}

// qc:BR-210
func TestEngineJSFetchChargesSharedBudget(t *testing.T) {
	body := []byte("0123456789abcde")
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

	time.Sleep(50 * time.Millisecond)
	rt.Tick()

	if v, ok := rt.Global("st"); !ok || v.Num != 200 {
		t.Errorf("fetch status = %+v, want 200 (a small fetch within budget must succeed)", v)
	}
	if used := sess.SubresourceBytesUsed(); used < int64(len(body)) {
		t.Errorf("SubresourceBytesUsed = %d, want >= %d (fetch body must be charged to the shared pool)", used, len(body))
	}
}

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

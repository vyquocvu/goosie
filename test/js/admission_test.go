package js_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Test doubles for the injected admission budgets.
//
// internal/js declares MutationBudget and SubresourceBudget so it never has to
// import internal/engine; the engine supplies the concrete implementations. The
// fakes below mirror that contract with SMALL caps so the JS wiring can be
// exercised without building a million-node document or a 128 MiB response. The
// engine-level test (test/engine/js_admission_test.go) proves the same wiring
// against the real MaxDocumentNodes / MaxDocumentDepth / maxSubresourceBytes.
// ---------------------------------------------------------------------------

var (
	errFakeNodes = errors.New("node limit exceeded")
	errFakeDepth = errors.New("tree depth limit exceeded")
)

// fakeMutationBudget is a js.MutationBudget double. A cap of zero disables that
// dimension. It charges on Reserve (only for children not already live under
// its root, so a move is free) and credits on Release, exactly like the engine
// budget, and rejects atomically without mutating its counter.
type fakeMutationBudget struct {
	mu       sync.Mutex
	root     *dom.Node
	maxNodes int
	maxDepth int
	live     int

	reserveCalls int
	releaseCalls int
}

func newFakeMutationBudget(doc *dom.Document) *fakeMutationBudget {
	return &fakeMutationBudget{
		root: &doc.Node,
		live: subtreeSize(&doc.Node),
	}
}

func (b *fakeMutationBudget) Reserve(parent *dom.Node, children []*dom.Node) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reserveCalls++
	add := 0
	for _, c := range children {
		if c == nil {
			continue
		}
		size, height := subtreeSize(c), subtreeHeight(c)
		if b.maxDepth > 0 && parent != nil {
			if pd := b.depthOf(parent); pd >= 0 && pd+1+height > b.maxDepth {
				return errFakeDepth
			}
		}
		if !b.IsLive(c) {
			add += size
		}
	}
	if b.maxNodes > 0 && b.live+add > b.maxNodes {
		return errFakeNodes
	}
	b.live += add
	return nil
}

func (b *fakeMutationBudget) Release(children []*dom.Node) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.releaseCalls++
	for _, c := range children {
		if c == nil {
			continue
		}
		b.live -= subtreeSize(c)
	}
	if b.live < 0 {
		b.live = 0
	}
}

func (b *fakeMutationBudget) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.live
}

func (b *fakeMutationBudget) IsLive(n *dom.Node) bool {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == b.root {
			return true
		}
	}
	return false
}

func (b *fakeMutationBudget) depthOf(n *dom.Node) int {
	d := 0
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == b.root {
			return d
		}
		d++
		if d > 1<<16 {
			return -1
		}
	}
	return -1
}

// subtreeSize counts a node's subtree iteratively, bounded so a malformed or
// cyclic tree cannot hang the test.
func subtreeSize(n *dom.Node) int {
	if n == nil {
		return 0
	}
	size := 0
	stack := []*dom.Node{n}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		size++
		if size > 1<<20 {
			return size
		}
		for ch := c.FirstChild; ch != nil; ch = ch.NextSibling {
			stack = append(stack, ch)
		}
	}
	return size
}

// subtreeHeight returns the number of edges from n to its deepest descendant.
func subtreeHeight(n *dom.Node) int {
	if n == nil {
		return 0
	}
	type ent struct {
		node *dom.Node
		d    int
	}
	maxH := 0
	stack := []ent{{n, 0}}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if e.d > maxH {
			maxH = e.d
		}
		if e.d > 1<<16 {
			return maxH
		}
		for ch := e.node.FirstChild; ch != nil; ch = ch.NextSibling {
			stack = append(stack, ent{ch, e.d + 1})
		}
	}
	return maxH
}

// fakeSubresourceBudget is a js.SubresourceBudget double with a small byte cap.
type fakeSubresourceBudget struct {
	mu   sync.Mutex
	cap  int
	used int
}

func (f *fakeSubresourceBudget) Take(n int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n < 0 || f.used+n > f.cap {
		return false
	}
	f.used += n
	return true
}

func (f *fakeSubresourceBudget) Used() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.used
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

// admissionDOM builds html>body under a properly constructed document node so
// depth/live accounting has a real root to walk to.
func admissionDOM() *dom.Document {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.HTML = html
	doc.Body = body
	return doc
}

func newAdmissionRuntime(t *testing.T, doc *dom.Document, b js.MutationBudget) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		r.SetMutationBudget(b)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// ---------------------------------------------------------------------------
// B1: appendChild node-count admission.
// ---------------------------------------------------------------------------

// TestAdmissionAppendChildRejectsPastNodeCap pins the core hole: a script that
// appends in a loop must be stopped at the document node cap with a catchable
// QuotaExceededError, the live count must never exceed the cap, and the runtime
// must stay usable afterward.
func TestAdmissionAppendChildRejectsPastNodeCap(t *testing.T) {
	doc := admissionDOM()
	b := newFakeMutationBudget(doc)
	b.maxNodes = b.count() + 3 // room for exactly three more nodes
	r := newAdmissionRuntime(t, doc, b)

	if err := r.Run(`
		var added = 0;
		var caught = '';
		var body = document.body;
		for (var i = 0; i < 10; i++) {
			try {
				body.appendChild(document.createElement('div'));
				added++;
			} catch (e) {
				caught = (e && e.name) ? e.name : String(e);
				break;
			}
		}
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("added"); !ok || v.Num != 3 {
		t.Errorf("added = %+v, want 3 (cap allowed exactly 3)", v)
	}
	if v, ok := r.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("caught = %+v, want QuotaExceededError", v)
	}
	if got := b.count(); got > b.maxNodes {
		t.Errorf("live nodes = %d, want <= %d", got, b.maxNodes)
	}

	// The runtime is still usable: queries and reads work after a rejection.
	if err := r.Run(`var divCount = document.getElementsByTagName('div').length;`, "inline"); err != nil {
		t.Fatalf("runtime unusable after admission reject: %v", err)
	}
	if v, ok := r.Global("divCount"); !ok || v.Num != 3 {
		t.Errorf("divCount = %+v, want 3", v)
	}
}

// TestAdmissionAppendChildRejectsPastDepthCap pins deep-nesting admission: a
// chain built one appendChild at a time must stop at the depth cap, and the
// resulting tree must never exceed it.
func TestAdmissionAppendChildRejectsPastDepthCap(t *testing.T) {
	doc := admissionDOM()
	b := newFakeMutationBudget(doc)
	b.maxDepth = 6 // document=0, html=1, body=2 -> divs may reach depth 6
	r := newAdmissionRuntime(t, doc, b)

	if err := r.Run(`
		var reached = 0;
		var caught = '';
		var cur = document.body;
		for (var i = 0; i < 50; i++) {
			try {
				var d = document.createElement('div');
				cur.appendChild(d);
				cur = d;
				reached++;
			} catch (e) {
				caught = (e && e.name) ? e.name : String(e);
				break;
			}
		}
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// body is depth 2; divs land at 3,4,5,6 then depth 7 is refused -> 4 divs.
	if v, ok := r.Global("reached"); !ok || v.Num != 4 {
		t.Errorf("reached = %+v, want 4", v)
	}
	if v, ok := r.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("caught = %+v, want QuotaExceededError", v)
	}
	if d := treeMaxDepth(doc); d > b.maxDepth {
		t.Errorf("tree depth = %d, want <= %d", d, b.maxDepth)
	}
}

// ---------------------------------------------------------------------------
// B2: repeated innerHTML cannot exceed the document cap in aggregate.
// ---------------------------------------------------------------------------

func TestAdmissionInnerHTMLAggregateCap(t *testing.T) {
	doc := admissionDOM()
	b := newFakeMutationBudget(doc)
	b.maxNodes = b.count() + 8
	r := newAdmissionRuntime(t, doc, b)

	if err := r.Run(`
		var done = 0;
		var caught = '';
		var c = document.body;
		for (var i = 0; i < 50; i++) {
			try {
				c.innerHTML = c.innerHTML + '<i></i>';
				done++;
			} catch (e) {
				caught = (e && e.name) ? e.name : String(e);
				break;
			}
		}
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("caught = %+v, want QuotaExceededError (repeated innerHTML must hit the aggregate cap)", v)
	}
	if got := b.count(); got > b.maxNodes {
		t.Errorf("live nodes after innerHTML flood = %d, want <= %d", got, b.maxNodes)
	}
	// The per-call parse cap (1024) is not the point: the document cap is. The
	// flood must have been stopped well before 50 iterations.
	if v, ok := r.Global("done"); !ok || v.Num >= 50 {
		t.Errorf("done = %+v, want it stopped before 50", v)
	}
}

// ---------------------------------------------------------------------------
// Atomicity: a rejected insertion leaves the DOM unchanged.
// ---------------------------------------------------------------------------

func TestAdmissionInnerHTMLRejectIsAtomic(t *testing.T) {
	doc := admissionDOM()
	div := doc.NewElement("div")
	div.SetAttribute("id", "main")
	doc.Body.AppendChild(div)
	p := doc.NewElement("p")
	p.AppendChild(doc.NewText("hello"))
	div.AppendChild(p)

	b := newFakeMutationBudget(doc)
	b.maxNodes = b.count() // no room for anything new
	r := newAdmissionRuntime(t, doc, b)

	if err := r.Run(`
		var caught = '';
		var el = document.getElementById('main');
		try { el.innerHTML = '<span>new</span>'; } catch (e) { caught = (e && e.name) ? e.name : String(e); }
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("caught = %+v, want QuotaExceededError", v)
	}
	// Atomic: the existing <p>hello</p> must still be there, not cleared.
	main := doc.ElementByID("main")
	if main == nil {
		t.Fatal("main is gone")
	}
	if main.FirstChild == nil || main.FirstChild.Data != "p" {
		t.Errorf("first child = %+v, want the original <p> (innerHTML reject must not clear)", main.FirstChild)
	}
	if got := main.FirstChild.TextContent(); got != "hello" {
		t.Errorf("text = %q, want %q", got, "hello")
	}
}

func TestAdmissionAppendChildRejectIsAtomic(t *testing.T) {
	doc := admissionDOM()
	div := doc.NewElement("div")
	div.SetAttribute("id", "main")
	doc.Body.AppendChild(div)

	b := newFakeMutationBudget(doc)
	b.maxNodes = b.count()
	r := newAdmissionRuntime(t, doc, b)

	if err := r.Run(`
		var caught = '';
		var p = document.getElementById('main');
		var child = document.createElement('span');
		try { p.appendChild(child); } catch (e) { caught = (e && e.name) ? e.name : String(e); }
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("caught"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("caught = %+v, want QuotaExceededError", v)
	}
	main := doc.ElementByID("main")
	if main.FirstChild != nil {
		t.Errorf("main gained a child (%+v); appendChild reject must leave it unchanged", main.FirstChild)
	}
}

// ---------------------------------------------------------------------------
// Release: removal credits the budget so remove/append does not monotonically
// exhaust it.
// ---------------------------------------------------------------------------

func TestAdmissionReleaseOnRemove(t *testing.T) {
	doc := admissionDOM()
	b := newFakeMutationBudget(doc)
	b.maxNodes = b.count() + 2
	r := newAdmissionRuntime(t, doc, b)

	if err := r.Run(`
		var body = document.body;
		var a = document.createElement('div');
		var c = document.createElement('div');
		body.appendChild(a);
		body.appendChild(c);
		var third = '';
		try { body.appendChild(document.createElement('div')); } catch (e) { third = (e && e.name) ? e.name : String(e); }
		a.remove();
		var afterRemove = '';
		try { body.appendChild(document.createElement('span')); } catch (e) { afterRemove = (e && e.name) ? e.name : String(e); }
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("third"); !ok || v.Str != "QuotaExceededError" {
		t.Errorf("third = %+v, want QuotaExceededError (cap reached)", v)
	}
	if v, ok := r.Global("afterRemove"); !ok || v.Str != "" {
		t.Errorf("afterRemove = %+v, want '' (remove() must credit the budget)", v)
	}
	if b.releaseCalls == 0 {
		t.Error("Release was never called; removals do not credit the budget")
	}
}

// ---------------------------------------------------------------------------
// Backwards compatibility: no budget means ungated (existing tests rely on it).
// ---------------------------------------------------------------------------

func TestAdmissionNoBudgetIsUngated(t *testing.T) {
	doc := admissionDOM()
	r := newAdmissionRuntime(t, doc, nil) // no budget installed

	if err := r.Run(`
		var n = 0;
		for (var i = 0; i < 100; i++) { document.body.appendChild(document.createElement('div')); n++; }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("n"); !ok || v.Num != 100 {
		t.Errorf("n = %+v, want 100 (no budget must not gate)", v)
	}
}

// ---------------------------------------------------------------------------
// B3: fetch charges the shared subresource budget.
// ---------------------------------------------------------------------------

func TestFetchChargesSubresourceBudget(t *testing.T) {
	body := []byte("hello world") // 11 bytes
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/api": {Status: 200, StatusText: "OK", Body: body, OK: true},
		},
	}
	sub := &fakeSubresourceBudget{cap: 1024}
	r := newFetchRuntime(t, m)
	r.SetSubresourceBudget(sub)

	if err := r.Run(`
		var st = 0;
		fetch('/api').then(function(resp) { st = resp.status; }).catch(function(e) { st = -1; });
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("st"); !ok || v.Num != 200 {
		t.Errorf("status = %+v, want 200 (a small fetch within budget must succeed)", v)
	}
	if got := sub.Used(); got != len(body) {
		t.Errorf("budget used = %d, want %d (response body must be charged)", got, len(body))
	}
}

func TestFetchSubresourceBudgetExhaustedRejects(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 2000)
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/big": {Status: 200, StatusText: "OK", Body: body, OK: true},
		},
	}
	sub := &fakeSubresourceBudget{cap: 1024}
	r := newFetchRuntime(t, m)
	r.SetSubresourceBudget(sub)

	if err := r.Run(`
		var caught = '';
		fetch('/big').catch(function(e) { caught = e; });
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if v, ok := r.Global("caught"); !ok || v.Str == "" {
		t.Errorf("caught = %+v, want a non-empty rejection reason (fetch must fail cleanly, not resolve)", v)
	}
	if got := sub.Used(); got > sub.cap {
		t.Errorf("budget used = %d, want <= %d (a rejected response must not be charged)", got, sub.cap)
	}
}

// TestFetchSubresourceBudgetConcurrentIsRaceSafe drives several fetches through
// one shared budget from concurrent goroutines. Run under -race, it pins that
// the budget's accounting is synchronized (the engine's real budget is a mutex-
// guarded counter shared with CSS/fonts/images).
func TestFetchSubresourceBudgetConcurrentIsRaceSafe(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/a": {Status: 200, Body: []byte("aaaa"), OK: true},
		},
	}
	sub := &fakeSubresourceBudget{cap: 1 << 20}
	r := newFetchRuntime(t, m)
	r.SetSubresourceBudget(sub)

	if err := r.Run(`
		for (var i = 0; i < 6; i++) { fetch('/a').then(function(){}).catch(function(){}); }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	drainFetch(t, r)

	if got := sub.Used(); got <= 0 {
		t.Errorf("budget used = %d, want > 0 after concurrent fetches", got)
	}
}

// treeMaxDepth measures the deepest node in a document (document node = 0).
func treeMaxDepth(doc *dom.Document) int {
	maxD := 0
	stack := []struct {
		n *dom.Node
		d int
	}{{&doc.Node, 0}}
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
			stack = append(stack, struct {
				n *dom.Node
				d int
			}{c, e.d + 1})
		}
	}
	return maxD
}

package js

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

// selectorIndexFixture is the TASK-051 §1 document: html, head, body, div#o,
// p.a, span.s, p.b, p.c.
const selectorIndexFixture = `<!DOCTYPE html><html><head></head><body><div id="o" data-x="y"><p class="a">1</p><span class="s"><p class="b">2</p></span></div><p class="c">3</p></body></html>`

func newSelectorRuntime(t *testing.T) (*Runtime, *dom.Document) {
	t.Helper()
	doc := domtest.Parse(selectorIndexFixture)
	r, err := New(Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return r, doc
}

// TestSelectorFastPathPredicate is the §3.2 "tighten the predicate" proof: the
// index may answer exactly one compound holding exactly one tag, class, or id
// condition. Everything else must decline so the general walk runs. This is
// package js (white-box) because indexKind and querySelectorAllIndexed are
// unexported; the doc names this package.
// qc:BR-225
func TestSelectorFastPathPredicate(t *testing.T) {
	cases := []struct {
		sel  string
		kind byte
		val  string
	}{
		{"div", 'T', "div"},
		{"p", 'T', "p"},
		{".a", '.', "a"},
		{"#o", '#', "o"},
		// Uppercase in a tag name must keep the walk: the index keys parsed
		// documents by their stored lowercase tag, and this is isLowerTag's
		// original purpose. The parser lowercases the condition value, so the
		// check has to read the raw spelling.
		{"DIV", 0, ""},
		// The pre-fix fast path sent every one of these to the tag index,
		// where they found nothing and reported success.
		{"div > p", 0, ""},
		{"div span", 0, ""},
		{"p.a", 0, ""},
		{"div#o", 0, ""},
		{"[data-x]", 0, ""},
		{`[data-x="y"]`, 0, ""},
		{"p:first-child", 0, ""},
		{".a, .c", 0, ""},
		{"*", 0, ""},
		{":scope", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			p := compileSelectorPlan(c.sel)
			if p.syntaxErr != "" {
				t.Fatalf("%q was rejected as %q", c.sel, p.syntaxErr)
			}
			if p.indexKind != c.kind || p.indexVal != c.val {
				t.Fatalf("plan(%q) = kind %q val %q, want kind %q val %q",
					c.sel, p.indexKind, p.indexVal, c.kind, c.val)
			}
		})
	}
}

// TestSelectorComplexSelectorDeclinesIndex proves the second half of the
// requirement: a complex selector is declined by querySelectorAllIndexed and
// still answered, by the walk. Results are compared to the measured Chromium
// counts.
// qc:BR-226
func TestSelectorComplexSelectorDeclinesIndex(t *testing.T) {
	r, doc := newSelectorRuntime(t)
	root := &doc.Node

	complex := []string{"div > p", "p.a", "[data-x]", "p:first-child", ".a, .c", "*"}
	for _, sel := range complex {
		p := compileSelectorPlan(sel)
		if p.syntaxErr != "" {
			t.Fatalf("%q was rejected as %q", sel, p.syntaxErr)
		}
		if _, ok := r.querySelectorAllIndexed(root, p); ok {
			t.Fatalf("querySelectorAllIndexed answered complex selector %q instead of declining", sel)
		}
		if _, ok := r.querySelectorIndexed(root, p); ok {
			t.Fatalf("querySelectorIndexed answered complex selector %q instead of declining", sel)
		}
	}

	// The general path answers them, with the measured counts.
	want := map[string]int{
		"div > p":       1,
		"p.a":           1,
		"[data-x]":      1,
		"p:first-child": 2,
		".a, .c":        2,
		"*":             8,
	}
	for sel, n := range want {
		p := compileSelectorPlan(sel)
		if got := len(r.querySelectorAllPlan(root, p)); got != n {
			t.Errorf("walk %q = %d nodes, want %d", sel, got, n)
		}
		if got := r.querySelectorPlan(root, p); got == nil {
			t.Errorf("walk querySelector(%q) = nil, want a node", sel)
		}
	}

	// A simple selector still takes the index, and answers the same nodes.
	p := compileSelectorPlan("p")
	nodes, ok := r.querySelectorAllIndexed(root, p)
	if !ok {
		t.Fatalf("querySelectorAllIndexed declined the simple selector %q", "p")
	}
	if len(nodes) != 3 {
		t.Fatalf("indexed querySelectorAll(%q) = %d nodes, want 3", "p", len(nodes))
	}
	if walk := r.querySelectorAllPlan(root, p); len(walk) != len(nodes) {
		t.Fatalf("walk and index disagree: %d vs %d nodes", len(walk), len(nodes))
	}
}

// TestSelectorPlanCacheReusesAndBounds pins the two counter-shaped cache
// properties M05 and §3.2's "cache compiled selectors per document" require:
// the same string compiles once, and the cache never grows past its cap.
// qc:BR-227
func TestSelectorPlanCacheReusesAndBounds(t *testing.T) {
	r, _ := newSelectorRuntime(t)

	first := r.selectorPlanFor("div > p")
	again := r.selectorPlanFor("div > p")
	if first != again {
		t.Fatalf("selectorPlanFor compiled the same string twice")
	}
	if other := r.selectorPlanFor("div p"); other == first {
		t.Fatalf("selectorPlanFor returned one plan for two different strings")
	}

	for i := 0; i <= selectorCacheCap; i++ {
		r.selectorPlanFor(fmt.Sprintf(".c%d", i))
		if len(r.selectorPlans) > selectorCacheCap {
			t.Fatalf("plan cache holds %d entries, cap is %d", len(r.selectorPlans), selectorCacheCap)
		}
	}

	// An invalid selector is cached too: the page must not pay for re-parsing
	// the same bad string, and the error must survive reuse.
	bad := r.selectorPlanFor("p:not()")
	if bad.syntaxErr == "" {
		t.Fatalf("invalid selector produced no syntax error")
	}
	if r.selectorPlanFor("p:not()") != bad {
		t.Fatalf("invalid selector plan was not reused")
	}
}

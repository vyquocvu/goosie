package js

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

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
		{"DIV", 0, ""},
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

	bad := r.selectorPlanFor("p:not()")
	if bad.syntaxErr == "" {
		t.Fatalf("invalid selector produced no syntax error")
	}
	if r.selectorPlanFor("p:not()") != bad {
		t.Fatalf("invalid selector plan was not reused")
	}
}

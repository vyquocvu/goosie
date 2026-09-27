package dom_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// tightLimits sits far below the engine's 50000 nodes / depth 128 caps, which
// test/engine/bounds_test.go pins on its own: at these numbers the fuzzer crosses a
// bound after a handful of tokens, so a node that escapes the accounting is a finding
// rather than a needle in an 8 MB input. Nodes is deliberately small enough for a
// 40-token input to exhaust it: at 128 the corpus never reaches the node bound, and
// disabling that check leaves this file green.
var tightLimits = dom.ParseLimits{Nodes: 24, Depth: 8, Attributes: 4, AttributeBytes: 8}

// looseLimits dominates tightLimits in every field, which is what makes the second
// parse meaningful: any token the tight parse accepted passes the loose checks too, so
// the two parses agree up to the point where the tight one stopped, and everything the
// loose parse then builds is attributable to the bound that stopped it.
var looseLimits = dom.ParseLimits{Nodes: 4096, Depth: 64, Attributes: 32, AttributeBytes: 2048}

// Each input lands in exactly one of three buckets, and every bucket carries an
// assertion rather than an early return. TestParsePathCoverage measures the split on a
// fixed corpus, because it cannot be measured during a search: `-fuzz` runs the target in
// worker processes whose counters die with them (a 90 s search of 4,768,757 execs
// incremented in-process accounting zero times).
type bucket int

const (
	// parsed under the tight limits and walked against them
	boundChecked bucket = iota
	// refused tightly, built under looser limits, refusal traced to a bound
	refusalExplained
	// refused tightly for an attribute bound the dropped token never reached
	refusalDropped
	// refused under both limit sets, where the assertion thins to "both name a limit"
	refusedBoth
)

func FuzzParseDocument(f *testing.F) {
	f.Add(`<html><body><p>text`)
	f.Add(`<div><span><a href=x><em>nesting four levels deep here`)
	f.Add("</p></div><<a>>\x00<![CDATA[\x00]]>")
	f.Add(`<p class="a" id="b" title="c" lang="d" dir="e">x`)
	f.Add(`<table><tr><td><svg><foreignObject><p>foster parented`)
	f.Add("<br " + strings.Repeat("a='b' ", 20) + ">x")
	f.Add(strings.Repeat("<div>", 60) + "deep" + strings.Repeat("</div>", 60))
	f.Fuzz(func(t *testing.T, html string) {
		assertParse(t, html)
	})
}

// assertParse is the target's whole claim, and TestParsePathCoverage runs the same
// function so the deterministic corpus measures exactly what a search executes.
func assertParse(t *testing.T, html string) bucket {
	t.Helper()
	doc, err := dom.ParseBounded(html, tightLimits)
	if err == nil {
		reportStats(t, "tight", html, measure(doc), tightLimits)
		return boundChecked
	}
	if !isBoundRefusal(err) {
		t.Errorf("parse of %q refused with %q, which names no resource limit", clip(html), err)
		return refusedBoth
	}
	loose, looseErr := dom.ParseBounded(html, looseLimits)
	if looseErr != nil {
		if !isBoundRefusal(looseErr) {
			t.Errorf("parse of %q refused the loose limits with %q, which names no resource limit", clip(html), looseErr)
		}
		return refusedBoth
	}
	stats := measure(loose)
	reportStats(t, "loose", html, stats, looseLimits)
	if stats.exceeds(tightLimits) {
		return refusalExplained
	}
	// An attribute refusal can name a bound no retained node ever breaks, because the
	// tree builder drops tokens outright: `<0><BodY long…>` is a second body, whose
	// attributes are ignored with it - a finding the search turned up on 2026-09-27,
	// pinned under testdata/fuzz/. The bound still held in the sense that matters here:
	// those bytes were scanned and refused. Node and depth refusals have no such escape,
	// since a bound the parser hit must be present in the tree it stopped building.
	if isAttrRefusal(err) && !stats.exceeds(tightLimits) {
		return refusalDropped
	}
	t.Errorf("parse of %q refused the tight limits %v with %q, but the same input parses under %v as %s, which breaks none of the tight bounds",
		clip(html), tightLimits, err, looseLimits, stats)
	return refusalExplained
}

func isAttrRefusal(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "attribute count limit") || strings.Contains(msg, "attribute byte limit")
}

func reportStats(t *testing.T, which string, html string, stats treeStats, limits dom.ParseLimits) {
	t.Helper()
	if stats.cycled {
		t.Errorf("parse of %q built a document with a node cycle", clip(html))
		return
	}
	if stats.exceeds(limits) {
		t.Errorf("parse of %q built %s under limits %v", clip(html), stats, limits)
	}
}

func isBoundRefusal(err error) bool {
	return strings.Contains(err.Error(), "limit exceeded")
}

type treeStats struct {
	nodes, maxDepth, maxAttrs, maxAttrBytes int
	cycled                                  bool
}

// exceeds reports whether a built tree breaks any bound the limits impose. A tight
// refusal this cannot explain is not the accounting firing - it is the parser refusing
// for a reason it does not report.
func (s treeStats) exceeds(l dom.ParseLimits) bool {
	return s.nodes > l.Nodes || s.maxDepth > l.Depth || s.maxAttrs > l.Attributes || s.maxAttrBytes > l.AttributeBytes
}

// maxDepth counts edges from the document node, the same convention allowNode's parent
// chain and engine.walkDocument use.
func (s treeStats) String() string {
	return fmt.Sprintf("%d nodes at most %d deep with at most %d attributes and a %d-byte attribute",
		s.nodes, s.maxDepth, s.maxAttrs, s.maxAttrBytes)
}

func measure(doc *dom.Document) treeStats {
	var s treeStats
	seen := map[*dom.Node]bool{}
	var walk func(n *dom.Node, depth int)
	walk = func(n *dom.Node, depth int) {
		if n == nil {
			return
		}
		if seen[n] {
			s.cycled = true
			return
		}
		seen[n] = true
		s.nodes++
		if depth > s.maxDepth {
			s.maxDepth = depth
		}
		if len(n.Attr) > s.maxAttrs {
			s.maxAttrs = len(n.Attr)
		}
		for _, a := range n.Attr {
			if b := len(a.Name) + len(a.Value); b > s.maxAttrBytes {
				s.maxAttrBytes = b
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, depth+1)
		}
	}
	walk(&doc.Node, 0)
	return s
}

func clip(s string) string {
	if len(s) <= 120 {
		return s
	}
	return s[:120] + "…"
}

// corpusTokens are the shapes the tree builder branches on: elements that push and pop
// the open stack, void and raw-text elements, table and SVG foster parenting, and the
// malformed fragments a real document is full of.
var corpusTokens = []string{
	"<div>", "</div>", "<p>", "<main>", "<span>", "<em>", "<b>", "<blockquote>",
	`<a href="x">`, "</a>", "<table>", "<tr>", "<td>", "</td>", "</tr>", "</table>",
	"<svg>", "<foreignObject>", `<p class="a b">`, `<input value="v">`, "<br>",
	"<img src=x>", "<!-- c -->", "<!doctype html>", "text", " ", "\x00", "<", ">",
	"</>", "<<", "<x", `">`, "</p>", "&amp;", "<style>s{color:red}</style>",
	"<font face='f' size=2 color=red width=9 height=8>",
}

// syntheticCorpus is seeded rather than random: the coverage numbers in this file's
// log line have to mean the same corpus on every machine and every run.
func syntheticCorpus(n int) []string {
	r := rand.New(rand.NewSource(0))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		var b strings.Builder
		for t := 1 + r.Intn(40); t > 0; t-- {
			b.WriteString(corpusTokens[r.Intn(len(corpusTokens))])
		}
		out = append(out, b.String())
	}
	return out
}

// TestParsePathCoverage is the audit of this file's own claim. A fuzz target that
// returns early on refusals can report tens of millions of executions while checking
// the bounds on a minority of them, so the split is measured on a fixed corpus here —
// the only place it can be, since a search's executions happen in worker processes.
func TestParsePathCoverage(t *testing.T) {
	const inputs = 4000
	var checked, explained, dropped, refused int
	for _, in := range syntheticCorpus(inputs) {
		switch assertParse(t, in) {
		case boundChecked:
			checked++
		case refusalExplained:
			explained++
		case refusalDropped:
			dropped++
		case refusedBoth:
			refused++
		}
	}
	t.Logf("%d synthetic documents: %d checked against the tight bounds (%.1f%%), %d refusals explained by the loose parse (%.1f%%), %d attribute refusals on tokens the tree builder dropped (%.1f%%), %d refused under both limit sets (%.1f%%)",
		inputs, checked, 100*float64(checked)/inputs, explained, 100*float64(explained)/inputs,
		dropped, 100*float64(dropped)/inputs, refused, 100*float64(refused)/inputs)
	if checked == 0 || explained == 0 {
		t.Errorf("corpus reaches %d tight-bound checks and %d explained refusals: the target has become one-sided, so its execution count no longer means the bounds are being tested", checked, explained)
	}
	if dropped > inputs/20 {
		t.Errorf("%d of %d inputs are explained only by the dropped-token escape; that path is the weakest assertion in this file and it should stay rare", dropped, inputs)
	}
	if refused > inputs/10 {
		t.Errorf("%d of %d inputs are refused under both limit sets, where the assertion thins to \"both refusals name a limit\"; widen looseLimits", refused, inputs)
	}
}

package engine_test

import (
	"math"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-163
func TestSessionRejectsUnboundedInput(t *testing.T) {
	tests := []struct {
		name, html string
		css        []string
		width      float32
		want       string
	}{
		{name: "HTML bytes", html: strings.Repeat(" ", engine.MaxDocumentBytes+1), width: 400, want: "HTML"},
		{name: "nodes", html: strings.Repeat("<br>", engine.MaxDocumentNodes+1), width: 400, want: "node"},
		{name: "depth", html: strings.Repeat("<div>", 129), width: 400, want: "depth"},
		{name: "attributes", html: "<p " + strings.Repeat("a='b' ", 1000) + ">x", width: 400, want: "attribute"},
		{name: "CSS bytes", css: []string{strings.Repeat(" ", engine.MaxCSSBytes+1)}, width: 400, want: "CSS"},
		{name: "rules", css: []string{strings.Repeat("p{}", engine.MaxCSSRules+1)}, width: 400, want: "rule"},
		{name: "selector list", css: []string{strings.Repeat("p,", engine.MaxCSSSelectors) + "p{}"}, width: 400, want: "selector"},
		{name: "selector chain", css: []string{strings.Repeat("p > ", engine.MaxSelectorParts) + "p{}"}, width: 400, want: "selector"},
		{name: "recursive selector", css: []string{strings.Repeat(":not(", 20) + "p" + strings.Repeat(")", 20) + "{}"}, width: 400, want: "nesting"},
		{name: "font", html: `<p style="font-size:513px">x`, width: 400, want: "font"},
		{name: "geometry", html: `<p style="width:2097153px">x`, width: 400, want: "geometry"},
		{name: "aggregate geometry", html: `<div style="height:1200000px"></div><div style="height:1200000px"></div>`, width: 400, want: "geometry"},
		{name: "numeric overflow", html: `<p style="width:` + strings.Repeat("9", 400) + `px">x`, width: 400, want: "numeric"},
		{name: "NaN width", width: float32(math.NaN()), want: "viewport"},
		{name: "Inf width", width: float32(math.Inf(1)), want: "viewport"},
		{name: "huge width", width: math.MaxFloat32, want: "viewport"},
		{name: "zero width", width: 0, want: "viewport"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.NewSession(tt.html, tt.css, tt.width)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %q error, got %v", tt.want, err)
			}
		})
	}
}

// qc:BR-162
func TestSessionRejectsOversizedHTMLBeforeParsing(t *testing.T) {
	// The payload is also millions of nodes and far past the depth limit, so a
	// parser-first path would report one of those limits instead. Only a byte
	// check that runs before the tree builder can answer with the byte cap.
	html := strings.Repeat("<div>", engine.MaxDocumentBytes/5+1)
	if len(html) <= engine.MaxDocumentBytes {
		t.Fatalf("fixture = %d bytes, want > %d", len(html), engine.MaxDocumentBytes)
	}
	_, err := engine.NewSession(html, nil, 400)
	if err == nil {
		t.Fatal("oversized document accepted")
	}
	if !strings.Contains(err.Error(), "HTML byte limit exceeded") {
		t.Fatalf("error = %q, want the document byte cap", err)
	}
	if strings.Contains(err.Error(), "node") || strings.Contains(err.Error(), "depth") {
		t.Fatalf("error = %q, byte cap was not checked before the tree limits", err)
	}
}

// qc:BR-164
func TestSessionRejectsAdversarialSelectorShapes(t *testing.T) {
	// The selector-count and part-count caps are exercised by
	// TestSessionRejectsUnboundedInput. These are the two costs that those
	// shapes cannot reach: a single compound carrying every condition, and a
	// chain built only from descendant combinators. Both are cheap to write and
	// expensive to match, so they are refused while the sheet is admitted.
	tests := []struct {
		name string
		css  string
		want string
	}{
		{
			name: "condition count",
			css:  "p" + strings.Repeat(".a", engine.MaxSelectorConditions+1) + "{color:red}",
			want: "selector condition limit",
		},
		{
			name: "descendant searches",
			css:  strings.Repeat("p ", engine.MaxSelectorSearches+1) + "p{color:red}",
			want: "selector search limit",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.css) > engine.MaxCSSBytes {
				t.Fatalf("fixture is %d bytes, already past the CSS byte cap; it would prove the wrong limit", len(tc.css))
			}
			_, err := engine.NewSession(`<p class="a">x</p>`, []string{tc.css}, 400)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// qc:BR-163
func TestParserStopsAtNodeAndDepthLimits(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 8, Attributes: 8, AttributeBytes: 64}
	if _, err := dom.ParseBounded(strings.Repeat("<br>", limits.Nodes+1), limits); err == nil ||
		!strings.Contains(err.Error(), "node limit") {
		t.Fatalf("flat document: err = %v, want the node limit", err)
	}
	if _, err := dom.ParseBounded(strings.Repeat("<div>", limits.Depth+1), limits); err == nil ||
		!strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("nested document: err = %v, want the depth limit", err)
	}
}

// qc:BR-163
func TestSessionRejectsOverdeepProvidedTree(t *testing.T) {
	// A MIME-aware parser can hand over a tree the HTML parser never built, so
	// the depth bound has to be checked again before styling recurses into it.
	// The validator stops at the first node past the limit, which is what keeps
	// this from becoming a stack overflow on a deeply linked tree.
	doc := dom.NewDocument()
	parent := &doc.Node
	for i := 0; i < 2000; i++ {
		n := &dom.Node{ID: doc.AllocateNodeID(), Type: dom.NodeElement, Data: "div", Doc: doc}
		parent.AppendChild(n)
		parent = n
	}
	_, err := engine.NewSession("", nil, 400, engine.WithDocumentTree(doc))
	if err == nil || !strings.Contains(err.Error(), "document tree depth limit") {
		t.Fatalf("err = %v, want the pre-parsed tree admission limit", err)
	}
}

func TestSessionAcceptsRepeatedTextAndRawHTML(t *testing.T) {
	for _, html := range []string{
		`<p>` + strings.Repeat("normal repeated text ", 1000) + `</p>`,
		`<script>` + strings.Repeat("<div>", 10000) + `</script><p>hello</p>`,
		`<p title="<div><div>">hello</p>`,
		strings.Repeat("<p>text</p>", 1000),
	} {
		if _, err := engine.NewSession(html, nil, 800); err != nil {
			t.Fatal(err)
		}
	}
}

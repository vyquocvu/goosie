package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"golang.org/x/net/html"
)

// TestRenderParsed_Empty — RenderParsed with no external CSS behaves
// like RenderHTML for inline-only stylesheets.
func TestRenderParsed_Empty(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><head><style>body { color: red; }</style></head><body><p>hi</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	obj, err := r.RenderParsed(context.Background(), doc, nil)
	if err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	if obj == nil {
		t.Fatal("RenderParsed returned nil canvas object")
	}
}

// TestRenderParsed_InlineExternalMerged — RenderParsed merges inline
// and external CSS in source order. The rule count covers both.
func TestRenderParsed_InlineExternalMerged(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><head><style>.a { color: red; }</style></head><body><p class="a b">hi</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	external := []renderer.ExternalCSS{
		{URL: "https://example.com/b.css", Source: []byte(".b { color: blue; }")},
	}
	if _, err := r.RenderParsed(context.Background(), doc, external); err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	r.StylesheetMu().RLock()
	defer r.StylesheetMu().RUnlock()
	if r.Stylesheet() == nil {
		t.Fatal("stylesheet nil after RenderParsed")
	}
	if got := len(r.Stylesheet().Rules); got < 2 {
		t.Errorf("expected at least 2 rules (inline + external), got %d", got)
	}
}

// TestRenderParsed_ExternalSourceOrder — multiple external stylesheets
// are appended in the order provided. Order matters because later rules
// override earlier ones in CSS cascade.
func TestRenderParsed_ExternalSourceOrder(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><body><p>x</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	external := []renderer.ExternalCSS{
		{URL: "https://example.com/first.css", Source: []byte(".x { color: red; }")},
		{URL: "https://example.com/second.css", Source: []byte(".x { color: green; }")},
		{URL: "https://example.com/third.css", Source: []byte(".x { color: blue; }")},
	}
	if _, err := r.RenderParsed(context.Background(), doc, external); err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	r.StylesheetMu().RLock()
	defer r.StylesheetMu().RUnlock()
	// Last rule with selector .x should win cascade.
	var last string
	for _, rule := range r.Stylesheet().Rules {
		if hasClassSelector(rule, ".x") {
			for _, d := range rule.Declarations {
				if d.Property == "color" {
					last = d.Value
				}
			}
		}
	}
	if last != "blue" {
		t.Errorf("last color rule = %q, want blue (source-order: third wins)", last)
	}
}

// TestRenderParsed_BrokenExternalCSSSkipped — external CSS that fails
// to parse is silently skipped. The renderer does not error out; the
// remaining styles still apply.
func TestRenderParsed_BrokenExternalCSSSkipped(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><body><p>x</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	external := []renderer.ExternalCSS{
		{URL: "broken.css", Source: []byte("{{this is not css")},
		{URL: "good.css", Source: []byte(".x { color: red; }")},
	}
	if _, err := r.RenderParsed(context.Background(), doc, external); err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	r.StylesheetMu().RLock()
	defer r.StylesheetMu().RUnlock()
	if got := len(r.Stylesheet().Rules); got < 1 {
		t.Errorf("expected at least 1 rule from good.css, got %d", got)
	}
}

// TestRenderParsed_NonCSSBodySkipped — bodies that fail
// shouldAttemptParseExternalCSS (e.g. HTML 404 pages) are not parsed.
func TestRenderParsed_NonCSSBodySkipped(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><body><p>x</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	external := []renderer.ExternalCSS{
		{URL: "html404", Source: []byte(`<!DOCTYPE html><html><body>Not Found</body></html>`)},
	}
	if _, err := r.RenderParsed(context.Background(), doc, external); err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	r.StylesheetMu().RLock()
	defer r.StylesheetMu().RUnlock()
	if got := len(r.Stylesheet().Rules); got != 0 {
		t.Errorf("expected 0 rules from HTML body, got %d", got)
	}
}

// TestRenderParsed_NoExternalCSSWorks — RenderParsed with only inline
// CSS and nil external returns the same result as the inline-only path.
func TestRenderParsed_NoExternalCSSWorks(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><head><style>.a { color: red; }</style></head><body><p class="a">x</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	obj, err := r.RenderParsed(context.Background(), doc, nil)
	if err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	if obj == nil {
		t.Fatal("nil canvas object")
	}
	r.StylesheetMu().RLock()
	defer r.StylesheetMu().RUnlock()
	if got := len(r.Stylesheet().Rules); got != 1 {
		t.Errorf("inline-only rules = %d, want 1", got)
	}
}

// TestRenderParsed_DoesNotCallLoadExternalCSS — the key M3 invariant:
// RenderParsed is the snapshot entry point; it does NOT trigger
// loadExternalCSS. A bogus <link> in the document must be ignored.
func TestRenderParsed_DoesNotCallLoadExternalCSS(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetHeadless(true)

	body := `<html><head>
		<link rel="stylesheet" href="https://no.such.host/x.css">
	</head><body><p>x</p></body></html>`
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if _, err := r.RenderParsed(context.Background(), doc, nil); err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	// No panic, no block: a regression to synchronous loadExternalCSS
	// would hang here on the unreachable URL.
}

// TestMergeInlineAndExternalCSS_EmptyCases — direct unit test on the
// merge helper, covering nil inputs.
func TestMergeInlineAndExternalCSS_EmptyCases(t *testing.T) {
	got := renderer.MergeInlineAndExternalCSS(nil, nil)
	if got == nil {
		t.Fatal("nil result for non-nil inputs")
	}
	if len(got.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(got.Rules))
	}

	got = renderer.MergeInlineAndExternalCSS(&css.StyleSheet{}, []renderer.ExternalCSS{
		{Source: []byte(".x { color: red; }")},
	})
	if len(got.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(got.Rules))
	}
}

// TestMergeInlineAndExternalCSS_NilExternal — nil external list with
// non-nil inline stylesheet.
func TestMergeInlineAndExternalCSS_NilExternal(t *testing.T) {
	inline := &css.StyleSheet{}
	inline.Rules = append(inline.Rules, css.Rule{
		Selectors:    []css.SelectorSequence{{Simple: css.SimpleSelector{Classes: []string{"x"}}}},
		Declarations: []css.Declaration{{Property: "color", Value: "red"}},
	})
	got := renderer.MergeInlineAndExternalCSS(inline, nil)
	if got == nil || len(got.Rules) != 1 {
		t.Errorf("nil external should preserve inline, got %+v", got)
	}
}

// hasClassSelector reports whether any of the rule's selector sequences
// match the simple class selector `.className`.
func hasClassSelector(rule css.Rule, className string) bool {
	wanted := strings.TrimPrefix(className, ".")
	for _, seq := range rule.Selectors {
		for _, c := range seq.Simple.Classes {
			if c == wanted {
				return true
			}
		}
	}
	return false
}

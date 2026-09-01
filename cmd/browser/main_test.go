package main

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/documentloader"
	"github.com/vyquocvu/goosie/internal/js"
	ghtml "golang.org/x/net/html"
)

// TestInlineScriptsByPosition_Aligns verifies that the position the
// ghtml.Parse DOM walker assigns to each inline <script> matches the
// position the streaming parser assigns via its resPos counter.
//
// The streaming parser increments resPos on every start tag EXCEPT
// <html>, <head>, and <body> (those return early in handleStartTag
// before the increment). The walker mirrors that quirk.
//
// The test captures positions from BOTH parsers and asserts the
// inline-script positions match exactly. The external <script src>
// is excluded from the comparison because the walker never emits it.
func TestInlineScriptsByPosition_Aligns(t *testing.T) {
	body := `<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>M4 align</title>
	<style>body { color: red; }</style>
</head>
<body>
	<h1>Hello</h1>
	<script>var first = 1;</script>
	<p>between</p>
	<script src="external.js"></script>
	<script>var third = 3;</script>
	<p>after</p>
</body>
</html>`

	var streamingPositions []int
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(body), dom.ParseConfig{
			OnResource: func(r dom.Resource) {
				if r.Kind == dom.ResourceScript && r.Inline {
					streamingPositions = append(streamingPositions, r.Position)
				}
			},
		})
	if err != nil {
		t.Fatalf("stream parse: %v", err)
	}

	parsed, err := ghtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ghtml parse: %v", err)
	}
	bodies := inlineScriptsByPosition(parsed)
	walkerPositions := make([]int, 0, len(bodies))
	for pos := range bodies {
		walkerPositions = append(walkerPositions, pos)
	}
	// Map iteration order is non-deterministic; sort for comparison.
	sort.Ints(walkerPositions)

	if len(streamingPositions) != len(walkerPositions) {
		t.Fatalf("position count mismatch: streaming=%v walker=%v",
			streamingPositions, walkerPositions)
	}
	for i := range streamingPositions {
		if streamingPositions[i] != walkerPositions[i] {
			t.Errorf("position mismatch at inline #%d: streaming=%d walker=%d",
				i, streamingPositions[i], walkerPositions[i])
		}
	}
}

// TestInlineScriptsByPosition_EmptyAndMissing verifies edge cases.
func TestInlineScriptsByPosition_EmptyAndMissing(t *testing.T) {
	if got := inlineScriptsByPosition(nil); got == nil || len(got) != 0 {
		t.Errorf("nil doc should return empty map, got %v", got)
	}

	// No scripts at all.
	noScripts, _ := ghtml.Parse(strings.NewReader(`<html><body><p>x</p></body></html>`))
	if got := inlineScriptsByPosition(noScripts); len(got) != 0 {
		t.Errorf("expected empty map for no scripts, got %d entries", len(got))
	}

	// Only external scripts.
	onlyExternal, _ := ghtml.Parse(strings.NewReader(
		`<html><head><script src="a.js"></script><script src="b.js"></script></head></html>`))
	if got := inlineScriptsByPosition(onlyExternal); len(got) != 0 {
		t.Errorf("external-only should be empty, got %d entries", len(got))
	}
}

// TestInlineScriptsByPosition_EmptyScriptsIgnored — empty <script></script>
// tags don't produce a body (and so don't get a map entry).
func TestInlineScriptsByPosition_EmptyScriptsIgnored(t *testing.T) {
	parsed, _ := ghtml.Parse(strings.NewReader(
		`<html><body><script></script><script>real</script></body></html>`))
	got := inlineScriptsByPosition(parsed)
	if len(got) != 1 {
		t.Errorf("expected 1 entry (empty script skipped), got %d", len(got))
	}
	for _, b := range got {
		if b != "real" {
			t.Errorf("body = %q, want %q", b, "real")
		}
	}
}

// TestInlineScriptsByPosition_ScriptWithSrcOnly — script with src but
// also text body: only the src is checked; the body is irrelevant.
func TestInlineScriptsByPosition_ScriptWithSrcOnly(t *testing.T) {
	parsed, _ := ghtml.Parse(strings.NewReader(
		`<html><body><script src="x.js">ignored body</script><script>kept</script></body></html>`))
	got := inlineScriptsByPosition(parsed)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	for _, b := range got {
		if b != "kept" {
			t.Errorf("body = %q, want %q", b, "kept")
		}
	}
}

// TestExecuteScriptQueue_DocumentOrder — M4 acceptance: scripts
// execute in document order. Mixes inline and external (mocked) in a
// known sequence and verifies the js.Runtime sees them in that order.
//
// js.Runtime is exercised indirectly via its last-evaluation observable
// (a global variable). Since js.Runtime maintains state across calls,
// we set a tracking variable in each script and assert the final value
// matches the last-executed script.
func TestExecuteScriptQueue_DocumentOrder(t *testing.T) {
	// Build a coordinator with three external scripts via mock fetcher.
	// We use the documentloader's NetFetcher pattern, but here we
	// bypass the full coordinator and call executeScriptQueue directly
	// with synthetic ScriptResults.

	rt := js.NewRuntime()
	defer rt.SetDOMMutationCallback(nil) // no-op cleanup

	// Synthetic results in known document order:
	// pos=0: inline "globalThis.M4_ORDER = ['inline-0']"
	// pos=1: external "globalThis.M4_ORDER.push('ext-1')"
	// pos=2: inline "globalThis.M4_ORDER.push('inline-2')"
	results := []documentloader.ScriptResult{
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 0},
		{Inline: false, Mode: documentloader.ScriptModeClassic, Position: 1,
			URL:    "https://example.com/ext-1.js",
			Source: []byte("globalThis.M4_ORDER.push('ext-1')")},
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 2},
	}
	// Fill inline sources from the DOM walk.
	doc, _ := ghtml.Parse(strings.NewReader(
		`<html><body><script>globalThis.M4_ORDER = ['inline-0']</script><script src="https://example.com/ext-1.js"></script><script>globalThis.M4_ORDER.push('inline-2')</script></body></html>`))

	executeScriptQueue(rt, "https://example.com/", nil, doc, results)

	// Verify final M4_ORDER matches the document order.
	orderVal, err := rt.RunScript(`globalThis.M4_ORDER.join(',')`)
	if err != nil {
		t.Fatalf("read M4_ORDER: %v", err)
	}
	if orderVal == nil || orderVal.String() != "inline-0,ext-1,inline-2" {
		t.Errorf("script order = %v, want inline-0,ext-1,inline-2",
			orderVal)
	}
}

// TestExecuteScriptQueue_EmptyQueue — no scripts, no panic.
func TestExecuteScriptQueue_EmptyQueue(t *testing.T) {
	rt := js.NewRuntime()
	executeScriptQueue(rt, "https://example.com/", nil, nil, nil)
	executeScriptQueue(rt, "https://example.com/", nil, nil, []documentloader.ScriptResult{})
}

// TestExecuteScriptQueue_NilRuntime — nil runtime is a no-op (caller
// must check before calling; this is a defensive guard).
func TestExecuteScriptQueue_NilRuntime(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil runtime panicked: %v", r)
		}
	}()
	executeScriptQueue(nil, "https://example.com/", nil, nil,
		[]documentloader.ScriptResult{{Inline: true, Position: 0, Source: []byte("x")}})
}

// TestExecuteScriptQueue_SortedDefensively — even if callbacks deliver
// results out of order, executeScriptQueue sorts by Position before
// running.
func TestExecuteScriptQueue_SortedDefensively(t *testing.T) {
	rt := js.NewRuntime()
	doc, _ := ghtml.Parse(strings.NewReader(
		`<html><body><script>globalThis.M4 = ['a']</script><script src="b.js"></script><script>globalThis.M4.push('c')</script></body></html>`))

	// Out-of-order delivery. The DOM has three scripts; we deliver
	// results for them in scrambled order.
	results := []documentloader.ScriptResult{
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 2}, // 'c' arrives first
		{Inline: false, Mode: documentloader.ScriptModeClassic, Position: 1,
			URL:    "https://example.com/b.js",
			Source: []byte("globalThis.M4.push('b')")}, // 'b' second
		{Inline: true, Mode: documentloader.ScriptModeClassic, Position: 0}, // 'a' third
	}
	executeScriptQueue(rt, "https://example.com/", nil, doc, results)

	order, _ := rt.RunScript(`globalThis.M4.join(',')`)
	if order == nil || order.String() != "a,b,c" {
		t.Errorf("expected sorted execution a,b,c; got %v", order)
	}
}

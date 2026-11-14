package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func corsFaceSession(t *testing.T, docBase, cdnURL string, headers map[string]string) *engine.Session {
	t.Helper()
	html := `<html><body><p id="p" style="font-family: 'CorsFace'">hi</p></body></html>`
	css := `@font-face { font-family: 'CorsFace'; src: url("` + cdnURL + `"); }`
	fetch := func(base, url string) (engine.FontResult, error) {
		return engine.FontResult{Data: []byte("font bytes"), Headers: headers}, nil
	}
	reg := &fakeRegistry{}
	s, err := engine.NewSession(html, []string{css}, 800,
		engine.WithCustomFontLoading(docBase, fetch, reg))
	if err != nil {
		t.Fatalf("build session: %v", err)
	}
	return s
}

func TestFontCORSMissingHeaderRefuses(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff", nil)
	if got := customIdx(t, s); got != 0 {
		t.Fatalf("cross-origin font without ACAO registered at index %d, want fallback 0", got)
	}
}

func TestFontCORSWildcardAllows(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff",
		map[string]string{"Access-Control-Allow-Origin": "*"})
	if got := customIdx(t, s); got != 1 {
		t.Fatalf("cross-origin font with ACAO * resolved to %d, want 1", got)
	}
}

func TestFontCORSExactOriginAllows(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff",
		map[string]string{"Access-Control-Allow-Origin": "https://example.com"})
	if got := customIdx(t, s); got != 1 {
		t.Fatalf("cross-origin font with matching ACAO resolved to %d, want 1", got)
	}
}

func TestFontCORSMismatchedOriginRefuses(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff",
		map[string]string{"Access-Control-Allow-Origin": "https://evil.example/"})
	if got := customIdx(t, s); got != 0 {
		t.Fatalf("cross-origin font with mismatched ACAO registered at index %d, want 0", got)
	}
}

func TestFontCORSSameOriginNeedsNoHeaders(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://example.com/face.woff", nil)
	if got := customIdx(t, s); got != 1 {
		t.Fatalf("same-origin font resolved to %d, want 1", got)
	}
}

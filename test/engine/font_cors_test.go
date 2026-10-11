package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// corsFaceSession builds a session whose @font-face src lives on cdnURL
// while the document sits at docBase, with the fetcher answering the given
// response headers. Registration is observed through the cascade: index 1
// means the face loaded, 0 means fallback after a CORS refusal.
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

// TestFontCORSMissingHeaderRefuses: a cross-origin font without
// Access-Control-Allow-Origin must not register; text keeps fallback.
func TestFontCORSMissingHeaderRefuses(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff", nil)
	if got := customIdx(t, s); got != 0 {
		t.Fatalf("cross-origin font without ACAO registered at index %d, want fallback 0", got)
	}
}

// TestFontCORSWildcardAllows: ACAO * admits the cross-origin face.
func TestFontCORSWildcardAllows(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff",
		map[string]string{"Access-Control-Allow-Origin": "*"})
	if got := customIdx(t, s); got != 1 {
		t.Fatalf("cross-origin font with ACAO * resolved to %d, want 1", got)
	}
}

// TestFontCORSExactOriginAllows: an explicit matching origin admits the face.
func TestFontCORSExactOriginAllows(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff",
		map[string]string{"Access-Control-Allow-Origin": "https://example.com"})
	if got := customIdx(t, s); got != 1 {
		t.Fatalf("cross-origin font with matching ACAO resolved to %d, want 1", got)
	}
}

// TestFontCORSMismatchedOriginRefuses: another site's origin admits nothing.
func TestFontCORSMismatchedOriginRefuses(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://cdn.example.com/face.woff",
		map[string]string{"Access-Control-Allow-Origin": "https://evil.example/"})
	if got := customIdx(t, s); got != 0 {
		t.Fatalf("cross-origin font with mismatched ACAO registered at index %d, want 0", got)
	}
}

// TestFontCORSSameOriginNeedsNoHeaders: same-origin faces load freely.
func TestFontCORSSameOriginNeedsNoHeaders(t *testing.T) {
	s := corsFaceSession(t, "https://example.com/", "https://example.com/face.woff", nil)
	if got := customIdx(t, s); got != 1 {
		t.Fatalf("same-origin font resolved to %d, want 1", got)
	}
}

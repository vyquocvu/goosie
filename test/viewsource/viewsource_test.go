package viewsource_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/viewsource"
)

func TestIsViewSourceURL(t *testing.T) {
	tests := []struct {
		input       string
		wantTarget  string
		wantIsMatch bool
	}{
		{"view-source:https://example.com/test", "https://example.com/test", true},
		{"VIEW-SOURCE:http://localhost:8080", "http://localhost:8080", true},
		{"https://example.com", "https://example.com", false},
		{"goosie://inspect", "goosie://inspect", false},
	}

	for _, tc := range tests {
		target, ok := viewsource.IsViewSourceURL(tc.input)
		if ok != tc.wantIsMatch {
			t.Errorf("IsViewSourceURL(%q) match = %v, want %v", tc.input, ok, tc.wantIsMatch)
		}
		if target != tc.wantTarget {
			t.Errorf("IsViewSourceURL(%q) target = %q, want %q", tc.input, target, tc.wantTarget)
		}
	}
}

func TestWrapViewSourceURL(t *testing.T) {
	if got := viewsource.WrapViewSourceURL("https://go.dev"); got != "view-source:https://go.dev" {
		t.Errorf("WrapViewSourceURL = %q, want 'view-source:https://go.dev'", got)
	}
	if got := viewsource.WrapViewSourceURL("view-source:https://go.dev"); got != "view-source:https://go.dev" {
		t.Errorf("WrapViewSourceURL idempotent failed, got %q", got)
	}
}

func TestHighlightHTML(t *testing.T) {
	sample := `<!DOCTYPE html><!-- Test Comment --><div class="wrapper" id='main'><a href="/about">About</a></div>`
	highlighted := viewsource.HighlightHTML(sample, "https://example.com/home/")

	if !strings.Contains(highlighted, "token-doctype") {
		t.Errorf("expected doctype token in %s", highlighted)
	}
	if !strings.Contains(highlighted, "token-comment") {
		t.Errorf("expected comment token in %s", highlighted)
	}
	if !strings.Contains(highlighted, "token-tag") {
		t.Errorf("expected tag token in %s", highlighted)
	}
	if !strings.Contains(highlighted, "token-attr-name") {
		t.Errorf("expected attr name token in %s", highlighted)
	}
	if !strings.Contains(highlighted, "token-attr-value") {
		t.Errorf("expected attr value token in %s", highlighted)
	}
	// Check resolved source-link
	if !strings.Contains(highlighted, `href="view-source:https://example.com/about"`) {
		t.Errorf("expected resolved view-source anchor link in %s", highlighted)
	}
}

func TestRenderHTML(t *testing.T) {
	doc := viewsource.RenderHTML("Line 1\nLine 2\nLine 3", "https://test.local", false)
	if !strings.Contains(doc, "view-source:https://test.local") {
		t.Errorf("expected title in RenderHTML")
	}
	if !strings.Contains(doc, `line-1`) || !strings.Contains(doc, `line-3`) {
		t.Errorf("expected line ids in RenderHTML")
	}
	if !strings.Contains(doc, "3 lines") {
		t.Errorf("expected line stats in RenderHTML")
	}

	darkDoc := viewsource.RenderHTML("Line 1", "https://test.local", true)
	if !strings.Contains(darkDoc, "#1e1e1e") {
		t.Errorf("expected dark background in dark mode RenderHTML")
	}
}

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/flags"
)

func TestFlags_InternalPageRendering(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := flags.NewManager(filepath.Join(tmpDir, "flags.json"))
	f := &framePath{
		flags: mgr,
	}

	title, htmlContent := buildFlagsPageHTML(f, "goosie://flags")
	if title != "Experiments - Goosie" {
		t.Errorf("title = %q, want 'Experiments - Goosie'", title)
	}
	if !strings.Contains(htmlContent, "Experiments &amp; Feature Flags") {
		t.Errorf("expected header in flags page")
	}
	if !strings.Contains(htmlContent, "Smooth Scrolling") {
		t.Errorf("expected Smooth Scrolling flag in page")
	}
	if !strings.Contains(htmlContent, "DNS-over-HTTPS (DoH) Enforcement") {
		t.Errorf("expected DoH flag in page")
	}
	if strings.Contains(htmlContent, "You have modified experimental feature flags") {
		t.Errorf("did not expect modified banner when all flags are default")
	}
}

func TestFlags_ToggleAndReset(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := flags.NewManager(filepath.Join(tmpDir, "flags.json"))
	f := &framePath{
		flags: mgr,
	}

	// 1. Toggle doh-dns from false to true
	buildFlagsPageHTML(f, "goosie://flags?action=toggle&id=doh-dns")
	if !f.flags.IsEnabled("doh-dns") {
		t.Errorf("expected doh-dns to be enabled after toggle")
	}

	_, modifiedHTML := buildFlagsPageHTML(f, "goosie://flags")
	if !strings.Contains(modifiedHTML, "You have modified experimental feature flags") {
		t.Errorf("expected modified banner after toggling flag")
	}

	// 2. Reset all
	buildFlagsPageHTML(f, "goosie://flags?action=reset")
	if f.flags.IsEnabled("doh-dns") {
		t.Errorf("expected doh-dns to be disabled after reset")
	}

	_, resetHTML := buildFlagsPageHTML(f, "goosie://flags")
	if strings.Contains(resetHTML, "You have modified experimental feature flags") {
		t.Errorf("expected modified banner to disappear after reset")
	}
}

func TestFlags_SearchFilter(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := flags.NewManager(filepath.Join(tmpDir, "flags.json"))
	f := &framePath{
		flags: mgr,
	}

	_, searchHTML := buildFlagsPageHTML(f, "goosie://flags?q=scrolling")
	if !strings.Contains(searchHTML, "Smooth Scrolling") {
		t.Errorf("expected Smooth Scrolling to match search query 'scrolling'")
	}
	if strings.Contains(searchHTML, "DNS-over-HTTPS") {
		t.Errorf("did not expect DNS-over-HTTPS to match search query 'scrolling'")
	}

	_, noMatchHTML := buildFlagsPageHTML(f, "goosie://flags?q=xyznonexistent")
	if !strings.Contains(noMatchHTML, "No flags match your search query") {
		t.Errorf("expected empty state when search has no matches")
	}
}

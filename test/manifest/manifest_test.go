package manifest_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/manifest"
)

func TestParse_FullManifest(t *testing.T) {
	data := []byte(`{
		"name": "Goosie Chat",
		"short_name": "Chat",
		"description": "PWA Chat Client",
		"start_url": "/chat/index.html",
		"display": "standalone",
		"theme_color": "#1a73e8",
		"background_color": "#ffffff",
		"icons": [
			{"src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png"}
		]
	}`)

	m, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if m.Name != "Goosie Chat" {
		t.Errorf("Name = %q, want 'Goosie Chat'", m.Name)
	}
	if m.ShortName != "Chat" {
		t.Errorf("ShortName = %q, want 'Chat'", m.ShortName)
	}
	if m.EffectiveName() != "Goosie Chat" {
		t.Errorf("EffectiveName = %q, want 'Goosie Chat'", m.EffectiveName())
	}
	if !m.IsStandalone() {
		t.Error("expected IsStandalone=true for display: standalone")
	}
	if m.ThemeColor != "#1a73e8" {
		t.Errorf("ThemeColor = %q, want '#1a73e8'", m.ThemeColor)
	}
	if len(m.Icons) != 1 || m.Icons[0].Sizes != "192x192" {
		t.Errorf("Icons = %+v, want 1 icon with size 192x192", m.Icons)
	}

	resolved := m.ResolveStartURL("https://example.com/app/")
	if resolved != "https://example.com/chat/index.html" {
		t.Errorf("ResolveStartURL = %q, want 'https://example.com/chat/index.html'", resolved)
	}
}

func TestParse_Defaults(t *testing.T) {
	data := []byte(`{"short_name": "Lite"}`)
	m, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if m.EffectiveName() != "Lite" {
		t.Errorf("EffectiveName = %q, want 'Lite'", m.EffectiveName())
	}
	if m.Display != manifest.DisplayBrowser {
		t.Errorf("Display default = %q, want 'browser'", m.Display)
	}
	if m.IsStandalone() {
		t.Error("expected IsStandalone=false for default browser display")
	}
}

func TestExtractManifestURL(t *testing.T) {
	tests := []struct {
		html    string
		base    string
		wantURL string
	}{
		{
			html:    `<html><head><link rel="manifest" href="/app.webmanifest"></head></html>`,
			base:    "https://example.com/sub/index.html",
			wantURL: "https://example.com/app.webmanifest",
		},
		{
			html:    `<html><head><link href="manifest.json" rel="manifest"></head></html>`,
			base:    "https://example.com/app/",
			wantURL: "https://example.com/app/manifest.json",
		},
		{
			html:    `<html><head><link rel="stylesheet" href="style.css"></head></html>`,
			base:    "https://example.com/",
			wantURL: "",
		},
	}

	for i, tc := range tests {
		got := manifest.ExtractManifestURL(tc.html, tc.base)
		if got != tc.wantURL {
			t.Errorf("Test %d: ExtractManifestURL = %q, want %q", i, got, tc.wantURL)
		}
	}
}

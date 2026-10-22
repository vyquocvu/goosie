package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/manifest"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestPWAManifestParsing(t *testing.T) {
	raw := `{
		"name": "Goosie Calculator",
		"short_name": "Calc",
		"start_url": "/calc/index.html",
		"display": "standalone",
		"theme_color": "#1a73e8",
		"background_color": "#ffffff",
		"icons": [
			{"src": "/icons/192.png", "sizes": "192x192", "type": "image/png"}
		]
	}`

	m, err := manifest.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if m.EffectiveName() != "Goosie Calculator" {
		t.Errorf("EffectiveName = %q, want %q", m.EffectiveName(), "Goosie Calculator")
	}
	if !m.IsStandalone() {
		t.Errorf("IsStandalone = false, want true")
	}
	resolved := m.ResolveStartURL("https://example.com/subpath/")
	if resolved != "https://example.com/calc/index.html" {
		t.Errorf("ResolveStartURL = %q, want %q", resolved, "https://example.com/calc/index.html")
	}
	if len(m.Icons) != 1 || m.Icons[0].Src != "/icons/192.png" {
		t.Errorf("unexpected icons: %+v", m.Icons)
	}
}

func TestPWAExtractManifestFromDOM(t *testing.T) {
	htmlSource := `<!DOCTYPE html>
<html>
<head>
  <title>PWA Demo</title>
  <link rel="manifest" href="/app.webmanifest">
</head>
<body>
  <h1>PWA Test</h1>
</body>
</html>`

	limits := dom.ParseLimits{Nodes: 100, Depth: 50, Attributes: 50, AttributeBytes: 1024}
	doc, err := dom.ParseBounded(htmlSource, limits)
	if err != nil || doc == nil {
		t.Fatalf("dom.ParseBounded error: %v", err)
	}

	manifestURL := findManifestLink(&doc.Node, "https://pwa.example.com/demo/index.html")
	expected := "https://pwa.example.com/app.webmanifest"
	if manifestURL != expected {
		t.Errorf("findManifestLink = %q, want %q", manifestURL, expected)
	}

	noManifestHTML := `<html><head><title>No manifest</title></head></html>`
	doc2, err := dom.ParseBounded(noManifestHTML, limits)
	if err != nil || doc2 == nil {
		t.Fatalf("dom.ParseBounded error: %v", err)
	}
	if u := findManifestLink(&doc2.Node, "https://example.com"); u != "" {
		t.Errorf("findManifestLink on document without manifest returned %q", u)
	}
}

func TestPWACommandLineFlag(t *testing.T) {
	cfg, err := parse([]string{"-app", "https://todo.app/login"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if cfg.appURL != "https://todo.app/login" {
		t.Errorf("cfg.appURL = %q, want %q", cfg.appURL, "https://todo.app/login")
	}
	if cfg.url != "https://todo.app/login" {
		t.Errorf("cfg.url = %q, want %q", cfg.url, "https://todo.app/login")
	}
}

type fakePWAWindow struct {
	lastBitmap *frame.Bitmap
	lastDamage []frame.Rect
}

func (w *fakePWAWindow) Events() <-chan surface.Event { return make(chan surface.Event) }
func (w *fakePWAWindow) Present(buf *frame.Bitmap, dmg []frame.Rect) error {
	w.lastBitmap = buf
	w.lastDamage = dmg
	return nil
}
func (w *fakePWAWindow) SetCursor(surface.Cursor) {}
func (w *fakePWAWindow) SetIME(bool)              {}
func (w *fakePWAWindow) ScaleFactor() float32     { return 1.0 }
func (w *fakePWAWindow) Close() error             { return nil }

func TestPWAStandaloneModeWindowPresentation(t *testing.T) {
	fonts, _ := raster.NewFonts()
	tb := toolbar.NewState(800, fonts)
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()
	tab.SetURL("https://myapp.com")

	fakeWin := &fakePWAWindow{}
	cw := &chromeWindow{
		Window:  fakeWin,
		toolbar: tb,
		tabMgr:  mgr,
		scale:   1,
		fonts:   fonts,
		events:  make(chan surface.Event, 10),
	}

	// Normal browser mode: chromeHeight > 0
	normalH := cw.chromeHeight()
	if normalH <= 0 {
		t.Fatalf("expected positive chromeHeight in browser mode, got %d", normalH)
	}

	// Standalone mode: chromeHeight == 0
	tab.SetStandaloneApp(true)
	standaloneH := cw.chromeHeight()
	if standaloneH != 0 {
		t.Fatalf("expected chromeHeight == 0 in standalone mode, got %d", standaloneH)
	}

	// Present in standalone mode should present buf directly without adding chrome height
	buf := frame.NewBitmap(800, 600)
	buf.FillRect(frame.Rect4(0, 0, 800, 600), frame.RGB(40, 50, 60), nil)
	damage := []frame.Rect{frame.Rect4(0, 0, 800, 600)}

	if err := cw.Present(buf, damage); err != nil {
		t.Fatalf("Present error: %v", err)
	}

	if fakeWin.lastBitmap == nil {
		t.Fatal("expected lastBitmap to be presented")
	}
	if fakeWin.lastBitmap.H != 600 {
		t.Errorf("presented height = %d, want 600 (chromeless)", fakeWin.lastBitmap.H)
	}

	// Pointer event at y=10 should NOT be consumed by tab bar in standalone mode
	clickEv := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerPress,
		Button: surface.ButtonLeft,
		Pos:    frame.Point{X: 100, Y: 10},
	}
	intercepted := cw.intercept(clickEv)
	if intercepted {
		t.Errorf("pointer event at y=10 was intercepted by chrome despite standalone mode")
	}
}

func TestPWAInspectorPageCardAndToggle(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()
	tab.SetURL("https://app.sample.com")
	tab.SetTitle("Sample App")
	sess, err := engine.NewSession("<html><body>Sample App</body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)

	f := &framePath{
		tabMgr: mgr,
	}

	tabQuery := strconv.FormatUint(tab.ID(), 10)

	// 1. Without manifest: shows no manifest state and toggle link
	title, htmlContent := buildInspectPageHTML(f, "goosie://inspect?tab="+tabQuery)
	if title != "Web Inspector" {
		t.Errorf("title = %q, want 'Web Inspector'", title)
	}
	if !strings.Contains(htmlContent, "Progressive Web App (PWA) &amp; Manifest") {
		t.Errorf("expected PWA card in inspector page")
	}
	if !strings.Contains(htmlContent, "No W3C Web App Manifest detected") {
		t.Errorf("expected No manifest text")
	}

	// 2. Set manifest on tab: inspector shows manifest metadata
	tab.SetManifest(&manifest.Manifest{
		Name:            "Sample Progressive App",
		ShortName:       "SampleApp",
		StartURL:        "/start",
		Display:         manifest.DisplayStandalone,
		ThemeColor:      "#ff5500",
		BackgroundColor: "#111111",
		Icons: []manifest.Icon{
			{Src: "/icon.png", Sizes: "512x512"},
		},
	})

	_, htmlWithManifest := buildInspectPageHTML(f, "goosie://inspect?tab="+tabQuery)
	if !strings.Contains(htmlWithManifest, "Sample Progressive App") {
		t.Errorf("expected app name in manifest card")
	}
	if !strings.Contains(htmlWithManifest, "#ff5500") {
		t.Errorf("expected theme color in manifest card")
	}

	// 3. Toggle standalone action
	if tab.IsStandaloneApp() {
		t.Errorf("expected initially not standalone")
	}
	buildInspectPageHTML(f, "goosie://inspect?action=toggle_standalone&tab="+tabQuery)
	if !tab.IsStandaloneApp() {
		t.Errorf("expected standalone = true after toggle action")
	}
	buildInspectPageHTML(f, "goosie://inspect?action=toggle_standalone&tab="+tabQuery)
	if tab.IsStandaloneApp() {
		t.Errorf("expected standalone = false after second toggle action")
	}
}

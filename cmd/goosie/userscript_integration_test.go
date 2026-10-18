package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/userscript"
)

func newUserscriptTestPath(t *testing.T) (*framePath, string) {
	t.Helper()
	dir := t.TempDir()
	spec := paint.SceneSpec{DocHeight: 256}
	_, layer := paint.BuildLayer(spec)
	pool := raster.New(1, 4, func(j raster.Job) error { return nil })
	pool.Start(context.Background())
	t.Cleanup(func() { _ = pool.Close() })
	sched := raster.NewScheduler(layer, pool, frame.Viewport{Size: frame.Size{W: 256, H: 256}}, 1, raster.Pref{})

	scriptsDir := filepath.Join(dir, "userscripts")
	_ = os.MkdirAll(scriptsDir, 0o755)

	f := &framePath{
		sched:       sched,
		pool:        pool,
		navResults:  make(chan navResult, 16),
		imgResults:  make(chan imgResult, 16),
		zoom:        1.0,
		userscripts: userscript.NewManager(scriptsDir),
		config: config{
			userscriptDir: scriptsDir,
		},
	}
	f.tabMgr = tabs.NewManager(nil)
	firstTab := f.tabMgr.NewTab()
	firstTab.SetLayer(layer)
	return f, scriptsDir
}

func TestExtensionsPage_EmptyState(t *testing.T) {
	f, _ := newUserscriptTestPath(t)

	title, htmlContent := buildExtensionsPageHTML(f, "goosie://extensions")
	if title != "Extensions - Goosie" {
		t.Errorf("title = %q, want Extensions - Goosie", title)
	}
	if !strings.Contains(htmlContent, "No User Scripts Installed") {
		t.Errorf("expected empty-state message in HTML, got:\n%s", htmlContent)
	}
}

func TestExtensionsPage_ListAndToggle(t *testing.T) {
	f, _ := newUserscriptTestPath(t)

	s1 := &userscript.Script{
		ID:          "dark-hn",
		Name:        "HackerNews Dark",
		Version:     "1.0.0",
		Description: "Dark theme for Hacker News",
		Matches:     []string{"https://news.ycombinator.com/*"},
		RunAt:       userscript.RunAtDocumentEnd,
		Enabled:     true,
	}
	s2 := &userscript.Script{
		ID:          "gh-wide",
		Name:        "GitHub Widescreen",
		Version:     "2.1.0",
		Description: "Full width repositories",
		Matches:     []string{"https://github.com/*"},
		RunAt:       userscript.RunAtDocumentStart,
		Enabled:     false,
	}

	f.userscripts.AddScript(s1)
	f.userscripts.AddScript(s2)

	title, htmlContent := buildExtensionsPageHTML(f, "goosie://extensions")
	if title != "Extensions - Goosie" {
		t.Errorf("title = %q, want Extensions - Goosie", title)
	}
	if !strings.Contains(htmlContent, "HackerNews Dark") || !strings.Contains(htmlContent, "GitHub Widescreen") {
		t.Error("expected both scripts in HTML listing")
	}
	if !strings.Contains(htmlContent, "Disable") || !strings.Contains(htmlContent, "Enable") {
		t.Error("expected toggle buttons in HTML")
	}

	// Toggle s1 to disabled
	_, _ = buildExtensionsPageHTML(f, "goosie://extensions?action=toggle&id=dark-hn")
	if s1.Enabled {
		t.Error("expected s1 to be disabled after toggle action")
	}

	// Toggle s1 back to enabled
	_, _ = buildExtensionsPageHTML(f, "goosie://extensions?action=toggle&id=dark-hn")
	if !s1.Enabled {
		t.Error("expected s1 to be enabled after second toggle action")
	}
}

// TestUserscripts_EndToEndInjection tests that navigating to a web page executes matching user scripts.
func TestUserscripts_EndToEndInjection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><h1 id="greeting">Hello World</h1></body></html>`))
	}))
	defer ts.Close()

	f, _ := newUserscriptTestPath(t)
	f.client = net.DefaultClient()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts

	// Register a script matching our test server
	script := &userscript.Script{
		ID:      "modifier",
		Name:    "Page Modifier",
		Matches: []string{ts.URL + "/*"},
		RunAt:   userscript.RunAtDocumentEnd,
		Code:    `document.getElementById("greeting").textContent = "Modified by Userscript";`,
		Enabled: true,
	}
	f.userscripts.AddScript(script)

	layer, _, _, sess, _, err := f.loadDocumentCtx(context.Background(), f.client, ts.URL+"/page", 800, 600, 1.0, "", false, false)
	if err != nil {
		t.Fatalf("loadDocumentCtx: %v", err)
	}
	if layer == nil || sess == nil {
		t.Fatal("expected non-nil layer and session")
	}
	defer sess.Close()

	val, err := sess.EvaluateJS(`document.getElementById("greeting").textContent`)
	if err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	if val != "Modified by Userscript" {
		t.Errorf("heading text = %q, want 'Modified by Userscript'", val)
	}
}

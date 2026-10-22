package main

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestIncognitoTabCreationAndToolbarSync(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width = 800
	f.config.height = 600
	f.toolbar = toolbar.NewState(800, nil)

	// Create incognito tab
	tab := f.newIncognitoTab()
	if !tab.Incognito() {
		t.Fatal("tab.Incognito() should be true")
	}
	if tab.Title() != "New Incognito Tab" {
		t.Errorf("title = %q, want 'New Incognito Tab'", tab.Title())
	}
	if tab.BGColor() != frame.RGB(40, 44, 52) {
		t.Errorf("bgColor = %v, want frame.RGB(40, 44, 52)", tab.BGColor())
	}

	// Toolbar should have received incognito mode update
	if !f.toolbar.Incognito {
		t.Fatal("f.toolbar.Incognito should be true after switching to incognito tab")
	}
}

func TestIncognitoSessionSuppression(t *testing.T) {
	f := newNavTestPath(t)
	regTab := f.tabMgr.NewTab()
	regTab.SetURL("https://example.com/regular")
	regTab.SetTitle("Regular")

	incogTab := f.newIncognitoTab()
	incogTab.SetURL("https://secret.com/private")
	incogTab.SetTitle("Secret")

	st := sessionState(f)
	if len(st.Tabs) != 2 { // firstTab + regTab
		t.Fatalf("expected 2 non-incognito tabs in sessionState, got %d", len(st.Tabs))
	}
	for _, ts := range st.Tabs {
		if ts.URL == "https://secret.com/private" || ts.Title == "Secret" {
			t.Fatal("incognito tab leaked into persistent sessionState")
		}
	}
}

func TestIncognitoHistorySuppression(t *testing.T) {
	tmpDir := t.TempDir()
	histStore, err := history.Load(filepath.Join(tmpDir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}

	f := newNavTestPath(t)
	f.history = histStore

	// 1. Regular tab navigation records history
	regTab := f.tabMgr.NewTab()
	f.switchTab(regTab.ID())
	f.applyNavResult(navResult{
		tabID:  regTab.ID(),
		serial: regTab.Nav.Serial,
		url:    "https://example.com/public",
		layer:  frame.NewLayer(1, frame.Rect4(0, 0, 100, 100), 1024, nil),
	})

	entries := f.history.Entries()
	if len(entries) != 1 || entries[0].URL != "https://example.com/public" {
		t.Fatalf("regular visit not recorded in history: %v", entries)
	}

	// 2. Incognito tab navigation must NOT record history
	incogTab := f.newIncognitoTab()
	f.switchTab(incogTab.ID())
	f.applyNavResult(navResult{
		tabID:  incogTab.ID(),
		serial: incogTab.Nav.Serial,
		url:    "https://secret.example.com/incognito",
		layer:  frame.NewLayer(1, frame.Rect4(0, 0, 100, 100), 1024, nil),
	})

	entries = f.history.Entries()
	if len(entries) != 1 {
		t.Fatalf("incognito visit leaked into history! entries: %v", entries)
	}
	for _, e := range entries {
		if e.URL == "https://secret.example.com/incognito" {
			t.Fatal("incognito visit found in history entries")
		}
	}
}

func TestIncognitoShortcuts(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tb := toolbar.NewState(800, nil)
	var incognitoTriggered bool
	cw := &chromeWindow{
		toolbar: tb,
		tabMgr:  mgr,
		onNewIncognitoTab: func() {
			incognitoTriggered = true
		},
	}

	// Cmd+Shift+N
	consumed := cw.handleTabShortcut('n', surface.ModCommand|surface.ModShift)
	if !consumed || !incognitoTriggered {
		t.Errorf("Cmd+Shift+N failed to trigger incognito: consumed=%v, triggered=%v", consumed, incognitoTriggered)
	}

	// Cmd+Shift+P
	incognitoTriggered = false
	consumed = cw.handleTabShortcut('p', surface.ModCommand|surface.ModShift)
	if !consumed || !incognitoTriggered {
		t.Errorf("Cmd+Shift+P failed to trigger incognito: consumed=%v, triggered=%v", consumed, incognitoTriggered)
	}
}

func TestIncognitoToolbarThemeDrawing(t *testing.T) {
	tb := toolbar.NewState(800, nil)
	tb.SetIncognito(true)

	buf := frame.NewBitmap(800, toolbar.ToolbarHeight)
	tb.Draw(buf, 0)

	// In incognito mode, toolbar background is frame.RGB(40, 44, 52)
	hasDarkBg := false
	for x := 0; x < 50; x++ {
		c := buf.At(x, 5).RGBA()
		if c.R == 40 && c.G == 44 && c.B == 52 {
			hasDarkBg = true
			break
		}
	}
	if !hasDarkBg {
		t.Error("toolbar in incognito mode did not draw dark stealth background frame.RGB(40, 44, 52)")
	}
}

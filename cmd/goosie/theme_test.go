package main

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

func TestThemeSwitching(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	// 1. Initial default is system (light)
	if f.isDarkMode() {
		t.Errorf("expected isDarkMode() = false initially, got true")
	}
	if f.isHighContrast() {
		t.Errorf("expected isHighContrast() = false initially, got true")
	}

	// 2. Set theme to dark
	f.setTheme("dark")
	if !f.isDarkMode() {
		t.Errorf("expected isDarkMode() = true after setTheme(\"dark\")")
	}
	activeTab := f.tabMgr.Active()
	if activeTab.BGColor() != frame.RGB(24, 24, 24) {
		t.Errorf("dark mode tab BGColor = %v, want #181818", activeTab.BGColor())
	}

	// 3. Set theme to high-contrast
	f.setTheme("high-contrast")
	if !f.isHighContrast() {
		t.Errorf("expected isHighContrast() = true after setTheme(\"high-contrast\")")
	}
	if f.isDarkMode() {
		t.Errorf("expected isDarkMode() = false under high-contrast")
	}

	// 4. Set theme to light
	f.setTheme("light")
	if f.isDarkMode() || f.isHighContrast() {
		t.Errorf("expected both false under light mode")
	}
	if activeTab.BGColor() != frame.RGB(255, 255, 255) {
		t.Errorf("light mode tab BGColor = %v, want #ffffff", activeTab.BGColor())
	}
}

func TestInspectPageThemeAction(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/demo")
	sess, err := engine.NewSession("<html><body>Demo Page</body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)

	// Trigger set_theme action via goosie://inspect
	_, inspectHTML := buildInspectPageHTML(f, "goosie://inspect?action=set_theme&theme=dark")
	if !f.isDarkMode() {
		t.Errorf("expected isDarkMode() = true after inspect action")
	}
	if !strings.Contains(inspectHTML, "Appearance &amp; Theme Mode") {
		t.Errorf("expected inspect HTML to render Appearance & Theme Mode card")
	}
	if !strings.Contains(inspectHTML, "Dark Mode") {
		t.Errorf("expected inspect HTML to contain Dark Mode button")
	}
}

func TestVersionPageAppearanceReport(t *testing.T) {
	f := newNavTestPath(t)
	f.setTheme("dark")

	_, versionHTML := buildVersionPageHTML(f)
	if !strings.Contains(versionHTML, "Appearance &amp; Theme Mode") {
		t.Errorf("expected version HTML to contain Appearance & Theme Mode section")
	}
	if !strings.Contains(versionHTML, "dark") {
		t.Errorf("expected version HTML to report dark theme")
	}
}

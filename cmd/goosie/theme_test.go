package main

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
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

// TestThemeChromeSurfacesStayInSync renders the tab strip and toolbar together
// for every appearance theme and asserts the two chrome surfaces, plus the
// security indicator, stay in step. This is the oracle the earlier
// TestThemeSwitching lacked: it only checked the tab's stored background.
// qc:BR-141
func TestThemeChromeSurfacesStayInSync(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	f.toolbar = toolbar.NewState(800, fonts)
	f.toolbar.URL = "https://example.test/"
	active := f.tabMgr.Active()
	active.SetURL("https://example.test/")
	active.SetTitle("Example")

	cases := []struct {
		theme string
		bg    frame.Color
	}{
		{"light", frame.RGB(255, 255, 255)},
		{"dark", frame.RGB(24, 24, 24)},
		{"high-contrast", frame.RGB(0, 0, 0)},
	}

	for _, tc := range cases {
		f.setTheme(tc.theme)

		if got := active.BGColor(); got != tc.bg {
			t.Errorf("%s: tab BGColor = %v, want %v", tc.theme, got, tc.bg)
		}

		chrome := frame.NewBitmap(800, tabs.TabBarHeight+toolbar.ToolbarHeight)
		tabs.DrawTabBar(chrome, f.tabMgr, 0, fonts, 1)
		f.toolbar.Draw(chrome, tabs.TabBarHeight)

		tabR := tabs.TabRect(0, 0, 1, 800)
		tabPix := chrome.At(int(tabR.X0)+3, tabs.TabBarHeight-3)
		toolbarPix := chrome.At(795, tabs.TabBarHeight+1)
		if tabPix != toolbarPix {
			t.Errorf("%s: active tab %v and toolbar %v disagree", tc.theme, tabPix, toolbarPix)
		}
		if toolbarPix != tc.bg {
			t.Errorf("%s: toolbar fill = %v, want %v", tc.theme, toolbarPix, tc.bg)
		}

		bar := toolbar.AddressBarRect(800)
		indicatorVisible := false
		for y := int(bar.Y0); y < int(bar.Y1) && !indicatorVisible; y++ {
			for x := int(bar.X0); x < int(bar.X0)+30; x++ {
				c := chrome.At(x, tabs.TabBarHeight+y)
				if c.G() > c.R()+20 && c.G() > c.B()+20 {
					indicatorVisible = true
					break
				}
			}
		}
		if !indicatorVisible {
			t.Errorf("%s: secure indicator is not visible against the chrome", tc.theme)
		}
	}
}

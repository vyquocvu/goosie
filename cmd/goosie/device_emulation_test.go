package main

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestDevicePresetsRegistry(t *testing.T) {
	if len(DevicePresets) < 3 {
		t.Fatalf("expected at least 3 device presets, got %d", len(DevicePresets))
	}

	iphone := DevicePresetByID("iphone-15-pro")
	if iphone == nil {
		t.Fatal("iphone-15-pro preset not found")
	}
	if iphone.Width != 393 || iphone.Height != 852 || iphone.DPR != 3.0 {
		t.Errorf("iphone-15-pro mismatch: %+v", iphone)
	}

	pixel := DevicePresetByID("pixel-7")
	if pixel == nil {
		t.Fatal("pixel-7 preset not found")
	}
	if pixel.Width != 412 || pixel.Height != 915 || pixel.DPR != 2.625 {
		t.Errorf("pixel-7 mismatch: %+v", pixel)
	}

	ipad := DevicePresetByID("ipad-air")
	if ipad == nil {
		t.Fatal("ipad-air preset not found")
	}
	if ipad.Width != 820 || ipad.Height != 1180 || ipad.DPR != 2.0 {
		t.Errorf("ipad-air mismatch: %+v", ipad)
	}

	if DevicePresetByID("nonexistent") != nil {
		t.Error("expected nil for nonexistent preset")
	}
}

func TestCycleDeviceEmulation(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 1024, height: 768, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com")
	sess, err := engine.NewSession("<html><body>Responsive Test</body></html>", nil, 1024)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)

	// Initially Desktop
	if f.ActiveEmulation() != nil {
		t.Fatal("expected ActiveEmulation == nil initially")
	}

	// 1st cycle -> iPhone 15 Pro
	f.cycleDeviceEmulation()
	if f.ActiveEmulation() == nil || f.ActiveEmulation().ID != "iphone-15-pro" {
		t.Fatalf("expected iphone-15-pro, got %+v", f.ActiveEmulation())
	}

	// 2nd cycle -> Pixel 7
	f.cycleDeviceEmulation()
	if f.ActiveEmulation() == nil || f.ActiveEmulation().ID != "pixel-7" {
		t.Fatalf("expected pixel-7, got %+v", f.ActiveEmulation())
	}

	// 3rd cycle -> iPad Air
	f.cycleDeviceEmulation()
	if f.ActiveEmulation() == nil || f.ActiveEmulation().ID != "ipad-air" {
		t.Fatalf("expected ipad-air, got %+v", f.ActiveEmulation())
	}

	// 4th cycle -> Desktop (nil)
	f.cycleDeviceEmulation()
	if f.ActiveEmulation() != nil {
		t.Fatalf("expected nil after cycling past last preset, got %+v", f.ActiveEmulation())
	}
}

func TestDeviceEmulationShortcutCmdShiftM(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)

	emulationToggled := 0
	cw := newChromeWindow(cw2, tb, mgr,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fonts, 1)
	cw.onDeviceEmulation = func() {
		emulationToggled++
	}

	// Trigger Cmd+Shift+M ('m')
	evKeyM := surface.Event{
		Kind: surface.EvKey,
		Key:  'm',
		Mods: surface.ModCommand | surface.ModShift,
	}
	if !cw.intercept(evKeyM) || emulationToggled != 1 {
		t.Fatalf("Cmd+Shift+M lowercase failed: handled=%v toggled=%d", cw.intercept(evKeyM), emulationToggled)
	}

	// Trigger Cmd+Shift+M ('M')
	evKeyCapM := surface.Event{
		Kind: surface.EvKey,
		Key:  'M',
		Mods: surface.ModCommand | surface.ModShift,
	}
	if !cw.intercept(evKeyCapM) || emulationToggled != 2 {
		t.Fatalf("Cmd+Shift+M uppercase failed: handled=%v toggled=%d", cw.intercept(evKeyCapM), emulationToggled)
	}
}

func TestInspectPageDeviceEmulation(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com")
	sess, err := engine.NewSession("<html><body>Inspect Emulation Test</body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)

	// Verify inspect page contains Device Emulation card
	title, htmlContent := f.buildInternalPageHTML("goosie://inspect")
	if !strings.Contains(title, "Web Inspector") {
		t.Fatalf("expected title Web Inspector, got %q", title)
	}
	if !strings.Contains(htmlContent, "Device Emulation") {
		t.Error("expected html to contain Device Emulation card")
	}
	if !strings.Contains(htmlContent, "iphone-15-pro") {
		t.Error("expected html to contain iphone-15-pro link")
	}

	// Switch emulation via inspect URL
	f.buildInternalPageHTML("goosie://inspect?action=emulate&device=iphone-15-pro")
	if f.ActiveEmulation() == nil || f.ActiveEmulation().ID != "iphone-15-pro" {
		t.Fatalf("expected active emulation iphone-15-pro, got %+v", f.ActiveEmulation())
	}

	// Reset emulation via inspect URL
	f.buildInternalPageHTML("goosie://inspect?action=emulate&device=reset")
	if f.ActiveEmulation() != nil {
		t.Fatalf("expected active emulation reset to nil, got %+v", f.ActiveEmulation())
	}
}

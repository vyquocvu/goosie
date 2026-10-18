package main

import (
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/frame"
)

// DevicePreset represents an emulated mobile or tablet device.
type DevicePreset struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	DPR       float32 `json:"dpr"`
	UserAgent string  `json:"userAgent,omitempty"`
	Mobile    bool    `json:"mobile"`
}

// Built-in popular mobile and tablet presets.
var DevicePresets = []DevicePreset{
	{
		ID:        "iphone-15-pro",
		Name:      "iPhone 15 Pro",
		Width:     393,
		Height:    852,
		DPR:       3.0,
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1 Goosie/0.2.0",
		Mobile:    true,
	},
	{
		ID:        "pixel-7",
		Name:      "Google Pixel 7",
		Width:     412,
		Height:    915,
		DPR:       2.625,
		UserAgent: "Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Mobile Safari/537.36 Goosie/0.2.0",
		Mobile:    true,
	},
	{
		ID:        "ipad-air",
		Name:      "iPad Air",
		Width:     820,
		Height:    1180,
		DPR:       2.0,
		UserAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1 Goosie/0.2.0",
		Mobile:    true,
	},
}

// DevicePresetByID finds a preset by its unique ID.
func DevicePresetByID(id string) *DevicePreset {
	for i := range DevicePresets {
		if DevicePresets[i].ID == id {
			return &DevicePresets[i]
		}
	}
	return nil
}

// ActiveEmulation returns the current device preset, or nil if desktop.
func (f *framePath) ActiveEmulation() *DevicePreset {
	return f.emulationPreset
}

// SetDeviceEmulation enables or disables device emulation.
func (f *framePath) SetDeviceEmulation(preset *DevicePreset) {
	f.emulationPreset = preset
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	reflowW := float32(f.config.width)
	if preset != nil {
		reflowW = float32(preset.Width)
	}
	if err := tab.Session().Reflow(reflowW); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: emulation reflow:", err)
		return
	}
	f.repaintTab(tab, frame.Rect{})
}

// cycleDeviceEmulation toggles through Desktop -> iPhone -> Pixel -> iPad -> Desktop.
func (f *framePath) cycleDeviceEmulation() {
	if f.emulationPreset == nil {
		f.SetDeviceEmulation(&DevicePresets[0])
		return
	}
	for i := range DevicePresets {
		if DevicePresets[i].ID == f.emulationPreset.ID {
			if i+1 < len(DevicePresets) {
				f.SetDeviceEmulation(&DevicePresets[i+1])
			} else {
				f.SetDeviceEmulation(nil) // cycle back to desktop
			}
			return
		}
	}
	f.SetDeviceEmulation(nil)
}

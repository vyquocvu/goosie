package toolbar

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// qc:BR-141
func TestToolbarThemePaintsDarkAndHighContrastChrome(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	render := func(theme string) (*frame.Bitmap, *State) {
		tb := NewState(800, fonts)
		tb.URL = "https://example.test/"
		tb.SetTheme(theme)
		buf := frame.NewBitmap(800, ToolbarHeight)
		tb.Draw(buf, 0)
		return buf, tb
	}

	light, _ := render("light")
	dark, darkState := render("dark")
	highContrast, highContrastState := render("high-contrast")

	if got := light.At(795, 1); got != frame.RGB(255, 255, 255) {
		t.Fatalf("light toolbar fill = %v, want #ffffff", got)
	}
	if got := dark.At(795, 1); got != frame.RGB(24, 24, 24) {
		t.Errorf("dark toolbar fill = %v, want #181818", got)
	}
	if got := highContrast.At(795, 1); got != frame.RGB(0, 0, 0) {
		t.Errorf("high-contrast toolbar fill = %v, want #000000", got)
	}

	bar := AddressBarRect(800)
	barX, barY := bar.X1-10, (bar.Y0+bar.Y1)/2
	if light.At(int(barX), int(barY)) == dark.At(int(barX), int(barY)) {
		t.Errorf("address bar fill did not change between light and dark")
	}

	if luminance(darkState.palette.text) <= luminance(darkState.palette.barBg) {
		t.Errorf("dark text luminance %v <= bar fill %v", darkState.palette.text, darkState.palette.barBg)
	}
	if luminance(highContrastState.palette.text) <= luminance(highContrastState.palette.barBg) {
		t.Errorf("high-contrast text luminance %v <= bar fill %v", highContrastState.palette.text, highContrastState.palette.barBg)
	}
}

// qc:BR-141
func TestToolbarThemeDefaultsToLight(t *testing.T) {
	tb := NewState(800, nil)
	tb.SetTheme("solarized")
	if tb.palette != lightPalette {
		t.Errorf("palette for unknown theme = %+v, want light", tb.palette)
	}
}

func luminance(c frame.Color) int {
	r := int(c >> 24 & 0xff)
	g := int(c >> 16 & 0xff)
	b := int(c >> 8 & 0xff)
	return (r*299 + g*587 + b*114) / 1000
}

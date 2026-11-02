package tabs

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// TestTabStripThemePaintsMatchingChrome pins the tab strip side of the theme:
// SetTheme must darken the strip and the active tab body, and in every theme
// the active tab must be the same color as the toolbar below it so the two
// chrome surfaces read as one.
// qc:BR-141
func TestTabStripThemePaintsMatchingChrome(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(nil)
	tab := mgr.NewTab()
	tab.SetTitle("Example")

	cases := []struct {
		theme    string
		barBg    frame.Color
		activeBg frame.Color
	}{
		{"light", frame.RGB(230, 230, 230), frame.RGB(255, 255, 255)},
		{"dark", frame.RGB(32, 32, 33), frame.RGB(24, 24, 24)},
		{"high-contrast", frame.RGB(0, 0, 0), frame.RGB(0, 0, 0)},
	}

	for _, tc := range cases {
		mgr.SetTheme(tc.theme)
		buf := frame.NewBitmap(800, TabBarHeight)
		DrawTabBar(buf, mgr, 0, fonts, 1)

		if got := buf.At(780, 5); got != tc.barBg {
			t.Errorf("%s strip background = %v, want %v", tc.theme, got, tc.barBg)
		}

		r := TabRect(0, 0, 1, 800)
		if got := buf.At(int(r.X0)+3, TabBarHeight-3); got != tc.activeBg {
			t.Errorf("%s active tab fill = %v, want %v", tc.theme, got, tc.activeBg)
		}
	}
}

// TestTabStripThemeDefaultsToLight keeps unknown theme names on the light
// palette.
// qc:BR-141
func TestTabStripThemeDefaultsToLight(t *testing.T) {
	mgr := NewManager(nil)
	mgr.SetTheme("solarized")
	if got := mgr.Theme(); got != "light" {
		t.Errorf("Theme() = %q, want %q", got, "light")
	}
}

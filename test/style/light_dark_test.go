package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestStyle_LightDark_DefaultLight(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div id="target" class="card">Text</div></body></html>`
	doc := domtest.Parse(html)
	cssText := `
    .card {
      color: light-dark(#111111, #eeeeee);
      background-color: light-dark(#ffffff, #000000);
    }
	`
	sheet := css.Parse(cssText)
	vp := style.Viewport{W: 800, H: 600, DarkMode: false}
	styles := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, vp, nil)

	el := doc.ElementByID("target")
	cs := styles[el.ID]

	// Light mode: color is #111111 (17, 17, 17), background is #ffffff (255, 255, 255)
	if cs.Color.R != 17 || cs.Color.G != 17 || cs.Color.B != 17 {
		t.Errorf("expected light color (17, 17, 17), got %+v", cs.Color)
	}
	if cs.BackgroundColor.R != 255 || cs.BackgroundColor.G != 255 || cs.BackgroundColor.B != 255 {
		t.Errorf("expected light background (255, 255, 255), got %+v", cs.BackgroundColor)
	}
}

func TestStyle_LightDark_ViewportDarkMode(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div id="target" class="card">Text</div></body></html>`
	doc := domtest.Parse(html)
	cssText := `
    .card {
      color: light-dark(#111111, #eeeeee);
      background-color: light-dark(#ffffff, #000000);
    }
	`
	sheet := css.Parse(cssText)
	vp := style.Viewport{W: 800, H: 600, DarkMode: true}
	styles := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, vp, nil)

	el := doc.ElementByID("target")
	cs := styles[el.ID]

	// Dark mode: color is #eeeeee (238, 238, 238), background is #000000 (0, 0, 0)
	if cs.Color.R != 238 || cs.Color.G != 238 || cs.Color.B != 238 {
		t.Errorf("expected dark color (238, 238, 238), got %+v", cs.Color)
	}
	if cs.BackgroundColor.R != 0 || cs.BackgroundColor.G != 0 || cs.BackgroundColor.B != 0 {
		t.Errorf("expected dark background (0, 0, 0), got %+v", cs.BackgroundColor)
	}
}

func TestStyle_LightDark_ElementColorSchemeOverride(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <div id="darkContainer" class="dark-box">
    <div id="darkChild" class="card">Dark Child</div>
    <div id="lightChild" class="card light-override">Light Override</div>
  </div>
</body>
</html>`
	doc := domtest.Parse(html)
	cssText := `
    .dark-box {
      color-scheme: dark;
    }
    .light-override {
      color-scheme: light;
    }
    .card {
      color: light-dark(#111111, #eeeeee);
      background-color: light-dark(#ffffff, #000000);
    }
	`
	sheet := css.Parse(cssText)
	// Viewport is light, but container specifies dark
	vp := style.Viewport{W: 800, H: 600, DarkMode: false}
	styles := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, vp, nil)

	darkChild := doc.ElementByID("darkChild")
	csDark := styles[darkChild.ID]
	if csDark.Color.R != 238 || csDark.Color.G != 238 || csDark.Color.B != 238 {
		t.Errorf("darkChild: expected dark color (238, 238, 238), got %+v", csDark.Color)
	}
	if csDark.BackgroundColor.R != 0 || csDark.BackgroundColor.G != 0 || csDark.BackgroundColor.B != 0 {
		t.Errorf("darkChild: expected dark background (0, 0, 0), got %+v", csDark.BackgroundColor)
	}

	lightChild := doc.ElementByID("lightChild")
	csLight := styles[lightChild.ID]
	if csLight.Color.R != 17 || csLight.Color.G != 17 || csLight.Color.B != 17 {
		t.Errorf("lightChild: expected light color (17, 17, 17), got %+v", csLight.Color)
	}
	if csLight.BackgroundColor.R != 255 || csLight.BackgroundColor.G != 255 || csLight.BackgroundColor.B != 255 {
		t.Errorf("lightChild: expected light background (255, 255, 255), got %+v", csLight.BackgroundColor)
	}
}

func TestStyle_ColorSchemeListUsesViewportPreference(t *testing.T) {
	html := `<!DOCTYPE html><html id="root"><body>Readable text</body></html>`
	sheet := css.Parse(`
    html {
      color-scheme: light dark;
      background-color: light-dark(#eeeeee, #222222);
    }
  `)

	for _, tc := range []struct {
		name      string
		darkMode  bool
		wantColor uint8
	}{
		{name: "light preference", darkMode: false, wantColor: 0xee},
		{name: "dark preference", darkMode: true, wantColor: 0x22},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := domtest.Parse(html)
			vp := style.Viewport{W: 800, H: 600, DarkMode: tc.darkMode}
			styles := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, vp, nil)
			root := doc.ElementByID("root")
			cs := styles[root.ID]
			if got := cs.BackgroundColor; got.R != tc.wantColor || got.G != tc.wantColor || got.B != tc.wantColor {
				t.Fatalf("background for darkMode=%t = %+v, want gray #%02x", tc.darkMode, got, tc.wantColor)
			}
		})
	}
}

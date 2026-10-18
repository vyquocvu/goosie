package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestEngine_LightDark_WithDarkMode(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
  <style>
    body {
      background-color: light-dark(#ffffff, #121212);
      color: light-dark(#000000, #ffffff);
    }
    .custom-dark {
      color-scheme: dark;
      background-color: light-dark(#ffffff, #333333);
    }
  </style>
</head>
<body id="b">
  <div id="c" class="custom-dark">Custom</div>
</body>
</html>`

	// 1. Session in Light mode (default)
	sLight, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600), engine.WithDarkMode(false))
	if err != nil {
		t.Fatalf("NewSession (light): %v", err)
	}

	bodyLight := sLight.Doc.ElementByID("b")
	bodyStyleLight := sLight.Styles[bodyLight.ID]
	// Light mode: background #ffffff, color #000000
	if bodyStyleLight.BackgroundColor.R != 255 || bodyStyleLight.BackgroundColor.G != 255 || bodyStyleLight.BackgroundColor.B != 255 {
		t.Errorf("light session body background = %+v, want #ffffff", bodyStyleLight.BackgroundColor)
	}
	if bodyStyleLight.Color.R != 0 || bodyStyleLight.Color.G != 0 || bodyStyleLight.Color.B != 0 {
		t.Errorf("light session body color = %+v, want #000000", bodyStyleLight.Color)
	}

	// Element with explicit color-scheme: dark still resolves dark even in light session
	customLight := sLight.Doc.ElementByID("c")
	customStyleLight := sLight.Styles[customLight.ID]
	if customStyleLight.BackgroundColor.R != 0x33 || customStyleLight.BackgroundColor.G != 0x33 || customStyleLight.BackgroundColor.B != 0x33 {
		t.Errorf("custom-dark in light session background = %+v, want #333333", customStyleLight.BackgroundColor)
	}

	// 2. Session in Dark mode
	sDark, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600), engine.WithDarkMode(true))
	if err != nil {
		t.Fatalf("NewSession (dark): %v", err)
	}

	bodyDark := sDark.Doc.ElementByID("b")
	bodyStyleDark := sDark.Styles[bodyDark.ID]
	// Dark mode: background #121212 (18, 18, 18), color #ffffff
	if bodyStyleDark.BackgroundColor.R != 18 || bodyStyleDark.BackgroundColor.G != 18 || bodyStyleDark.BackgroundColor.B != 18 {
		t.Errorf("dark session body background = %+v, want #121212", bodyStyleDark.BackgroundColor)
	}
	if bodyStyleDark.Color.R != 255 || bodyStyleDark.Color.G != 255 || bodyStyleDark.Color.B != 255 {
		t.Errorf("dark session body color = %+v, want #ffffff", bodyStyleDark.Color)
	}
}

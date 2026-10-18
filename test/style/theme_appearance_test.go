package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCSSPrefersColorSchemeResolution(t *testing.T) {
	sheetText := `
		body { background-color: #ffffff; color: #000000; }
		@media (prefers-color-scheme: dark) {
			body { background-color: #121212; color: #ffffff; }
		}
	`

	// 1. Light Mode (default)
	lightSheet := css.ParseWithOptions(sheetText, css.MediaOptions{ViewportWidth: 800, DarkMode: false})
	if len(lightSheet.Rules) != 1 {
		t.Fatalf("expected 1 rule in light mode, got %d", len(lightSheet.Rules))
	}
	if lightSheet.Rules[0].Declarations[0].Value != "#ffffff" {
		t.Errorf("light mode body background = %q, want #ffffff", lightSheet.Rules[0].Declarations[0].Value)
	}

	// 2. Dark Mode
	darkSheet := css.ParseWithOptions(sheetText, css.MediaOptions{ViewportWidth: 800, DarkMode: true})
	if len(darkSheet.Rules) != 2 {
		t.Fatalf("expected 2 rules in dark mode, got %d", len(darkSheet.Rules))
	}
	lastDecl := darkSheet.Rules[1].Declarations[0]
	if lastDecl.Value != "#121212" {
		t.Errorf("dark mode body background = %q, want #121212", lastDecl.Value)
	}
}

func TestCSSPrefersContrastResolution(t *testing.T) {
	sheetText := `
		.btn { border: 1px solid #ccc; }
		@media (prefers-contrast: more) {
			.btn { border: 3px solid #000; }
		}
	`

	// Normal contrast
	normalSheet := css.ParseWithOptions(sheetText, css.MediaOptions{ViewportWidth: 800, HighContrast: false})
	if len(normalSheet.Rules) != 1 {
		t.Fatalf("expected 1 rule in normal contrast mode, got %d", len(normalSheet.Rules))
	}

	// High contrast
	hcSheet := css.ParseWithOptions(sheetText, css.MediaOptions{ViewportWidth: 800, HighContrast: true})
	if len(hcSheet.Rules) != 2 {
		t.Fatalf("expected 2 rules in high contrast mode, got %d", len(hcSheet.Rules))
	}
}

func TestJSMatchMediaColorScheme(t *testing.T) {
	// Light mode runtime
	rtLight, err := js.New(js.Options{
		DarkMode: false,
	})
	if err != nil {
		t.Fatalf("js.New light failed: %v", err)
	}
	defer rtLight.Close()

	resDark, err := rtLight.Evaluate("window.matchMedia('(prefers-color-scheme: dark)').matches")
	if err != nil || resDark != "false" {
		t.Errorf("light runtime prefers-color-scheme: dark = %v (err: %v), want false", resDark, err)
	}
	resLight, err := rtLight.Evaluate("window.matchMedia('(prefers-color-scheme: light)').matches")
	if err != nil || resLight != "true" {
		t.Errorf("light runtime prefers-color-scheme: light = %v (err: %v), want true", resLight, err)
	}

	// Dark mode runtime
	rtDark, err := js.New(js.Options{
		DarkMode: true,
	})
	if err != nil {
		t.Fatalf("js.New dark failed: %v", err)
	}
	defer rtDark.Close()

	resDark2, err := rtDark.Evaluate("window.matchMedia('(prefers-color-scheme: dark)').matches")
	if err != nil || resDark2 != "true" {
		t.Errorf("dark runtime prefers-color-scheme: dark = %v (err: %v), want true", resDark2, err)
	}
}

func TestEngineSessionWithAppearanceOptions(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head>
<style>
body { background-color: #ffffff; color: #111111; }
@media (prefers-color-scheme: dark) {
  body { background-color: #222222; color: #eeeeee; }
}
</style>
</head><body><p>Hello Theme</p></body></html>`

	// 1. Session with Light Mode
	sessLight, err := engine.NewSession(html, nil, 800, engine.WithDarkMode(false))
	if err != nil {
		t.Fatalf("NewSession light failed: %v", err)
	}
	if sessLight.DarkMode() {
		t.Errorf("expected DarkMode() to be false")
	}

	bodyStyleLight := sessLight.Styles[sessLight.Doc.Body.ID]
	if bodyStyleLight == nil {
		t.Fatal("body style not found")
	}
	if bodyStyleLight.BackgroundColor != (css.Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("light session body background = %v, want #ffffff", bodyStyleLight.BackgroundColor)
	}

	// 2. Session with Dark Mode
	sessDark, err := engine.NewSession(html, nil, 800, engine.WithDarkMode(true))
	if err != nil {
		t.Fatalf("NewSession dark failed: %v", err)
	}
	if !sessDark.DarkMode() {
		t.Errorf("expected DarkMode() to be true")
	}

	bodyStyleDark := sessDark.Styles[sessDark.Doc.Body.ID]
	if bodyStyleDark == nil {
		t.Fatal("body style not found")
	}
	if bodyStyleDark.BackgroundColor != (css.Color{R: 34, G: 34, B: 34, A: 255}) {
		t.Errorf("dark session body background = %v, want #222222", bodyStyleDark.BackgroundColor)
	}
}

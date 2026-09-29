package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

// BackgroundColor tests verify the canvas background derivation. The
// engine checks html's background first, then body's, then falls back
// to white. This matters because a wrong background colour is the most
// visible rendering bug a user can see: every pixel on the canvas
// starts with it.

// TestBackgroundColorDefaultWhite verifies that a document with no
// background styles on either html or body defaults to white. This is
// the common case for most pages and the baseline every other test
// compares against.
func TestBackgroundColorDefaultWhite(t *testing.T) {
	s, err := engine.NewSession(`<html><body><p>hello</p></body></html>`, nil, 800,
		engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	got := s.BackgroundColor()
	want := frame.RGB(255, 255, 255)
	if got != want {
		t.Errorf("BackgroundColor() = %v, want white %v", got, want)
	}
}

// TestBackgroundColorHTMLBodyOverridesDefault verifies that a
// background-color on <html> takes precedence over the white default
// and over any body background.
func TestBackgroundColorHTMLBodyOverridesDefault(t *testing.T) {
	html := `<html style="background-color: rgb(200, 100, 50);"><body><p>hi</p></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	got := s.BackgroundColor()
	want := frame.RGB(200, 100, 50)
	if got != want {
		t.Errorf("BackgroundColor() = %v, want html background %v", got, want)
	}
}

// TestBackgroundColorBodyUsedWhenHTMLTransparent verifies that when
// html has no background (or a transparent one), the body's background
// is used instead.
func TestBackgroundColorBodyUsedWhenHTMLTransparent(t *testing.T) {
	html := `<html><body style="background-color: rgb(10, 20, 30);"><p>hi</p></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	got := s.BackgroundColor()
	want := frame.RGB(10, 20, 30)
	if got != want {
		t.Errorf("BackgroundColor() = %v, want body background %v", got, want)
	}
}

// TestBackgroundColorHTMLBeatsBody verifies that when both html and
// body have backgrounds, html wins. This is the CSS background
// propagation rule: html's background is the canvas background.
func TestBackgroundColorHTMLBeatsBody(t *testing.T) {
	html := `<html style="background-color: red;">` +
		`<body style="background-color: blue;"><p>hi</p></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	got := s.BackgroundColor()
	want := frame.RGB(255, 0, 0)
	if got != want {
		t.Errorf("BackgroundColor() = %v, want html red %v (html should beat body)", got, want)
	}
}

// TestBackgroundColorAuthorCSS verifies that an author stylesheet
// setting the body background is picked up when the inline style on
// html is absent.
func TestBackgroundColorAuthorCSS(t *testing.T) {
	html := `<html><body><p>styled</p></body></html>`
	css := []string{"body { background-color: rgb(42, 84, 126); }"}
	s, err := engine.NewSession(html, css, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	got := s.BackgroundColor()
	want := frame.RGB(42, 84, 126)
	if got != want {
		t.Errorf("BackgroundColor() = %v, want author CSS body background %v", got, want)
	}
}

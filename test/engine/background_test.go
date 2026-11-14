package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

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

func TestBackgroundColorImpliedHtmlBeatsBody(t *testing.T) {
	html := `<!DOCTYPE html><title>t</title><style>html { background-color: green; } body { background-color: red; margin: 0; }</style><p>x</p>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	got := s.BackgroundColor()
	want := frame.RGB(0, 128, 0)
	if got != want {
		t.Errorf("BackgroundColor() = %v, want green %v (implied html beats body)", got, want)
	}
}

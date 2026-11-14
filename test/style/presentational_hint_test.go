package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestBgcolorPresentationalHint(t *testing.T) {
	s := styleFor(t, `<html><body><table><tr><td bgcolor="#ff6600">x</td></tr></table></body></html>`, "", "td")
	want := css.Color{R: 0xff, G: 0x66, B: 0x00, A: 0xff}
	if s.BackgroundColor != want {
		t.Errorf("bgcolor hint BackgroundColor = %+v, want %+v", s.BackgroundColor, want)
	}
}

func TestAuthorCssOverridesBgcolorHint(t *testing.T) {
	s := styleFor(t, `<html><body><table><tr><td class="c" bgcolor="#ff6600">x</td></tr></table></body></html>`,
		`td.c { background-color: #00ff00 }`, "td")
	want := css.Color{R: 0x00, G: 0xff, B: 0x00, A: 0xff}
	if s.BackgroundColor != want {
		t.Errorf("author rule should beat hint: BackgroundColor = %+v, want %+v", s.BackgroundColor, want)
	}
}

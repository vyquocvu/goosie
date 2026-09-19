package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// Legacy presentational attributes paint the page when there is no CSS to
// replace them (news.ycombinator.com's whole header bar is a <td bgcolor>), but
// a real author rule for the same property must still win.
func TestBgcolorPresentationalHint(t *testing.T) {
	s := styleFor(t, `<html><body><table><tr><td bgcolor="#ff6600">x</td></tr></table></body></html>`, "", "td")
	want := css.Color{0xff, 0x66, 0x00, 0xff}
	if s.BackgroundColor != want {
		t.Errorf("bgcolor hint BackgroundColor = %+v, want %+v", s.BackgroundColor, want)
	}
}

func TestAuthorCssOverridesBgcolorHint(t *testing.T) {
	s := styleFor(t, `<html><body><table><tr><td class="c" bgcolor="#ff6600">x</td></tr></table></body></html>`,
		`td.c { background-color: #00ff00 }`, "td")
	want := css.Color{0x00, 0xff, 0x00, 0xff}
	if s.BackgroundColor != want {
		t.Errorf("author rule should beat hint: BackgroundColor = %+v, want %+v", s.BackgroundColor, want)
	}
}

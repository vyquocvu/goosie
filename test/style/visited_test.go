package style_test

import "testing"

func TestVisitedPseudoClassNeverMatches(t *testing.T) {
	const html = `<html><body><a href="#">link</a></body></html>`

	a := styleFor(t, html, `a { color: #3366cc } a:visited { color: #6b4ba3 }`, "a")
	if want := col(0x33, 0x66, 0xcc, 0xff); a.Color != want {
		t.Errorf("a color = %+v, want %+v (the unvisited link color)", a.Color, want)
	}

	b := styleFor(t, html, `a:link { color: #3366cc } a:visited { color: #6b4ba3 }`, "a")
	if want := col(0x33, 0x66, 0xcc, 0xff); b.Color != want {
		t.Errorf("a color = %+v, want %+v", b.Color, want)
	}

	c := styleFor(t, html, `a:visited { color: #6b4ba3 }`, "a")
	if want := col(0x6b, 0x4b, 0xa3, 0xff); c.Color == want {
		t.Errorf("a color = %+v, want it to stay the default", c.Color)
	}
}

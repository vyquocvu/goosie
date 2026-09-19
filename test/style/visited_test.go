package style_test

import "testing"

// TestVisitedPseudoClassNeverMatches pins the link-history question down. The
// engine keeps no browsing history, so no link is ever visited and `a:visited`
// must not match - which is also what a Playwright reference captures, since it
// loads each page in a fresh context. Matching it as `a:link` did let the later
// rule win the cascade, so every link on Wikipedia came out MediaWiki's visited
// purple #6b4ba3 instead of its link blue #3366cc.
func TestVisitedPseudoClassNeverMatches(t *testing.T) {
	const html = `<html><body><a href="#">link</a></body></html>`

	a := styleFor(t, html, `a { color: #3366cc } a:visited { color: #6b4ba3 }`, "a")
	if want := col(0x33, 0x66, 0xcc, 0xff); a.Color != want {
		t.Errorf("a color = %+v, want %+v (the unvisited link color)", a.Color, want)
	}

	// :link is the other half of the pair and still matches a hyperlink.
	b := styleFor(t, html, `a:link { color: #3366cc } a:visited { color: #6b4ba3 }`, "a")
	if want := col(0x33, 0x66, 0xcc, 0xff); b.Color != want {
		t.Errorf("a color = %+v, want %+v", b.Color, want)
	}

	// A visited-only rule must not paint anything at all, so the UA default
	// stands rather than the declaration being applied and then discarded.
	c := styleFor(t, html, `a:visited { color: #6b4ba3 }`, "a")
	if want := col(0x6b, 0x4b, 0xa3, 0xff); c.Color == want {
		t.Errorf("a color = %+v, want it to stay the default", c.Color)
	}
}

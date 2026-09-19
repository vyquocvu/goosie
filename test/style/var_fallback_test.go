package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestUnresolvableVarLeavesThePropertyAlone covers CSS's invalid-at-computed-
// value-time rule. A theme sheet that defines its variables for only one colour
// scheme leaves every other scheme's `color: var(--text-color)` unresolvable,
// and goosie used to substitute an empty string: the property parsed as a zero
// colour, so developer.mozilla.org's inline <code> runs kept their width and
// painted no glyphs at all. Dropping the declaration is what browsers do - the
// property falls back to inherited for an inherited property, and to its
// initial value otherwise.
func TestUnresolvableVarLeavesThePropertyAlone(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`

	p := styleFor(t, html, `body { color: #112233 } p { color: var(--no-such-var) }`, "p")
	if want := col(0x11, 0x22, 0x33, 0xff); p.Color != want {
		t.Errorf("p color = %+v, want %+v (the inherited color)", p.Color, want)
	}

	// The contrast case: a declared fallback is a value, so it wins.
	q := styleFor(t, html, `body { color: #112233 } p { color: var(--no-such-var, #ff0000) }`, "p")
	if want := col(0xff, 0x00, 0x00, 0xff); q.Color != want {
		t.Errorf("p color = %+v, want %+v", q.Color, want)
	}
}

// TestUnparseableColorLeavesTheInheritedOne is the same failure one layer
// deeper. developer.mozilla.org polyfills light-dark() with custom properties
// whose values expand to `initial` beside a colour, and none of that parses as
// a colour - so its inline <code> runs painted a transparent glyph each, which
// reads as a blank gap between the parentheses around them.
func TestUnparseableColorLeavesTheInheritedOne(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`
	for _, value := range []string{"initial", "initial #fff", "notacolor", "light-dark(#000000,#ffffff)"} {
		p := styleFor(t, html, `body { color: #112233 } p { color: `+value+` }`, "p")
		if want := col(0x11, 0x22, 0x33, 0xff); p.Color != want {
			t.Errorf("color: %s -> %+v, want %+v (the inherited color)", value, p.Color, want)
		}
	}
	// `transparent` is a real colour, not a parse failure.
	p := styleFor(t, html, `body { color: #112233 } p { color: transparent }`, "p")
	if want := (css.Color{R: 0, G: 0, B: 0, A: 0}); p.Color != want {
		t.Errorf("color: transparent -> %+v, want %+v", p.Color, want)
	}
}

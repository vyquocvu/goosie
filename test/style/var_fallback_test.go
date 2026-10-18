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
	for _, value := range []string{"initial", "initial #fff", "notacolor", "unsupported-func(#000000,#ffffff)"} {
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

// TestChainedInvalidVarTriggersFallback verifies that when a custom property
// references an unresolvable variable without fallback, it becomes invalid at
// computed-value time, so referencing it with a fallback uses that fallback.
func TestChainedInvalidVarTriggersFallback(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`
	const sheet = `
		:root { --broken: var(--does-not-exist); }
		p { color: var(--broken, #00aa00); }
	`
	p := styleFor(t, html, sheet, "p")
	if want := col(0x00, 0xaa, 0x00, 0xff); p.Color != want {
		t.Errorf("p color = %+v, want %+v", p.Color, want)
	}
}

// TestNestedVarFallbacks verifies deeply chained fallbacks resolve to the final valid value.
func TestNestedVarFallbacks(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`
	const sheet = `
		p { color: var(--missing-a, var(--missing-b, var(--missing-c, #123456))); }
	`
	p := styleFor(t, html, sheet, "p")
	if want := col(0x12, 0x34, 0x56, 0xff); p.Color != want {
		t.Errorf("p color = %+v, want %+v", p.Color, want)
	}
}

// TestCircularCustomPropertiesTriggerFallback verifies that cycles poison variables as
// invalid, allowing fallback values to be used.
func TestCircularCustomPropertiesTriggerFallback(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`
	const sheet = `
		:root {
			--cycle-a: var(--cycle-b);
			--cycle-b: var(--cycle-a);
		}
		p { color: var(--cycle-a, #336699); }
	`
	p := styleFor(t, html, sheet, "p")
	if want := col(0x33, 0x66, 0x99, 0xff); p.Color != want {
		t.Errorf("p color = %+v, want %+v", p.Color, want)
	}
}

// TestMultipleVarsInDeclaration verifies that all vars in a single property are expanded.
func TestMultipleVarsInDeclaration(t *testing.T) {
	const html = `<html><body><div>box</div></body></html>`
	const sheet = `
		:root {
			--top: 10px;
			--right: 20px;
			--bottom: 30px;
			--left: 40px;
		}
		div {
			margin: var(--top) var(--right) var(--bottom) var(--left);
		}
	`
	cs := styleFor(t, html, sheet, "div")
	if cs.MarginTop != 10 || cs.MarginRight != 20 || cs.MarginBottom != 30 || cs.MarginLeft != 40 {
		t.Errorf("margins got (%v, %v, %v, %v), want (10, 20, 30, 40)",
			cs.MarginTop, cs.MarginRight, cs.MarginBottom, cs.MarginLeft)
	}
}

// TestMultipleVarsPartialFailureDropsDeclaration verifies that if any var in a
// declaration fails without fallback, the entire declaration is invalid at computed-value time.
func TestMultipleVarsPartialFailureDropsDeclaration(t *testing.T) {
	const html = `<html><body><div>box</div></body></html>`
	const sheet = `
		:root {
			--top: 10px;
		}
		div {
			margin: 5px;
			margin: var(--top) var(--missing-var);
		}
	`
	cs := styleFor(t, html, sheet, "div")
	// The invalid declaration should be dropped, retaining margin: 5px.
	if cs.MarginTop != 5 || cs.MarginRight != 5 || cs.MarginBottom != 5 || cs.MarginLeft != 5 {
		t.Errorf("margins got (%v, %v, %v, %v), want 5px all around",
			cs.MarginTop, cs.MarginRight, cs.MarginBottom, cs.MarginLeft)
	}
}

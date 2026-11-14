package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestUnresolvableVarLeavesThePropertyAlone(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`

	p := styleFor(t, html, `body { color: #112233 } p { color: var(--no-such-var) }`, "p")
	if want := col(0x11, 0x22, 0x33, 0xff); p.Color != want {
		t.Errorf("p color = %+v, want %+v (the inherited color)", p.Color, want)
	}

	q := styleFor(t, html, `body { color: #112233 } p { color: var(--no-such-var, #ff0000) }`, "p")
	if want := col(0xff, 0x00, 0x00, 0xff); q.Color != want {
		t.Errorf("p color = %+v, want %+v", q.Color, want)
	}
}

func TestUnparseableColorLeavesTheInheritedOne(t *testing.T) {
	const html = `<html><body><p>text</p></body></html>`
	for _, value := range []string{"initial", "initial #fff", "notacolor", "unsupported-func(#000000,#ffffff)"} {
		p := styleFor(t, html, `body { color: #112233 } p { color: `+value+` }`, "p")
		if want := col(0x11, 0x22, 0x33, 0xff); p.Color != want {
			t.Errorf("color: %s -> %+v, want %+v (the inherited color)", value, p.Color, want)
		}
	}
	p := styleFor(t, html, `body { color: #112233 } p { color: transparent }`, "p")
	if want := (css.Color{R: 0, G: 0, B: 0, A: 0}); p.Color != want {
		t.Errorf("color: transparent -> %+v, want %+v", p.Color, want)
	}
}

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
	if cs.MarginTop != 5 || cs.MarginRight != 5 || cs.MarginBottom != 5 || cs.MarginLeft != 5 {
		t.Errorf("margins got (%v, %v, %v, %v), want 5px all around",
			cs.MarginTop, cs.MarginRight, cs.MarginBottom, cs.MarginLeft)
	}
}

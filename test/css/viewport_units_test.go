package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// The cascade rewrites viewport units once per declaration of every element it
// resolves, so the cost of that rewrite is multiplied by the document's size rather
// than by the number of rules that actually use a viewport unit - which in a normal
// page is a small fraction. These tests pin what the rewrite must never do: spend
// work on a value that cannot contain a viewport term.

// noViewportTerm values are ones the rewrite has to leave exactly as authored. Each
// is written so that no digit run is followed by a viewport unit's letters.
var noViewportTerm = []string{
	"2px solid #ccc", "rgba(0, 0, 0, 0.5)", "bold", "0 auto", "#f0f0f2",
	"none", "center", "uppercase", "1.5", "hidden", "600 12px/1.4 Helvetica",
	"", " ", "translate(3px, 4px)",
}

func TestResolveViewportUnitsLeavesValuesWithoutAViewportTerm(t *testing.T) {
	for _, v := range noViewportTerm {
		out, found := css.ResolveViewportUnits(v, 1440, 900)
		if found {
			t.Errorf("ResolveViewportUnits(%q) reported a viewport term; there is none", v)
		}
		if out != v {
			t.Errorf("ResolveViewportUnits(%q) = %q, want it unchanged", v, out)
		}
	}
}

// The load benchmark's memory profile put regexp.(*Regexp).replaceAll eleventh by
// retained-object count for a real page, at three allocations per call even for a
// value that matches nothing - a bitState, an output buffer, and the scan that fills
// them. With the cascade calling this for every non-length declaration of every
// element, that is the third-largest allocation site in loading a document that
// never mentions a viewport unit.
func TestResolveViewportUnitsWithoutAViewportTermIsFree(t *testing.T) {
	for _, v := range noViewportTerm {
		if v == "" {
			continue
		}
		if n := testing.AllocsPerRun(200, func() { _, _ = css.ResolveViewportUnits(v, 1440, 900) }); n != 0 {
			t.Errorf("ResolveViewportUnits(%q) allocated %v times per call; a value with no viewport term must cost no work at all", v, n)
		}
	}
}

// The assertion above is only worth anything if the instrument reports a number when
// there is one to report, so the same measurement over a value that does carry a
// viewport term has to be above zero.
func TestResolveViewportUnitsAllocationsAreNotVacuous(t *testing.T) {
	const expr = "calc(100vw - 20px)"
	if n := testing.AllocsPerRun(200, func() { _, _ = css.ResolveViewportUnits(expr, 1440, 900) }); n == 0 {
		t.Errorf("ResolveViewportUnits(%q) reported no allocations, so the zero above proves nothing", expr)
	}
}

func TestResolveViewportUnitsRewritesEveryUnit(t *testing.T) {
	cases := []struct{ in, want string }{
		{"50vw", "720px"},
		{"50vh", "450px"},
		{"10svw", "144px"},
		{"10svh", "90px"},
		{"10dvw", "144px"},
		{"10dvh", "90px"},
		{"10lvw", "144px"},
		{"10lvh", "90px"},
		{"10vmin", "90px"},
		{"10vmax", "144px"},
		// A viewport unit is a hundredth of the frame, so 100vw becomes the frame's
		// own width and the subtraction is left for the expression evaluator.
		{"calc(100vw - 20px)", "calc(1440px - 20px)"},
		{"0 0 8px 2px rgba(0,0,0,.4)", "0 0 8px 2px rgba(0,0,0,.4)"},
		// A unit inside a quoted string is rewritten too, because the rewrite works
		// on the declaration's text and does not model quoting. That is the behaviour
		// today, and the fast path above must not quietly change it.
		{`"5vw"`, `"72px"`},
		// The pattern is lowercase, so an uppercase unit is not a viewport term at all.
		{"10VH", "10VH"},
	}
	for _, c := range cases {
		out, _ := css.ResolveViewportUnits(c.in, 1440, 900)
		if out != c.want {
			t.Errorf("ResolveViewportUnits(%q) = %q, want %q", c.in, out, c.want)
		}
	}
}

// A hint that is present but is not a unit - the letters "ma" in a colour name - has
// to fall through to the ordinary answer rather than being rewritten.
func TestResolveViewportUnitsFallsThroughOnHintsWithoutUnits(t *testing.T) {
	for _, v := range []string{"maroon", "canvas", "max-content", "middle"} {
		out, found := css.ResolveViewportUnits(v, 1440, 900)
		if found || out != v {
			t.Errorf("ResolveViewportUnits(%q) = (%q, %v), want it unchanged", v, out, found)
		}
	}
}

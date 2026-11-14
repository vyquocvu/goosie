package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

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
		{"calc(100vw - 20px)", "calc(1440px - 20px)"},
		{"0 0 8px 2px rgba(0,0,0,.4)", "0 0 8px 2px rgba(0,0,0,.4)"},
		{`"5vw"`, `"72px"`},
		{"10VH", "10VH"},
	}
	for _, c := range cases {
		out, _ := css.ResolveViewportUnits(c.in, 1440, 900)
		if out != c.want {
			t.Errorf("ResolveViewportUnits(%q) = %q, want %q", c.in, out, c.want)
		}
	}
}

func TestResolveViewportUnitsFallsThroughOnHintsWithoutUnits(t *testing.T) {
	for _, v := range []string{"maroon", "canvas", "max-content", "middle"} {
		out, found := css.ResolveViewportUnits(v, 1440, 900)
		if found || out != v {
			t.Errorf("ResolveViewportUnits(%q) = (%q, %v), want it unchanged", v, out, found)
		}
	}
}

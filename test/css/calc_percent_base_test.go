package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestEvalFuncWithBaseResolvesPercentages guards the layout-time calc path: a
// `%` term resolves against the base the caller supplies (the containing
// length for box sizes, the parent font for font-relative shares), while the
// base-less EvalFunc keeps reporting such expressions unsupported so the
// cascade never answers with a sum that silently dropped the share.
func TestEvalFuncWithBaseResolvesPercentages(t *testing.T) {
	for _, c := range []struct {
		fn, expr string
		base     float32
		want     float32
	}{
		{"calc", "100% - 100px", 760, 660},
		{"calc", "50% + 20px", 760, 400},
		{"calc", "10px * 2", 760, 20},
		{"min", "400px, 80%", 1000, 400},
		{"min", "400px, 80%", 400, 320},
		{"max", "200px, 50%", 1000, 500},
		{"max", "200px, 50%", 300, 200},
		{"clamp", "200px, 60%, 500px", 1000, 500},
		{"clamp", "200px, 60%, 500px", 400, 240},
		{"calc", "min(500px, 80%) - 20px", 1000, 480},
		{"calc", "50% + 2em", 200, 132},
	} {
		got, ok := css.EvalFuncWithBase(c.fn, c.expr, 16, 1440, 900, c.base)
		if !ok {
			t.Errorf("EvalFuncWithBase(%q, %q, base %g) reported itself unsupported", c.fn, c.expr, c.base)
			continue
		}
		if got != c.want {
			t.Errorf("EvalFuncWithBase(%q, %q, base %g) = %g, want %g", c.fn, c.expr, c.base, got, c.want)
		}
	}
}

// TestEvalFuncStillRejectsPercentagesWithoutABase guards the cascade-time
// contract EvalFuncWithBase was split off from: with no containing length in
// scope a percentage keeps reporting unsupported.
func TestEvalFuncStillRejectsPercentagesWithoutABase(t *testing.T) {
	for _, expr := range []string{"100% - 100px", "min(400px, 80%)", "max(200px, 50%)"} {
		if _, ok := css.EvalFunc("calc", expr, 16, 1440, 900); ok {
			t.Errorf("EvalFunc(calc, %q) succeeded without a percentage base", expr)
		}
	}
}

// TestValueFuncHasPercent guards the deferral predicate the cascade uses to
// decide whether a math function stays as text for layout.
func TestValueFuncHasPercent(t *testing.T) {
	if !css.ParseValue("calc(100% - 10px)").FuncHasPercent() {
		t.Error("calc(100% - 10px) should report a percentage term")
	}
	if css.ParseValue("calc(10px * 2)").FuncHasPercent() {
		t.Error("calc(10px * 2) should report no percentage term")
	}
	if css.ParseValue("100px").FuncHasPercent() {
		t.Error("a plain length should report no percentage term")
	}
}

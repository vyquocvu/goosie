package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestEvalCalcSumsUnits(t *testing.T) {
	// The rule a real page writes: a header's parts, added up, in mixed units.
	got, ok := css.EvalCalc("10px + 18px + 0.25rem + 0.25rem", 16, 1280, 800)
	if !ok || got != 36 {
		t.Fatalf("EvalCalc(10px + 18px + 0.25rem + 0.25rem) = %g, ok=%v; want 36", got, ok)
	}
}

func TestEvalCalcPrecedenceAndParentheses(t *testing.T) {
	for _, c := range []struct {
		expr string
		want float32
	}{
		{"100px * 2 - 1em", 184},
		{"(100px + 20px) * 2", 240},
		{"-4px + 10px", 6},
		{"1in - 96px", 0},
		{"50vw / 4", 160},
	} {
		got, ok := css.EvalCalc(c.expr, 16, 1280, 800)
		if !ok {
			t.Errorf("EvalCalc(%q) reported itself unsupported", c.expr)
			continue
		}
		if got != c.want {
			t.Errorf("EvalCalc(%q) = %g, want %g", c.expr, got, c.want)
		}
	}
}

// A percentage inside calc resolves against the containing block, which a
// declaration does not have. Summing the rest would answer with a length that
// silently dropped that term, so such an expression - like a unit the engine
// cannot name - reports itself unsupported.
func TestEvalCalcLeavesUnresolvableTermsUnsupported(t *testing.T) {
	for _, expr := range []string{"50% + 10px", "1lh + 2px", "10px +"} {
		if _, ok := css.EvalCalc(expr, 16, 1280, 800); ok {
			t.Errorf("EvalCalc(%q) reported itself supported", expr)
		}
	}
}

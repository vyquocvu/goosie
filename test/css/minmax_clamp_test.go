package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// clamp(), min() and max() are how modern pages size their type and their gutters:
// CSS-Tricks sets its body text with `clamp(14px,calc(0.8rem + 0.25vw),20px)` and
// MDN's side padding is `max(1rem,calc(50vw - 720px + 1rem))`. Each argument is
// itself an expression, so they share calc()'s grammar.
func TestEvalClampBoundsItsValue(t *testing.T) {
	// 4rem is 64, the middle term 16 + 44.8 = 60.8, so the lower bound wins.
	cases := []struct {
		name, fn, expr string
		want           float32
	}{
		{"lower bound", "clamp", "4rem, 1rem + 3.5vw, 5rem", 64},
		{"upper bound", "clamp", "1rem, 100vw, 3rem", 48},
		{"in range", "clamp", "1rem, 20px + 20px, 3rem", 40},
		{"max of a nested calc", "max", "1rem, calc(50vw - 720px + 1rem)", 16},
		{"min", "min", "20px, 3em", 20},
		{"nested in clamp", "clamp", "10px, max(20px, 1em), 40px", 20},
	}
	for _, c := range cases {
		got, ok := css.EvalFunc(c.fn, c.expr, 16, 1280, 800)
		if !ok {
			t.Errorf("%s: %s(%s) unsupported", c.name, c.fn, c.expr)
			continue
		}
		if d := got - c.want; d > 0.01 || d < -0.01 {
			t.Errorf("%s: %s(%s) = %v, want %v", c.name, c.fn, c.expr, got, c.want)
		}
	}
}

func TestEvalFunctionsRejectWhatTheyCannotAnswer(t *testing.T) {
	for _, c := range []struct{ name, expr string }{
		{"clamp", "1rem, 2rem"},
		{"min", "50%, 10px"},
		{"max", ""},
		{"sheen", "1px"},
	} {
		if v, ok := css.EvalFunc(c.name, c.expr, 16, 1280, 800); ok {
			t.Errorf("%s(%s) = %v, want unsupported", c.name, c.expr, v)
		}
	}
}

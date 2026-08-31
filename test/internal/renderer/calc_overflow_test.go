package renderer_test

import (
	"strings"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// Regression: github.com crashed with a stack overflow. evalCalcExpr's
// handlers required a ')' suffix while parseLength's isCalcExpr dispatch
// matches on the prefix alone, so any function-prefixed fragment with
// trailing tokens or a missing close paren bounced between the two forever.
// Every case here must terminate quickly and return a finite value.
func TestEvalCalcTerminatesOnMalformedInput(t *testing.T) {
	cases := []string{
		"min(320px, 100%) auto",   // trailing token (shorthand/grid split)
		"calc(1px + 2px);",        // trailing semicolon
		"clamp(1rem, 2vw",         // unterminated
		"min(16px, calc(2vw - 1px", // unterminated nested
		"max(calc(100% - 16px",    // unterminated nested max
		"calc(calc(calc(8px",      // nested unterminated
		"min()",                   // empty call
		"clamp(1px)",              // wrong arity
	}
	for _, expr := range cases {
		expr := expr
		t.Run(expr, func(t *testing.T) {
			done := make(chan float32, 1)
			go func() { done <- renderer.EvalCalcExpr(expr, 16, 1280, 800, 800) }()
			select {
			case v := <-done:
				if v != v { // NaN
					t.Errorf("renderer.EvalCalcExpr(%q) = NaN", expr)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("renderer.EvalCalcExpr(%q) did not terminate (stack overflow regression)", expr)
			}
		})
	}
}

func TestEvalCalcDeepNestingTerminates(t *testing.T) {
	deep := strings.Repeat("calc(1px + ", 60) + "1px" + strings.Repeat(")", 60)
	done := make(chan float32, 1)
	go func() { done <- renderer.EvalCalcExpr(deep, 16, 1280, 800, 800) }()
	select {
	case v := <-done:
		if v != v {
			t.Errorf("NaN from deep nesting")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deep nesting did not terminate")
	}
}

// Balanced expressions (including ones with trailing tokens, which the
// balanced-paren parser now tolerates) still evaluate correctly.
func TestEvalCalcBalancedValues(t *testing.T) {
	cases := []struct {
		expr     string
		fontSize float32
		pct      float32
		want     float32
	}{
		{"calc(100% - 16px)", 16, 800, 784},
		{"calc(10px + 5px)", 16, 800, 15},
		{"min(320px, 100%)", 16, 800, 320},
		{"min(320px, 100%)", 16, 200, 200},
		{"max(10px, 2vh)", 16, 800, 16},
		{"clamp(4px, 10vh, 32px)", 16, 800, 32},
	}
	for _, tc := range cases {
		if got := renderer.EvalCalcExpr(tc.expr, tc.fontSize, 1280, 800, tc.pct); got != tc.want {
			t.Errorf("renderer.EvalCalcExpr(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

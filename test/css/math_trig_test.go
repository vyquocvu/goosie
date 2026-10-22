package css_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestTrigonometricFunctions(t *testing.T) {
	tests := []struct {
		expr     string
		expected float32
		delta    float32
	}{
		{"sin(0)", 0.0, 0.001},
		{"sin(30deg)", 0.5, 0.001},
		{"sin(0.52359877)", 0.5, 0.001}, // unitless radians
		{"sin(0.25turn)", 1.0, 0.001},
		{"sin(100grad)", 1.0, 0.001},
		{"sin(pi / 6)", 0.5, 0.001},
		{"cos(0)", 1.0, 0.001},
		{"cos(60deg)", 0.5, 0.001},
		{"cos(pi / 3)", 0.5, 0.001},
		{"cos(180deg)", -1.0, 0.001},
		{"tan(0)", 0.0, 0.001},
		{"tan(45deg)", 1.0, 0.001},
		{"tan(pi / 4)", 1.0, 0.001},
		{"asin(0.5) / 1deg", 30.0, 0.01},
		{"asin(1) / 1deg", 90.0, 0.01},
		{"acos(0.5) / 1deg", 60.0, 0.01},
		{"acos(1) / 1deg", 0.0, 0.01},
		{"atan(1) / 1deg", 45.0, 0.01},
		{"atan(0)", 0.0, 0.001},
		{"atan2(100px, 100px) / 1deg", 45.0, 0.01},
		{"atan2(0, 100px) / 1deg", 0.0, 0.01},
	}

	for _, tc := range tests {
		val, ok := css.EvalCalc(tc.expr, 16, 1440, 900)
		if !ok {
			t.Errorf("EvalCalc(%q) failed unexpectedly", tc.expr)
			continue
		}
		if math.Abs(float64(val-tc.expected)) > float64(tc.delta) {
			t.Errorf("EvalCalc(%q) = %f, want %f (within delta %f)", tc.expr, val, tc.expected, tc.delta)
		}
	}
}

func TestExponentialPowerFunctions(t *testing.T) {
	tests := []struct {
		expr     string
		expected float32
		delta    float32
	}{
		{"pow(2, 3)", 8.0, 0.001},
		{"pow(10, 2)", 100.0, 0.001},
		{"sqrt(16)", 4.0, 0.001},
		{"sqrt(256)", 16.0, 0.001},
		{"hypot(3px, 4px)", 5.0, 0.001},
		{"hypot(6, 8)", 10.0, 0.001},
		{"hypot(1, 2, 2)", 3.0, 0.001}, // 1^2 + 2^2 + 2^2 = 9 -> sqrt(9) = 3
		{"exp(0)", 1.0, 0.001},
		{"exp(1)", float32(math.E), 0.001},
		{"log(1)", 0.0, 0.001},
		{"log(e)", 1.0, 0.001},
		{"log(100, 10)", 2.0, 0.001},
		{"log(8, 2)", 3.0, 0.001},
	}

	for _, tc := range tests {
		val, ok := css.EvalCalc(tc.expr, 16, 1440, 900)
		if !ok {
			t.Errorf("EvalCalc(%q) failed unexpectedly", tc.expr)
			continue
		}
		if math.Abs(float64(val-tc.expected)) > float64(tc.delta) {
			t.Errorf("EvalCalc(%q) = %f, want %f (within delta %f)", tc.expr, val, tc.expected, tc.delta)
		}
	}
}

func TestSignAndSteppedFunctions(t *testing.T) {
	tests := []struct {
		expr     string
		expected float32
		delta    float32
	}{
		{"abs(-15px)", 15.0, 0.001},
		{"abs(15px)", 15.0, 0.001},
		{"abs(0)", 0.0, 0.001},
		{"sign(-42px)", -1.0, 0.001},
		{"sign(42px)", 1.0, 0.001},
		{"sign(0)", 0.0, 0.001},
		{"round(124px, 50px)", 100.0, 0.001},
		{"round(126px, 50px)", 150.0, 0.001},
		{"round(up, 121px, 50px)", 150.0, 0.001},
		{"round(down, 129px, 50px)", 100.0, 0.001},
		{"round(to-zero, -129px, 50px)", -100.0, 0.001},
		{"round(14.8)", 15.0, 0.001},
		{"mod(18px, 5px)", 3.0, 0.001},
		{"rem(18px, 5px)", 3.0, 0.001},
		{"mod(-18px, 5px)", 2.0, 0.001},
		{"rem(-18px, 5px)", -3.0, 0.001},
	}

	for _, tc := range tests {
		val, ok := css.EvalCalc(tc.expr, 16, 1440, 900)
		if !ok {
			t.Errorf("EvalCalc(%q) failed unexpectedly", tc.expr)
			continue
		}
		if math.Abs(float64(val-tc.expected)) > float64(tc.delta) {
			t.Errorf("EvalCalc(%q) = %f, want %f (within delta %f)", tc.expr, val, tc.expected, tc.delta)
		}
	}
}

func TestMathFunctionsInValueLength(t *testing.T) {
	// Direct length function evaluation
	v1 := css.ParseValue("sin(30deg) * 100px")
	// Or inside calc
	v2 := css.ParseValue("calc(100px * sin(30deg))")
	if l := v2.ToLength(); math.Abs(float64(l-50.0)) > 0.01 {
		t.Errorf("ToLength() = %f, want 50.0", l)
	}

	v3 := css.ParseValue("hypot(60px, 80px)")
	if l := v3.ToLength(); math.Abs(float64(l-100.0)) > 0.01 {
		t.Errorf("hypot ToLength() = %f, want 100.0", l)
	}

	v4 := css.ParseValue("abs(-75px)")
	if l := v4.ToLength(); l != 75.0 {
		t.Errorf("abs ToLength() = %f, want 75.0", l)
	}

	_ = v1
}

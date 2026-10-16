package css_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestParseTransitionShorthandBasic verifies that a simple transition shorthand
// parses the property, duration, timing function, and delay correctly.
func TestParseTransitionShorthandBasic(t *testing.T) {
	transitions := css.ParseTransitionShorthand("background 0.3s ease")
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	tr := transitions[0]
	if tr.Property != "background" {
		t.Errorf("Property = %q, want %q", tr.Property, "background")
	}
	if math.Abs(tr.Duration-0.3) > 0.001 {
		t.Errorf("Duration = %v, want 0.3", tr.Duration)
	}
	if tr.Timing != css.TimingEase {
		t.Errorf("Timing = %v, want TimingEase", tr.Timing)
	}
	if tr.Delay != 0 {
		t.Errorf("Delay = %v, want 0", tr.Delay)
	}
}

// TestParseTransitionShorthandAll verifies that "all" is the default property.
func TestParseTransitionShorthandAll(t *testing.T) {
	transitions := css.ParseTransitionShorthand("0.5s linear")
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	if transitions[0].Property != "all" {
		t.Errorf("Property = %q, want %q", transitions[0].Property, "all")
	}
	if transitions[0].Timing != css.TimingLinear {
		t.Errorf("Timing = %v, want TimingLinear", transitions[0].Timing)
	}
}

// TestParseTransitionShorthandMultiple verifies comma-separated transitions.
func TestParseTransitionShorthandMultiple(t *testing.T) {
	transitions := css.ParseTransitionShorthand("background 0.3s ease, color 0.5s linear")
	if len(transitions) != 2 {
		t.Fatalf("got %d transitions, want 2", len(transitions))
	}
	if transitions[0].Property != "background" {
		t.Errorf("[0].Property = %q, want %q", transitions[0].Property, "background")
	}
	if transitions[1].Property != "color" {
		t.Errorf("[1].Property = %q, want %q", transitions[1].Property, "color")
	}
	if math.Abs(transitions[1].Duration-0.5) > 0.001 {
		t.Errorf("[1].Duration = %v, want 0.5", transitions[1].Duration)
	}
}

// TestParseTransitionShorthandDelay verifies the optional delay parameter.
func TestParseTransitionShorthandDelay(t *testing.T) {
	transitions := css.ParseTransitionShorthand("opacity 0.3s ease-in 0.1s")
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	tr := transitions[0]
	if tr.Property != "opacity" {
		t.Errorf("Property = %q, want %q", tr.Property, "opacity")
	}
	if tr.Timing != css.TimingEaseIn {
		t.Errorf("Timing = %v, want TimingEaseIn", tr.Timing)
	}
	if math.Abs(tr.Delay-0.1) > 0.001 {
		t.Errorf("Delay = %v, want 0.1", tr.Delay)
	}
}

// TestParseTransitionShorthandMs verifies millisecond time units.
func TestParseTransitionShorthandMs(t *testing.T) {
	transitions := css.ParseTransitionShorthand("background 300ms ease-out")
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	if math.Abs(transitions[0].Duration-0.3) > 0.001 {
		t.Errorf("Duration = %v, want 0.3 (300ms)", transitions[0].Duration)
	}
	if transitions[0].Timing != css.TimingEaseOut {
		t.Errorf("Timing = %v, want TimingEaseOut", transitions[0].Timing)
	}
}

// TestParseTransitionShorthandNone verifies that "none" returns no transitions.
func TestParseTransitionShorthandNone(t *testing.T) {
	transitions := css.ParseTransitionShorthand("none")
	if len(transitions) != 0 {
		t.Errorf("got %d transitions for 'none', want 0", len(transitions))
	}
}

// TestParseTransitionShorthandEmpty verifies that an empty value returns no transitions.
func TestParseTransitionShorthandEmpty(t *testing.T) {
	transitions := css.ParseTransitionShorthand("")
	if len(transitions) != 0 {
		t.Errorf("got %d transitions for empty, want 0", len(transitions))
	}
}

// TestParseTransitionShorthandEaseInOut verifies the ease-in-out timing function.
func TestParseTransitionShorthandEaseInOut(t *testing.T) {
	transitions := css.ParseTransitionShorthand("all 1s ease-in-out")
	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	if transitions[0].Timing != css.TimingEaseInOut {
		t.Errorf("Timing = %v, want TimingEaseInOut", transitions[0].Timing)
	}
}

// TestParseTimingFunc verifies the ParseTimingFunc function.
func TestParseTimingFunc(t *testing.T) {
	tests := []struct {
		input string
		want  css.TimingFunc
	}{
		{"ease", css.TimingEase},
		{"linear", css.TimingLinear},
		{"ease-in", css.TimingEaseIn},
		{"ease-out", css.TimingEaseOut},
		{"ease-in-out", css.TimingEaseInOut},
		{"EASE", css.TimingEase},     // case insensitive
		{"Linear", css.TimingLinear}, // case insensitive
		{"unknown", css.TimingEase},  // unknown defaults to ease
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := css.ParseTimingFunc(tc.input)
			if got != tc.want {
				t.Errorf("ParseTimingFunc(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestTimingFuncEval verifies that timing functions produce correct eased values.
func TestTimingFuncEval(t *testing.T) {
	// Linear should be identity.
	for _, v := range []float64{0, 0.25, 0.5, 0.75, 1.0} {
		got := css.TimingLinear.Eval(v)
		if math.Abs(got-v) > 0.001 {
			t.Errorf("Linear.Eval(%v) = %v, want %v", v, got, v)
		}
	}

	// All timing functions should return 0 at t=0 and 1 at t=1.
	for _, tf := range []css.TimingFunc{css.TimingEase, css.TimingLinear, css.TimingEaseIn, css.TimingEaseOut, css.TimingEaseInOut} {
		if got := tf.Eval(0); got != 0 {
			t.Errorf("TimingFunc(%v).Eval(0) = %v, want 0", tf, got)
		}
		if got := tf.Eval(1); got != 1 {
			t.Errorf("TimingFunc(%v).Eval(1) = %v, want 1", tf, got)
		}
	}

	// EaseIn should be slower at start (t=0.5 should be < 0.5).
	if got := css.TimingEaseIn.Eval(0.5); got >= 0.5 {
		t.Errorf("EaseIn.Eval(0.5) = %v, want < 0.5", got)
	}

	// EaseOut should be faster at start (t=0.5 should be > 0.5).
	if got := css.TimingEaseOut.Eval(0.5); got <= 0.5 {
		t.Errorf("EaseOut.Eval(0.5) = %v, want > 0.5", got)
	}
}

// TestParseTransitionProperty verifies the transition-property longhand.
func TestParseTransitionProperty(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"all", []string{"all"}},
		{"none", nil},
		{"background, color", []string{"background", "color"}},
		{"opacity", []string{"opacity"}},
		{"", nil},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := css.ParseTransitionProperty(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseTransitionProperty(%q) = %v (len %d), want %v (len %d)",
					tc.input, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestParseTransitionDuration verifies the transition-duration longhand.
func TestParseTransitionDuration(t *testing.T) {
	durations := css.ParseTransitionDuration("0.3s, 500ms")
	if len(durations) != 2 {
		t.Fatalf("got %d durations, want 2", len(durations))
	}
	if math.Abs(durations[0]-0.3) > 0.001 {
		t.Errorf("[0] = %v, want 0.3", durations[0])
	}
	if math.Abs(durations[1]-0.5) > 0.001 {
		t.Errorf("[1] = %v, want 0.5", durations[1])
	}
}

// TestParseTransitionDelay verifies the transition-delay longhand.
func TestParseTransitionDelay(t *testing.T) {
	delays := css.ParseTransitionDelay("0.1s, 200ms")
	if len(delays) != 2 {
		t.Fatalf("got %d delays, want 2", len(delays))
	}
	if math.Abs(delays[0]-0.1) > 0.001 {
		t.Errorf("[0] = %v, want 0.1", delays[0])
	}
	if math.Abs(delays[1]-0.2) > 0.001 {
		t.Errorf("[1] = %v, want 0.2", delays[1])
	}
}

// TestParseTransitionTimingFunction verifies the transition-timing-function longhand.
func TestParseTransitionTimingFunction(t *testing.T) {
	funcs := css.ParseTransitionTimingFunction("ease-in, linear, ease-out")
	if len(funcs) != 3 {
		t.Fatalf("got %d timing functions, want 3", len(funcs))
	}
	if funcs[0] != css.TimingEaseIn {
		t.Errorf("[0] = %v, want TimingEaseIn", funcs[0])
	}
	if funcs[1] != css.TimingLinear {
		t.Errorf("[1] = %v, want TimingLinear", funcs[1])
	}
	if funcs[2] != css.TimingEaseOut {
		t.Errorf("[2] = %v, want TimingEaseOut", funcs[2])
	}
}

// TestTransitionInStylesheet verifies that transition declarations parse
// correctly when embedded in a full stylesheet.
func TestTransitionInStylesheet(t *testing.T) {
	sheet := css.Parse(`
		.box {
			background: red;
			transition: background 0.3s ease;
		}
		.box:hover {
			background: blue;
		}
	`)
	if len(sheet.Rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(sheet.Rules))
	}

	// Find the transition declaration.
	found := false
	for _, d := range sheet.Rules[0].Declarations {
		if d.Property == "transition" {
			found = true
			transitions := css.ParseTransitionShorthand(d.Value)
			if len(transitions) != 1 {
				t.Fatalf("got %d transitions, want 1", len(transitions))
			}
			if transitions[0].Property != "background" {
				t.Errorf("Property = %q, want %q", transitions[0].Property, "background")
			}
			if math.Abs(transitions[0].Duration-0.3) > 0.001 {
				t.Errorf("Duration = %v, want 0.3", transitions[0].Duration)
			}
		}
	}
	if !found {
		t.Error("transition declaration not found in stylesheet")
	}
}

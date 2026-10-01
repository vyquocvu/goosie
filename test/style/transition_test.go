package style_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestTransitionShorthandComputedStyle verifies that the transition shorthand
// is parsed and stored on the ComputedStyle.
func TestTransitionShorthandComputedStyle(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: background 0.3s ease }`, "div")
	if len(s.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(s.Transitions))
	}
	tr := s.Transitions[0]
	if tr.Property != "background" {
		t.Errorf("Property = %q, want %q", tr.Property, "background")
	}
	if math.Abs(tr.Duration-0.3) > 0.001 {
		t.Errorf("Duration = %v, want 0.3", tr.Duration)
	}
	if tr.Timing != css.TimingEase {
		t.Errorf("Timing = %v, want TimingEase", tr.Timing)
	}
}

// TestTransitionMultipleComputedStyle verifies multiple comma-separated transitions.
func TestTransitionMultipleComputedStyle(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: background 0.3s ease, color 0.5s linear }`, "div")
	if len(s.Transitions) != 2 {
		t.Fatalf("got %d transitions, want 2", len(s.Transitions))
	}
	if s.Transitions[0].Property != "background" {
		t.Errorf("[0].Property = %q, want %q", s.Transitions[0].Property, "background")
	}
	if s.Transitions[1].Property != "color" {
		t.Errorf("[1].Property = %q, want %q", s.Transitions[1].Property, "color")
	}
	if math.Abs(s.Transitions[1].Duration-0.5) > 0.001 {
		t.Errorf("[1].Duration = %v, want 0.5", s.Transitions[1].Duration)
	}
}

// TestTransitionNoneComputedStyle verifies that "none" produces no transitions.
func TestTransitionNoneComputedStyle(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: none }`, "div")
	if len(s.Transitions) != 0 {
		t.Errorf("got %d transitions for 'none', want 0", len(s.Transitions))
	}
}

// TestTransitionWithDelayComputedStyle verifies the delay parameter.
func TestTransitionWithDelayComputedStyle(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: opacity 0.3s ease-in 0.1s }`, "div")
	if len(s.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(s.Transitions))
	}
	tr := s.Transitions[0]
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

// TestTransitionDefaultProperty verifies that omitting the property defaults to "all".
func TestTransitionDefaultProperty(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: 0.5s linear }`, "div")
	if len(s.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(s.Transitions))
	}
	if s.Transitions[0].Property != "all" {
		t.Errorf("Property = %q, want %q", s.Transitions[0].Property, "all")
	}
}

// TestTransitionMsUnitComputedStyle verifies millisecond time units.
func TestTransitionMsUnitComputedStyle(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: background 300ms ease-out }`, "div")
	if len(s.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(s.Transitions))
	}
	if math.Abs(s.Transitions[0].Duration-0.3) > 0.001 {
		t.Errorf("Duration = %v, want 0.3 (300ms)", s.Transitions[0].Duration)
	}
	if s.Transitions[0].Timing != css.TimingEaseOut {
		t.Errorf("Timing = %v, want TimingEaseOut", s.Transitions[0].Timing)
	}
}

// TestTransitionEaseInOutComputedStyle verifies the ease-in-out timing function.
func TestTransitionEaseInOutComputedStyle(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { transition: all 1s ease-in-out }`, "div")
	if len(s.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(s.Transitions))
	}
	if s.Transitions[0].Timing != css.TimingEaseInOut {
		t.Errorf("Timing = %v, want TimingEaseInOut", s.Transitions[0].Timing)
	}
}

// TestNoTransitionDefault verifies that elements without transition declarations
// have no transitions.
func TestNoTransitionDefault(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="box">x</div></body></html>`,
		`.box { background: red }`, "div")
	if len(s.Transitions) != 0 {
		t.Errorf("got %d transitions, want 0 (no transition declared)", len(s.Transitions))
	}
}

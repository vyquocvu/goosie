package style_test

import (
	"testing"
)

// BR-074 gradient parse: a stop position written as the CSS unitless zero
// (`#0a0 0`) is a legal <length>, so the ramp is kept; a unitless non-zero
// (`#0a0 50`) is not, so the whole background is dropped. Both halves are
// measured, not recalled: scripts/verify-br074-color-idle-scroll.js prints
// Chromium 1.63 on 2026-11-04 answering
//
//	SYNTAX unitless-zero           kept=YES computed=linear-gradient(rgb(0, 170, 0) 0px, rgb(170, 0, 0) 100%)
//	SYNTAX double-unitless-zero    kept=YES computed=linear-gradient(rgb(17, 17, 17) 0px, rgb(17, 17, 17) 50%, …
//	SYNTAX unitless-fifty-invalid  kept=DROPPED computed=none pixels rgb(255,255,255) ×3
//
// Goosie read `0` as part of the colour instead, so `#0a0 0` failed ParseColor
// and parseLinearGradientArgs returned an empty ramp: the author's gradient
// painted NOTHING - not a wrong colour, no colour at all.

func stopPositions(t *testing.T, css string) []float32 {
	t.Helper()
	s := styleFor(t, `<html><body><div>x</div></body></html>`, css, "div")
	var at []float32
	for _, st := range s.BackgroundGradient.Stops {
		at = append(at, st.At)
	}
	return at
}

// `0` as a stop position is the zero end of the line, exactly as `0%` is.
//
// qc:BR-074
func TestGradientUnitlessZeroStopPosition(t *testing.T) {
	got := stopPositions(t, `div { background: linear-gradient(180deg,#0a0 0,#a00 100%); }`)
	want := []float32{0, 1}
	if len(got) != len(want) {
		t.Fatalf("stops = %v, want the two ends %v: a unitless zero position is a legal <length>", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stop %d sits at %v, want %v", i, got[i], want[i])
		}
	}
	s := styleFor(t, `<html><body><div>x</div></body></html>`,
		`div { background: linear-gradient(180deg,#0a0 0,#a00 100%); }`, "div")
	first, last := s.BackgroundGradient.Stops[0].Color, s.BackgroundGradient.Stops[1].Color
	if first.G != 170 || last.R != 170 {
		t.Errorf("ramp colours are %+v and %+v, want rgb(0,170,0) then rgb(170,0,0)", first, last)
	}
}

// A unitless zero also heads a double position - `#111 0 50%` is the solid band
// from the top of the line to halfway, which the reference paints as one flat
// colour above the mid line and another below it.
//
// qc:BR-074
func TestGradientDoublePositionWithUnitlessZero(t *testing.T) {
	got := stopPositions(t, `div { background: linear-gradient(#111 0 50%,#eee 50% 100%); }`)
	want := []float32{0, 0.5, 0.5, 1}
	if len(got) != len(want) {
		t.Fatalf("stops = %v, want the four band edges %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("band edge %d sits at %v, want %v", i, got[i], want[i])
		}
	}
}

// The guard the fix must not blow past: `50` is not a <length-percentage>, so
// Chromium drops the declaration and this engine must not invent a ramp.
//
// qc:BR-074
func TestGradientUnitlessNonZeroStopPositionIsInvalid(t *testing.T) {
	if got := stopPositions(t, `div { background: linear-gradient(180deg,#0a0 50,#a00); }`); len(got) != 0 {
		t.Errorf("stops = %v for an invalid unitless position, want no ramp at all", got)
	}
}

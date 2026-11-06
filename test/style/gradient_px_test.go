package style_test

import "testing"

// BR-074's length half: a stop position written as a CSS <length> - `0px`,
// `30px` - is a legal <length-percentage>, and Chromium keeps and paints the
// ramp. The cascade cannot turn a length into a fraction of the gradient line
// because the line's length belongs to the laid-out box, so the length rides
// on the stop unresolved and paint resolves it against the box it measures.
// Until that carrying existed, ParseColor saw `#0a0 0px` and dropped the whole
// ramp: the author's gradient painted NOTHING - not a wrong colour, no colour
// at all.
//
// Measured scripts/verify-br074-color-idle-scroll.js (Chromium 1.63, this
// host, 2026-11-06):
//
//	SYNTAX px-ends kept=YES computed=linear-gradient(rgb(0, 170, 0) 0px, rgb(170, 0, 0) 60px)
//	SYNTAX px-mid  kept=YES computed=linear-gradient(rgb(255, 0, 0), rgb(0, 255, 0) 30px, rgb(0, 0, 255) 60px)

// qc:BR-074
func TestGradientPxStopPositionsRideTheStopUnresolved(t *testing.T) {
	s := styleFor(t, `<html><body><div>x</div></body></html>`,
		`div { background: linear-gradient(180deg,#0a0 0px,#a00 60px); }`, "div")
	stops := s.BackgroundGradient.Stops
	if len(stops) != 2 {
		t.Fatalf("stops = %d, want two: a px position is a legal <length-percentage>, not a parse failure", len(stops))
	}
	for i, want := range []float32{0, 60} {
		if stops[i].Px != want || !stops[i].PxSet {
			t.Errorf("stop %d carries px %v set=%v, want %v set", i, stops[i].Px, stops[i].PxSet, want)
		}
	}
	if stops[0].Color.G != 170 || stops[1].Color.R != 170 {
		t.Errorf("ramp colours are %+v and %+v, want rgb(0,170,0) then rgb(170,0,0)", stops[0].Color, stops[1].Color)
	}
}

// A ramp mixing fractions and lengths cannot be distributed at cascade time:
// the fractions are set, the lengths wait, and a stop the declaration left
// unpositioned keeps its -1 until paint has resolved the lengths against the
// box and can distribute around them.
//
// qc:BR-074
func TestGradientMixedLengthAndFractionRampDefersDistribution(t *testing.T) {
	s := styleFor(t, `<html><body><div>x</div></body></html>`,
		`div { background: linear-gradient(#0a0 0%, #a00 30px, #00b); }`, "div")
	stops := s.BackgroundGradient.Stops
	if len(stops) != 3 {
		t.Fatalf("stops = %d, want three", len(stops))
	}
	if stops[0].At != 0 || stops[0].PxSet {
		t.Errorf("stop 0 = at %v px %v set %v, want the 0%% fraction", stops[0].At, stops[0].Px, stops[0].PxSet)
	}
	if !stops[1].PxSet || stops[1].Px != 30 || stops[1].At != -1 {
		t.Errorf("stop 1 = at %v px %v set %v, want the 30px length unresolved", stops[1].At, stops[1].Px, stops[1].PxSet)
	}
	if stops[2].At != -1 || stops[2].PxSet {
		t.Errorf("stop 2 = at %v px %v set %v, want unpositioned until paint distributes", stops[2].At, stops[2].Px, stops[2].PxSet)
	}
}

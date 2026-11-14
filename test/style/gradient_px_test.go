package style_test

import "testing"

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

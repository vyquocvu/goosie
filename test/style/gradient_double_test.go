package style_test

import (
	"testing"
)

// TestGradientDoublePositionStops pins solid-band expansion: `yellow 0%
// 25%` is yellow from 0 to 0.25, so four bands need eight stops.
func TestGradientDoublePositionStops(t *testing.T) {
	s := styleFor(t, `<html><body><div>x</div></body></html>`,
		`div { background-image: linear-gradient(to right, yellow 0% 25%, purple 25% 50%, yellow 50% 75%, purple 75% 100%); }`, "div")
	got := s.BackgroundGradient.Stops
	want := []float32{0, 0.25, 0.25, 0.5, 0.5, 0.75, 0.75, 1}
	if len(got) != len(want) {
		t.Fatalf("stops = %v, want 8 band edges", got)
	}
	for i := range want {
		if got[i].At != want[i] {
			t.Errorf("stop %d at %v, want %v", i, got[i].At, want[i])
		}
	}
	if got[0].Color.G != 255 || got[2].Color.B != 128 {
		t.Errorf("band colors wrong: %+v", got)
	}
}

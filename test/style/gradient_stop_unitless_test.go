package style_test

import (
	"testing"
)

func stopPositions(t *testing.T, css string) []float32 {
	t.Helper()
	s := styleFor(t, `<html><body><div>x</div></body></html>`, css, "div")
	var at []float32
	for _, st := range s.BackgroundGradient.Stops {
		at = append(at, st.At)
	}
	return at
}

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

// qc:BR-074
func TestGradientUnitlessNonZeroStopPositionIsInvalid(t *testing.T) {
	if got := stopPositions(t, `div { background: linear-gradient(180deg,#0a0 50,#a00); }`); len(got) != 0 {
		t.Errorf("stops = %v for an invalid unitless position, want no ramp at all", got)
	}
}

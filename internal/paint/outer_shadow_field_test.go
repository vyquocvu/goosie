package paint

import "testing"

// outerAxisProfile is the measured falloff, and the collapsed case is the part of
// it that has to be stated rather than left to arithmetic: when a negative spread
// takes the shadow rect's end before its start, the two distribution terms swap
// and their difference goes negative. A negative float converting to a uint8 mask
// byte is implementation-defined (Go spec, Conversions), so "the clamp turns it
// into nothing" is a fact about this host, not a law. The law is measured: Chromium
// leaves every pixel of `0 0 16px -30px` on a 120x60 box at the page's own colour,
// and so does a zero-size box declaring a blur, whose rect is a point.
// qc:BR-281
func TestOuterAxisProfileOfACollapsedRectIsZeroEverywhere(t *testing.T) {
	for _, tc := range []struct {
		name          string
		start, lo, hi int32
		n             int
	}{
		{"spread past the left edge", 0, 40, 10, 64},
		{"spread exactly inverted", 0, 10, 10, 32},
		{"a point rect at the origin", -8, 0, 0, 24},
	} {
		got := outerAxisProfile(tc.start, tc.n, tc.lo, tc.hi, 4)
		for i, v := range got {
			if v != 0 {
				t.Errorf("%s: profile[%d] = %v, want 0: a rect with no span has nothing to blur, "+
					"and a negative value would reach the mask as an implementation-defined byte", tc.name, i, v)
				break
			}
		}
	}
}

// The same function on a rect that does have a span must stay a probability: never
// negative, never above 1, and heaviest inside the rect. That is what makes the
// mask byte a coverage and not an artefact of the subtraction.
// qc:BR-281
func TestOuterAxisProfileStaysACoverageOnARealSpan(t *testing.T) {
	const lo, hi = int32(20), int32(40)
	got := outerAxisProfile(0, 64, lo, hi, 4)
	peak := 0
	for i, v := range got {
		if v < 0 || v > 1 {
			t.Fatalf("profile[%d] = %v: coverage must be a probability", i, v)
		}
		if v > got[peak] {
			peak = i
		}
	}
	if int32(peak) < lo || int32(peak) >= hi {
		t.Errorf("the profile peaks at x=%d, want it inside the shadow rect (%d,%d)", peak, lo, hi)
	}
	for i := 0; i < int(lo); i++ {
		if got[i] > got[i+1] {
			t.Errorf("coverage falls from x=%d to x=%d on the approach to the rect", i, i+1)
		}
	}
	for i := int(hi); i < 63; i++ {
		if got[i] < got[i+1] {
			t.Errorf("coverage rises from x=%d to x=%d past the rect's far edge", i, i+1)
		}
	}
}

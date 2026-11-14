package paint

import "testing"

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

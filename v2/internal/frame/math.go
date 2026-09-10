package frame

import "math"

// floor32 and ceil32 exist because the rounding direction at a device-pixel
// boundary is a correctness decision, not an implementation detail: clips round
// inward, coverage bounds round outward. They are written with an explicit
// float64 round trip rather than int32(v) because converting a negative float
// to an integer truncates toward zero, which is neither floor nor ceil.

func floor32(v float32) int32 { return int32(math.Floor(float64(v))) }

func ceil32(v float32) int32 { return int32(math.Ceil(float64(v))) }

// round32 rounds half away from zero, used for glyph positions where the
// nearest pixel is the visually correct choice.
func round32(v float32) int32 { return int32(math.Round(float64(v))) }

// floorDiv divides a by b rounding toward negative infinity. Go's / operator
// truncates toward zero, which would map content at x = -1 into tile 0 next to
// tile -1, so a tile lookup on negative coordinates has to be explicit.
func floorDiv(a, b int32) int32 {
	if b <= 0 {
		return 0
	}
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

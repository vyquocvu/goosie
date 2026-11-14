package frame

import "math"

func floor32(v float32) int32 { return int32(math.Floor(float64(v))) }

func ceil32(v float32) int32 { return int32(math.Ceil(float64(v))) }

func round32(v float32) int32 { return int32(math.Round(float64(v))) }

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

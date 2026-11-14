package style

import "github.com/vyquocvu/goosie/internal/frame"

func (s *ComputedStyle) UsedBorderRadius(w, h, scale float32) frame.Corners {
	if s == nil {
		return frame.Corners{}
	}
	declared := s.BorderRadius
	if declared == ([4][2]float32{}) {
		return frame.Corners{}
	}
	sideW, sideH := float64(w), float64(h)
	resolve := func(v float32, side float64) float64 {
		if v < -2 {
			return float64(-2-v) * side / 100
		}
		if v <= 0 {
			return 0
		}
		return float64(v) * float64(scale)
	}
	var c [4][2]float64
	for i := 0; i < 4; i++ {
		c[i][0] = resolve(declared[i][0], sideW)
		c[i][1] = resolve(declared[i][1], sideH)
	}
	fit := func(sum, side float64) float64 {
		if sum <= side || sum <= 0 {
			return 1
		}
		return side / sum
	}
	k := fit(c[0][0]+c[1][0], sideW)
	if v := fit(c[3][0]+c[2][0], sideW); v < k {
		k = v
	}
	if v := fit(c[0][1]+c[3][1], sideH); v < k {
		k = v
	}
	if v := fit(c[1][1]+c[2][1], sideH); v < k {
		k = v
	}
	rad := func(i int) frame.Radius {
		return frame.Radius{X: float32(c[i][0] * k), Y: float32(c[i][1] * k)}
	}
	return frame.Corners{TL: rad(0), TR: rad(1), BR: rad(2), BL: rad(3)}
}

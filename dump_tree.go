package main

import (
	"fmt"
	"math"
)

func clamp(val, min, max float32) float32 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func oklabToSrgb(l, a, b float32) (r, g, bOut float32) {
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b

	lv := l_ * l_ * l_
	mv := m_ * m_ * m_
	sv := s_ * s_ * s_

	lr := +4.0767416621*lv - 3.3077115913*mv + 0.2309699292*sv
	lg := -1.2684380046*lv + 2.6097574011*mv - 0.3413193965*sv
	lb := -0.0041960863*lv - 0.7034186147*mv + 1.7076147010*sv

	gammaCorrect := func(c float32) float32 {
		if c <= 0.0031308 {
			return 12.92 * c
		}
		return float32(1.055*math.Pow(float64(c), 1.0/2.4) - 0.055)
	}

	r = gammaCorrect(lr)
	g = gammaCorrect(lg)
	bOut = gammaCorrect(lb)
	return
}

func parseOklch(l, c, h float32) (uint8, uint8, uint8) {
	hRad := h * 3.14159265358979 / 180.0
	a := c * float32(math.Cos(float64(hRad)))
	b := c * float32(math.Sin(float64(hRad)))

	r, g, bv := oklabToSrgb(l, a, b)
	return uint8(clamp(r*255, 0, 255)), uint8(clamp(g*255, 0, 255)), uint8(clamp(bv*255, 0, 255))
}

func main() {
	bgR, bgG, bgB := parseOklch(0.95, 0.005, 220)
	txtR, txtG, txtB := parseOklch(0.45, 0.01, 107)
	lnkR, lnkG, lnkB := parseOklch(0.58, 0.14, 251)
	hlR, hlG, hlB := parseOklch(0.979, 0.129, 108)

	fmt.Printf("bg: rgb(%d, %d, %d)\n", bgR, bgG, bgB)
	fmt.Printf("text: rgb(%d, %d, %d)\n", txtR, txtG, txtB)
	fmt.Printf("link: rgb(%d, %d, %d)\n", lnkR, lnkG, lnkB)
	fmt.Printf("highlight: rgb(%d, %d, %d)\n", hlR, hlG, hlB)
}

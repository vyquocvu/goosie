package layout

import (
	"math"
	"strconv"
	"strings"
	"unicode"
)

// computeMotionOffset computes (dx, dy) translation from an offset-path string and offset-distance length.
func computeMotionOffset(pathStr string, distance float32) (float32, float32) {
	if distance == 0 || pathStr == "" || pathStr == "none" {
		return 0, 0
	}
	pathStr = strings.TrimSpace(pathStr)
	if strings.HasPrefix(strings.ToLower(pathStr), "ray(") && strings.HasSuffix(pathStr, ")") {
		inner := strings.TrimSpace(pathStr[4 : len(pathStr)-1])
		return computeRayOffset(inner, distance)
	}
	if strings.HasPrefix(strings.ToLower(pathStr), "path(") && strings.HasSuffix(pathStr, ")") {
		inner := strings.TrimSpace(pathStr[5 : len(pathStr)-1])
		inner = strings.Trim(inner, `"'`)
		return computePathOffset(inner, distance)
	}
	return 0, 0
}

func computeRayOffset(param string, distance float32) (float32, float32) {
	fields := strings.Fields(param)
	if len(fields) == 0 {
		return 0, 0
	}
	angleToken := strings.ToLower(fields[0])
	var radians float64
	if strings.HasSuffix(angleToken, "deg") {
		val, _ := strconv.ParseFloat(strings.TrimSuffix(angleToken, "deg"), 64)
		radians = val * math.Pi / 180.0
	} else if strings.HasSuffix(angleToken, "rad") {
		val, _ := strconv.ParseFloat(strings.TrimSuffix(angleToken, "rad"), 64)
		radians = val
	} else if strings.HasSuffix(angleToken, "turn") {
		val, _ := strconv.ParseFloat(strings.TrimSuffix(angleToken, "turn"), 64)
		radians = val * 2.0 * math.Pi
	} else if strings.HasSuffix(angleToken, "grad") {
		val, _ := strconv.ParseFloat(strings.TrimSuffix(angleToken, "grad"), 64)
		radians = val * math.Pi / 200.0
	} else {
		val, _ := strconv.ParseFloat(angleToken, 64)
		radians = val * math.Pi / 180.0
	}

	// 0deg points up along negative Y axis; angle grows clockwise in CSS.
	dx := float32(float64(distance) * math.Sin(radians))
	dy := float32(-float64(distance) * math.Cos(radians))
	return dx, dy
}

type pathPoint struct {
	x, y float32
}

func computePathOffset(d string, distance float32) (float32, float32) {
	points := parsePathPoints(d)
	if len(points) < 2 {
		return 0, 0
	}
	origin := points[0]
	rem := float64(distance)
	for i := 0; i < len(points)-1; i++ {
		p0 := points[i]
		p1 := points[i+1]
		segDx := float64(p1.x - p0.x)
		segDy := float64(p1.y - p0.y)
		segLen := math.Hypot(segDx, segDy)
		if segLen == 0 {
			continue
		}
		if rem <= segLen {
			t := rem / segLen
			targetX := float64(p0.x) + t*segDx
			targetY := float64(p0.y) + t*segDy
			return float32(targetX) - origin.x, float32(targetY) - origin.y
		}
		rem -= segLen
	}
	last := points[len(points)-1]
	return last.x - origin.x, last.y - origin.y
}

func parsePathPoints(d string) []pathPoint {
	var points []pathPoint
	var curX, curY float32
	var startX, startY float32

	tokens := tokenizePath(d)
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		cmd := tok[0]
		i++

		switch cmd {
		case 'M', 'm':
			if i+1 < len(tokens) {
				x := parseFloat(tokens[i])
				y := parseFloat(tokens[i+1])
				i += 2
				if cmd == 'm' {
					curX += x
					curY += y
				} else {
					curX = x
					curY = y
				}
				startX, startY = curX, curY
				points = append(points, pathPoint{curX, curY})
			}
		case 'L', 'l':
			if i+1 < len(tokens) {
				x := parseFloat(tokens[i])
				y := parseFloat(tokens[i+1])
				i += 2
				if cmd == 'l' {
					curX += x
					curY += y
				} else {
					curX = x
					curY = y
				}
				points = append(points, pathPoint{curX, curY})
			}
		case 'H', 'h':
			if i < len(tokens) {
				x := parseFloat(tokens[i])
				i++
				if cmd == 'h' {
					curX += x
				} else {
					curX = x
				}
				points = append(points, pathPoint{curX, curY})
			}
		case 'V', 'v':
			if i < len(tokens) {
				y := parseFloat(tokens[i])
				i++
				if cmd == 'v' {
					curY += y
				} else {
					curY = y
				}
				points = append(points, pathPoint{curX, curY})
			}
		case 'Z', 'z':
			curX, curY = startX, startY
			points = append(points, pathPoint{curX, curY})
		}
	}
	return points
}

func tokenizePath(d string) []string {
	var tokens []string
	var buf strings.Builder
	for _, r := range d {
		if r == ',' || unicode.IsSpace(r) {
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
			continue
		}
		if isCommand(r) {
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
			tokens = append(tokens, string(r))
			continue
		}
		buf.WriteRune(r)
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	return tokens
}

func isCommand(r rune) bool {
	switch r {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

func parseFloat(s string) float32 {
	v, _ := strconv.ParseFloat(s, 32)
	return float32(v)
}

package paint

import (
	"math"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

func convertColor(c css.Color) frame.Color {
	return frame.RGBA(c.R, c.G, c.B, c.A)
}

func convertStops(stops []style.GradientStop, opacity float32) []frame.GradientStop {
	out := make([]frame.GradientStop, 0, len(stops))
	for _, s := range stops {
		c := convertColor(s.Color)
		if opacity > 0 && opacity < 1 {
			c = frame.ScaleColor(c, opacity)
		}
		out = append(out, frame.GradientStop{At: s.At, Color: c})
	}
	return out
}

func gradient(g style.Gradient, lineLen float32, opacity float32) frame.LinearGradient {
	if g.Empty() {
		return frame.LinearGradient{}
	}
	return frame.LinearGradient{Angle: g.Angle, Stops: convertStops(style.StopsAlong(g.Stops, lineLen), opacity)}
}

func radialGradient(g style.RadialGradient, rx, ry, cx, cy float32, opacity float32) frame.RadialGradient {
	if g.Empty() {
		return frame.RadialGradient{}
	}
	return frame.RadialGradient{Cx: cx, Cy: cy, Rx: rx, Ry: ry, Stops: convertStops(style.StopsAlong(g.Stops, rx), opacity)}
}

func gradientLineLength(angle, tw, th float32) float32 {
	a := float64(angle) * math.Pi / 180
	dirX, dirY := math.Sin(a), -math.Cos(a)
	return float32(math.Abs(float64(tw)*dirX) + math.Abs(float64(th)*dirY))
}

func radialRadii(g style.RadialGradient, tw, th float32) (rx, ry, cx, cy float32) {
	cx, cy = g.Cx, g.Cy
	if g.CxPxSet && tw > 0 {
		cx = g.CxPx / tw
	}
	if g.CyPxSet && th > 0 {
		cy = g.CyPx / th
	}
	l, t := cx*tw, cy*th
	hn, hf := min(l, tw-l), max(l, tw-l)
	vn, vf := min(t, th-t), max(t, th-t)
	switch {
	case g.Shape == "circle":
		var r float32
		switch g.Size {
		case "closest-side":
			r = min(hn, vn)
		case "farthest-side":
			r = max(hf, vf)
		case "closest-corner":
			r = float32(math.Hypot(float64(hn), float64(vn)))
		default:
			r = float32(math.Hypot(float64(hf), float64(vf)))
		}
		return r, r, cx, cy
	default:
		switch g.Size {
		case "closest-side", "closest-corner":
			return hn, vn, cx, cy
		case "farthest-side":
			return hf, vf, cx, cy
		default:
			k := float32(math.Hypot(float64(hf)/float64(hn), float64(vf)/float64(vn)))
			return hn * k, vn * k, cx, cy
		}
	}
}

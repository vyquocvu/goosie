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

// convertStops turns declared stops into premultiplied ones with the box's
// opacity folded into each colour, so the command carries one multiplier
// rather than two and the rasterizer never revisits the style.
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

// gradient turns a declared ramp into a command: length-written stop
// positions resolve against lineLen, the length of the gradient line across
// the tile the ramp maps onto.
func gradient(g style.Gradient, lineLen float32, opacity float32) frame.LinearGradient {
	if g.Empty() {
		return frame.LinearGradient{}
	}
	return frame.LinearGradient{Angle: g.Angle, Stops: convertStops(style.StopsAlong(g.Stops, lineLen), opacity)}
}

// radialGradient turns a declared radial ramp into a command, resolving its
// length-written stops against the radius the rasterizer will blend across.
func radialGradient(g style.RadialGradient, radius float32, opacity float32) frame.RadialGradient {
	if g.Empty() {
		return frame.RadialGradient{}
	}
	return frame.RadialGradient{Shape: g.Shape, Cx: g.Cx, Cy: g.Cy, Stops: convertStops(style.StopsAlong(g.Stops, radius), opacity)}
}

// gradientLineLength is the CSS gradient line across a tile: |W·sin θ| +
// |H·cos θ| - the exact measure internal/frame re-derives from the command
// rect per pixel. A length-written stop must divide by the same number or it
// lands at the wrong pixels.
func gradientLineLength(angle, tw, th float32) float32 {
	a := float64(angle) * math.Pi / 180
	dirX, dirY := math.Sin(a), -math.Cos(a)
	return float32(math.Abs(float64(tw)*dirX) + math.Abs(float64(th)*dirY))
}

// radialRadius mirrors the radius internal/frame.FillRadialGradient derives
// from the command rect, so length stops divide by the length the
// rasterizer actually blends across. The circle rule - half the shorter side,
// not the reach to the farthest corner - is itself recorded as wrong against
// the reference in GOOSIE-RADIALGRAD-001; resolving against it keeps px stops
// consistent with the colours already painted.
func radialRadius(g style.RadialGradient, tw, th float32) float32 {
	if g.Shape == "circle" {
		return float32(math.Min(float64(tw), float64(th)) / 2)
	}
	cx, cy := float64(g.Cx)*float64(tw), float64(g.Cy)*float64(th)
	dx := math.Max(cx, float64(tw)-cx)
	dy := math.Max(cy, float64(th)-cy)
	return float32(math.Sqrt(dx*dx + dy*dy))
}

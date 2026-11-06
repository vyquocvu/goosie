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
// length-written stops against the horizontal semi-axis the rasterizer blends
// along (the gradient ray) and carrying the resolved radii and centre.
func radialGradient(g style.RadialGradient, rx, ry, cx, cy float32, opacity float32) frame.RadialGradient {
	if g.Empty() {
		return frame.RadialGradient{}
	}
	return frame.RadialGradient{Cx: cx, Cy: cy, Rx: rx, Ry: ry, Stops: convertStops(style.StopsAlong(g.Stops, rx), opacity)}
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

// radialRadii resolves the gradient's centre and semi-axis radii against the
// command rect, so the rasterizer never revisits the sizing rule. The table is
// the CSS sizing grammar measured against Chromium (GOOSIE-RADIALGRAD-001):
// each keyword names a side or corner the ending shape must touch, taken from
// the centre's four side distances - hn/hf the nearest and farthest
// horizontally, vn/vf vertically. A circle carries one radius; an ellipse
// carries per-axis radii, the closest-side pair for the side and corner
// keywords, and for farthest-corner the closest-side aspect scaled by the
// factor that reaches the farthest corner. A circle with default size is that
// same scaling collapsed to one radius.
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
		default: // farthest-corner
			r = float32(math.Hypot(float64(hf), float64(vf)))
		}
		return r, r, cx, cy
	default: // ellipse
		switch g.Size {
		case "closest-side", "closest-corner":
			return hn, vn, cx, cy
		case "farthest-side":
			return hf, vf, cx, cy
		default: // farthest-corner
			k := float32(math.Hypot(float64(hf)/float64(hn), float64(vf)/float64(vn)))
			return hn * k, vn * k, cx, cy
		}
	}
}

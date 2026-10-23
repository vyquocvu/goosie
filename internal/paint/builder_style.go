package paint

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

func convertColor(c css.Color) frame.Color {
	return frame.RGBA(c.R, c.G, c.B, c.A)
}

// gradient turns a declared ramp into premultiplied stops and folds the box's
// opacity into each of them, so the command carries one multiplier rather than
// two and the rasterizer never revisits the style.
func gradient(g style.Gradient, opacity float32) frame.LinearGradient {
	if g.Empty() {
		return frame.LinearGradient{}
	}
	stops := make([]frame.GradientStop, 0, len(g.Stops))
	for _, s := range g.Stops {
		c := convertColor(s.Color)
		if opacity > 0 && opacity < 1 {
			c = frame.ScaleColor(c, opacity)
		}
		stops = append(stops, frame.GradientStop{At: s.At, Color: c})
	}
	return frame.LinearGradient{Angle: g.Angle, Stops: stops}
}

func radialGradient(g style.RadialGradient, opacity float32) frame.RadialGradient {
	if g.Empty() {
		return frame.RadialGradient{}
	}
	stops := make([]frame.GradientStop, 0, len(g.Stops))
	for _, s := range g.Stops {
		c := convertColor(s.Color)
		if opacity > 0 && opacity < 1 {
			c = frame.ScaleColor(c, opacity)
		}
		stops = append(stops, frame.GradientStop{At: s.At, Color: c})
	}
	return frame.RadialGradient{Shape: g.Shape, Cx: g.Cx, Cy: g.Cy, Stops: stops}
}

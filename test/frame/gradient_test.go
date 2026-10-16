package frame_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

// darkToLight is a vertical ramp from opaque black to opaque white, the simplest
// shape that tells direction, position, and interpolation apart.
func darkToLight() frame.LinearGradient {
	return frame.LinearGradient{
		Angle: 180,
		Stops: []frame.GradientStop{
			{At: 0, Color: frame.RGB(0, 0, 0)},
			{At: 1, Color: frame.RGB(255, 255, 255)},
		},
	}
}

func TestFillLinearGradientRunsDownByDefault(t *testing.T) {
	b := frame.NewBitmap(4, 8)
	b.FillLinearGradient(frame.Rect4(0, 0, 4, 8), frame.Corners{}, darkToLight(), nil)
	if top, bottom := b.At(1, 0).G(), b.At(1, 7).G(); top > 40 || bottom < 215 {
		t.Errorf("ramp ran the wrong way: top %v, bottom %v", top, bottom)
	}
	if mid := b.At(1, 4).G(); mid < 100 || mid > 160 {
		t.Errorf("midpoint = %v, want about half", mid)
	}
	for y := 1; y < 8; y++ {
		if b.At(1, y).G() < b.At(1, y-1).G() {
			t.Fatalf("row %d is darker than the row above: ramp is not monotonic", y)
		}
	}
}

func TestFillLinearGradientAnglePicksTheAxis(t *testing.T) {
	b := frame.NewBitmap(8, 4)
	g := darkToLight()
	g.Angle = 90
	b.FillLinearGradient(frame.Rect4(0, 0, 8, 4), frame.Corners{}, g, nil)
	if left, right := b.At(0, 1).G(), b.At(7, 1).G(); left > 40 || right < 215 {
		t.Errorf("horizontal ramp went %v to %v, want dark to light", left, right)
	}
	if a, c := b.At(3, 0).G(), b.At(3, 3).G(); a != c {
		t.Errorf("a column varied down the box (%v then %v); a 90deg ramp must not", a, c)
	}
}

func TestFillLinearGradientStopsOutsideTheRampHold(t *testing.T) {
	b := frame.NewBitmap(4, 4)
	g := frame.LinearGradient{Angle: 180, Stops: []frame.GradientStop{
		{At: 0.25, Color: frame.RGB(0, 0, 0)},
		{At: 0.75, Color: frame.RGB(255, 255, 255)},
	}}
	b.FillLinearGradient(frame.Rect4(0, 0, 4, 4), frame.Corners{}, g, nil)
	if b.At(1, 0).G() != 0 {
		t.Errorf("top row = %v, want the first stop's colour", b.At(1, 0).G())
	}
	if b.At(1, 3).G() != 255 {
		t.Errorf("bottom row = %v, want the last stop's colour", b.At(1, 3).G())
	}
}

func TestFillLinearGradientLeavesRoundedCornersAlone(t *testing.T) {
	b := frame.NewBitmap(8, 8)
	b.FillRect(b.Bounds(), frame.RGB(255, 255, 255), nil)
	rad := frame.Corners{
		TL: frame.Radius{X: 4, Y: 4}, TR: frame.Radius{X: 4, Y: 4},
		BR: frame.Radius{X: 4, Y: 4}, BL: frame.Radius{X: 4, Y: 4},
	}
	b.FillLinearGradient(frame.Rect4(0, 0, 8, 8), rad, darkToLight(), nil)
	if b.At(0, 0).G() != 255 {
		t.Errorf("corner pixel = %v, want the background it cut away", b.At(0, 0).G())
	}
	if b.At(4, 4).G() == 255 {
		t.Error("box centre unpainted")
	}
}

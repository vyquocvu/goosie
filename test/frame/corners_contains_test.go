package frame_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

// qc:BR-073
func TestSquareCornersAreThePlainRectTest(t *testing.T) {
	c := frame.Corners{}
	for _, p := range []struct{ x, y float32 }{{0, 0}, {99, 0}, {0, 99}, {50, 50}} {
		if !c.Contains(0, 0, 100, 100, p.x, p.y) {
			t.Errorf("(%v,%v) excluded by a square box", p.x, p.y)
		}
	}
	for _, p := range []struct{ x, y float32 }{{-1, 50}, {100, 50}, {50, -1}, {50, 100}} {
		if c.Contains(0, 0, 100, 100, p.x, p.y) {
			t.Errorf("(%v,%v) inside a square box that stops there", p.x, p.y)
		}
	}
}

// qc:BR-073
func TestRoundedCornerExcludesPixelsOutsideTheCurve(t *testing.T) {
	r := frame.Radius{X: 30, Y: 30}
	c := frame.Corners{TL: r, TR: r, BR: r, BL: r}
	if c.Contains(0, 0, 120, 60, 2, 2) {
		t.Error("(2,2) is outside the top-left curve but counted as hit")
	}
	if !c.Contains(0, 0, 120, 60, 30, 2) {
		t.Error("(30,2) is straight below the curve centre and must count as hit")
	}
	if !c.Contains(0, 0, 120, 60, 60, 30) {
		t.Error("the centre of a rounded box must still be inside it")
	}
	if c.Contains(0, 0, 120, 60, 118, 2) {
		t.Error("(118,2) is outside the top-right curve but counted as hit")
	}
	if c.Contains(0, 0, 120, 60, 2, 58) {
		t.Error("(2,58) is outside the bottom-left curve but counted as hit")
	}
	if c.Contains(0, 0, 120, 60, 118, 58) {
		t.Error("(118,58) is outside the bottom-right curve but counted as hit")
	}
	if !c.Contains(0, 0, 120, 60, 60, 0) {
		t.Error("(60,0) is on the straight top edge and must count as hit")
	}
}

// qc:BR-073
func TestEllipticalCornerTestsEachAxis(t *testing.T) {
	c := frame.Corners{TL: frame.Radius{X: 40, Y: 10}}
	if c.Contains(0, 0, 200, 100, 1, 1) {
		t.Error("(1,1) is beyond the 10px vertical radius but counted as hit")
	}
	if !c.Contains(0, 0, 200, 100, 30, 1) {
		t.Error("(30,1) is inside the ellipse and must count as hit")
	}
	mixed := frame.Corners{TL: frame.Radius{X: 20, Y: 20}}
	if mixed.Contains(0, 0, 200, 100, 1, 1) {
		t.Error("(1,1) is outside the one declared corner and must not count as hit")
	}
	if !mixed.Contains(0, 0, 200, 100, 1, 99) {
		t.Error("the bottom-left corner is square here, so (1,99) must count as hit")
	}
	if !mixed.Contains(0, 0, 200, 100, 100, 1) {
		t.Error("the straight part of the top edge must count as hit")
	}
}

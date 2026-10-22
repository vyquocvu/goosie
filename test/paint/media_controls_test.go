package paint_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// These tests pin the geometry the painter and the browser's pointer handling
// share. They are the reason VideoControlLayout is a function rather than a few
// expressions inside paintVideo: a control whose ink and whose click target are
// computed in two places drift apart the first time either is edited, and the
// failure is a button a user can see and cannot press.

func TestVideoControlLayoutFitsTheBox(t *testing.T) {
	box := frame.Rect4(100, 50, 420, 230)
	c := paint.VideoControlLayout(box, 1)
	if !c.Has {
		t.Fatal("a 320x180 box produced no control bar")
	}
	if c.Bar != frame.Rect4(100, 198, 420, 230) {
		t.Fatalf("bar = %v, want the bottom 32px of the box", c.Bar)
	}
	if c.Play.X0 < box.X0 || c.Play.X1 > box.X1 || c.Play.Y1 != box.Y1 {
		t.Fatalf("play button %v is not inside the box %v", c.Play, box)
	}
	if c.Track.X0 <= c.Play.X1 {
		t.Fatalf("track %v starts at or before the play button %v", c.Track, c.Play)
	}
	if c.Mute.X0 < c.Track.X1 {
		t.Fatalf("mute %v overlaps the track %v", c.Mute, c.Track)
	}
	if c.Mute.X1 > box.X1 {
		t.Fatalf("mute button %v runs past the box %v", c.Mute, box)
	}
}

// TestVideoControlLayoutScalesWithDPR pins that the bar is laid out in device
// pixels: a Retina video must get a Retina-sized control bar, or the buttons are
// half the size the pointer expects.
func TestVideoControlLayoutScalesWithDPR(t *testing.T) {
	at1 := paint.VideoControlLayout(frame.Rect4(0, 0, 320, 180), 1)
	at2 := paint.VideoControlLayout(frame.Rect4(0, 0, 640, 360), 2)
	if !at1.Has || !at2.Has {
		t.Fatal("a control bar was missing at one of the two scales")
	}
	if got, want := at2.Bar.Y1-at2.Bar.Y0, (at1.Bar.Y1-at1.Bar.Y0)*2; got != want {
		t.Fatalf("bar height at 2x = %d, want %d", got, want)
	}
	if got, want := at2.Play.X1-at2.Play.X0, (at1.Play.X1-at1.Play.X0)*2; got != want {
		t.Fatalf("play button width at 2x = %d, want %d", got, want)
	}
}

// TestVideoControlLayoutRefusesABoxTooSmallToUse pins that a cramped element gets
// no bar at all rather than a bar whose parts overlap. A seek track two pixels
// wide is worse than none: it is a target the user aims at and misses.
func TestVideoControlLayoutRefusesABoxTooSmallToUse(t *testing.T) {
	for _, box := range []frame.Rect{
		frame.Rect4(0, 0, 60, 180),  // too narrow for the bar's own insets
		frame.Rect4(0, 0, 320, 0),   // no height
		frame.Rect4(0, 0, 320, -20), // inverted
	} {
		if c := paint.VideoControlLayout(box, 1); c.Has {
			t.Fatalf("box %v produced a control bar %+v", box, c)
		}
	}
}

// TestVideoControlsAtMatchesThePaintedRects pins the hit test against the
// geometry, including the gaps: the bar's empty space is not a control, and
// neither is anywhere outside it.
func TestVideoControlsAtMatchesThePaintedRects(t *testing.T) {
	c := paint.VideoControlLayout(frame.Rect4(100, 50, 420, 230), 1)
	if !c.Has {
		t.Fatal("no control bar to test against")
	}

	mid := func(r frame.Rect) frame.Point {
		return frame.Point{X: (r.X0 + r.X1) / 2, Y: (r.Y0 + r.Y1) / 2}
	}
	for _, tc := range []struct {
		name string
		at   frame.Point
		want paint.VideoControlPart
	}{
		{"play", mid(c.Play), paint.VideoControlPlay},
		{"track", mid(c.Track), paint.VideoControlTrack},
		{"mute", mid(c.Mute), paint.VideoControlMute},
		{"above the bar", frame.Point{X: 200, Y: 100}, paint.VideoControlNone},
		{"below the box", frame.Point{X: 200, Y: 400}, paint.VideoControlNone},
		{"left of the box", frame.Point{X: 10, Y: 210}, paint.VideoControlNone},
		{"right of the box", frame.Point{X: 900, Y: 210}, paint.VideoControlNone},
	} {
		if got := c.At(tc.at); got != tc.want {
			t.Errorf("%s at %v = %v, want %v", tc.name, tc.at, got, tc.want)
		}
	}
}

// TestVideoTrackFractionClamps pins that a pointer which drifts past either end
// while the button is held seeks to the beginning or the end rather than to a
// negative time or past the duration.
func TestVideoTrackFractionClamps(t *testing.T) {
	c := paint.VideoControlLayout(frame.Rect4(100, 50, 420, 230), 1)
	if !c.Has || c.Track.Empty() {
		t.Fatal("no track to test against")
	}
	for _, tc := range []struct {
		x    int32
		want float64
	}{
		{c.Track.X0, 0},
		{c.Track.X1, 1},
		{(c.Track.X0 + c.Track.X1) / 2, 0.5},
		{c.Track.X0 - 500, 0},
		{c.Track.X1 + 500, 1},
	} {
		got := c.TrackFraction(tc.x)
		if diff := got - tc.want; diff > 0.01 || diff < -0.01 {
			t.Errorf("TrackFraction(%d) = %v, want %v", tc.x, got, tc.want)
		}
	}
	// A layout with no track has nowhere to seek, and must not divide by zero.
	if got := (paint.VideoControls{}).TrackFraction(10); got != 0 {
		t.Fatalf("TrackFraction on an empty layout = %v, want 0", got)
	}
}

// TestVideoTimeLabel pins the clock the bar and the accessibility tree both read.
func TestVideoTimeLabel(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0:00"},
		{1.4, "0:01"},
		{59.6, "1:00"},
		{75, "1:15"},
		{3599, "59:59"},
		{3600, "60:00"},
	} {
		if got := paint.VideoTimeLabel(tc.in); got != tc.want {
			t.Errorf("VideoTimeLabel(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// An unknown duration and a negative position both read as the start rather
	// than as a NaN, which is what a page shows a user before metadata arrives.
	for _, in := range []float64{math.NaN(), -3, math.Inf(1)} {
		if got := paint.VideoTimeLabel(in); got != "0:00" {
			t.Errorf("VideoTimeLabel(%v) = %q, want 0:00", in, got)
		}
	}
}

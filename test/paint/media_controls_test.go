package paint_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

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

func TestVideoControlLayoutRefusesABoxTooSmallToUse(t *testing.T) {
	for _, box := range []frame.Rect{
		frame.Rect4(0, 0, 60, 180),
		frame.Rect4(0, 0, 320, 0),
		frame.Rect4(0, 0, 320, -20),
	} {
		if c := paint.VideoControlLayout(box, 1); c.Has {
			t.Fatalf("box %v produced a control bar %+v", box, c)
		}
	}
}

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
	if got := (paint.VideoControls{}).TrackFraction(10); got != 0 {
		t.Fatalf("TrackFraction on an empty layout = %v, want 0", got)
	}
}

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
	for _, in := range []float64{math.NaN(), -3, math.Inf(1)} {
		if got := paint.VideoTimeLabel(in); got != "0:00" {
			t.Errorf("VideoTimeLabel(%v) = %q, want 0:00", in, got)
		}
	}
}

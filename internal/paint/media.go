package paint

import (
	"image"
	"math"
	"strconv"

	"github.com/vyquocvu/goosie/internal/frame"
)

// VideoState is the playback condition a video box paints. It is deliberately
// not the media package's State: paint needs to know what to draw, not where a
// player is in its lifecycle, and keeping the two apart is what lets a backend
// change without touching the display list.
type VideoState uint8

const (
	// VideoIdle has no source and paints the empty viewport.
	VideoIdle VideoState = iota
	// VideoLoading has a source whose bytes or metadata have not arrived.
	VideoLoading
	// VideoReady has a frame and is not advancing.
	VideoReady
	// VideoPlaying is advancing.
	VideoPlaying
	// VideoFailed paints an error message instead of a frame.
	VideoFailed
)

// VideoView is one video element's presentation state, attached to its layout
// object by the engine and read back out by the builder. It is plain data: no
// player, no controller, no backend, so a frozen display list stays frozen and
// the raster workers never reach into media state.
type VideoView struct {
	// Pixels is the frame to draw, nil when nothing has decoded yet. The
	// pixels are immutable for the lifetime of the list that carries them.
	Pixels image.Image
	// Width and Height are the decoded dimensions, for object-fit.
	Width, Height int
	State         VideoState
	// Position and Duration drive the progress bar. Duration is NaN until
	// metadata is known, which the bar paints as an empty track rather than as
	// a full one.
	Position float64
	Duration float64
	Muted    bool
	// Message is the loading or error text painted over the viewport when
	// there is no frame to show.
	Message string
	// Unsupported says this session has no playback backend at all, so the
	// element paints its message and no control bar: a bar that can never do
	// anything is worse than an admission that nothing will.
	Unsupported bool
}

// HasFrame reports whether there is something to draw in the viewport.
func (v VideoView) HasFrame() bool { return v.Pixels != nil }

// VideoControls is the geometry of a video element's control bar in device
// pixels, plus whether there is one at all.
type VideoControls struct {
	Bar   frame.Rect
	Play  frame.Rect
	Track frame.Rect
	Mute  frame.Rect
	Has   bool
}

// videoControlMetrics are the bar's proportions in CSS pixels. They are shared by
// the painter and the hit tester, which is the point of this function existing: a
// control whose ink and whose click area are computed separately drift apart the
// first time one of them is edited, and the result is a button the user can see
// and cannot press.
const (
	videoBarHeight   = 32.0
	videoPlayInset   = 10.0
	videoPlayWidth   = 16.0
	videoTrackInset  = 34.0
	videoMuteWidth   = 20.0
	videoTimeWidth   = 62.0
	videoBarMinWidth = 120.0
)

// VideoControlLayout computes the control bar for a video box.
//
// box is the element's own content box in device pixels, uncut by any ancestor
// clip. Laying the bar out against an already-clipped rectangle is what makes
// controls slide around as the element scrolls out of view: the bar belongs to the
// element, and only its ink gets clipped afterwards.
func VideoControlLayout(box frame.Rect, scale float32) VideoControls {
	if scale <= 0 {
		scale = 1
	}
	w, h := float32(box.X1-box.X0), float32(box.Y1-box.Y0)
	if w < videoBarMinWidth*scale || h <= 0 {
		return VideoControls{}
	}
	barH := int32(videoBarHeight * scale)
	if float32(barH) > h {
		barH = int32(h)
	}
	if barH <= 0 {
		return VideoControls{}
	}
	c := VideoControls{
		Has: true,
		Bar: frame.Rect4(box.X0, box.Y1-barH, box.X1, box.Y1),
		Play: frame.Rect4(box.X0+int32(videoPlayInset*scale), box.Y1-barH,
			box.X0+int32((videoPlayInset+videoPlayWidth)*scale), box.Y1),
	}
	// The track takes what is left between the play button and the mute/time end,
	// and is dropped rather than squeezed when there is no room for it to be
	// usable: a two-pixel seek target is worse than no seek bar.
	trackRight := box.X1 - int32((videoMuteWidth+videoTimeWidth)*scale)
	trackLeft := box.X0 + int32(videoTrackInset*scale)
	if trackRight > trackLeft+int32(30*scale) {
		c.Track = frame.Rect4(trackLeft, box.Y1-barH, trackRight, box.Y1)
		c.Mute = frame.Rect4(trackRight, box.Y1-barH,
			trackRight+int32(videoMuteWidth*scale), box.Y1)
	}
	return c
}

// At reports which control covers a device-pixel point, or VideoControlNone. The
// bar itself is not a control: clicking its empty part does nothing, which is what
// a browser does too.
func (c VideoControls) At(p frame.Point) VideoControlPart {
	if !c.Has {
		return VideoControlNone
	}
	switch {
	case c.Play.Contains(p):
		return VideoControlPlay
	case c.Mute.Contains(p):
		return VideoControlMute
	case c.Track.Contains(p):
		return VideoControlTrack
	}
	return VideoControlNone
}

// VideoControlPart names one hittable region of a video's control bar.
type VideoControlPart uint8

const (
	VideoControlNone VideoControlPart = iota
	VideoControlPlay
	VideoControlTrack
	VideoControlMute
)

// TrackFraction turns a device-pixel x inside the track into a 0..1 position,
// which is what a click on the bar seeks to. It clamps, so a pointer that drifts
// past either end while the button is held seeks to the beginning or the end
// rather than to a negative time.
func (c VideoControls) TrackFraction(x int32) float64 {
	if !c.Has || c.Track.Empty() {
		return 0
	}
	f := float64(x-c.Track.X0) / float64(c.Track.X1-c.Track.X0)
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// VideoTimeLabel formats a playback time as m:ss, which is what both the control
// bar and the accessibility tree have to agree on. An unknown duration and a
// negative position both read as 0:00 rather than as a NaN a user would see.
func VideoTimeLabel(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	total := int(math.Floor(seconds + 0.5))
	m, s := total/60, total%60
	if m >= 100 {
		return strconv.Itoa(m) + ":" + pad2(s)
	}
	return strconv.Itoa(m) + ":" + pad2(s)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

package paint

import (
	"image"
	"math"
	"strconv"

	"github.com/vyquocvu/goosie/internal/frame"
)

type VideoState uint8

const (
	VideoIdle VideoState = iota
	VideoLoading
	VideoReady
	VideoPlaying
	VideoFailed
)

type VideoView struct {
	Pixels        image.Image
	Width, Height int
	State         VideoState
	Position      float64
	Duration      float64
	Muted         bool
	Message       string
	Unsupported   bool
}

func (v VideoView) HasFrame() bool { return v.Pixels != nil }

type VideoControls struct {
	Bar   frame.Rect
	Play  frame.Rect
	Track frame.Rect
	Mute  frame.Rect
	Has   bool
}

const (
	videoBarHeight   = 32.0
	videoPlayInset   = 10.0
	videoPlayWidth   = 16.0
	videoTrackInset  = 34.0
	videoMuteWidth   = 20.0
	videoTimeWidth   = 62.0
	videoBarMinWidth = 120.0
)

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
	trackRight := box.X1 - int32((videoMuteWidth+videoTimeWidth)*scale)
	trackLeft := box.X0 + int32(videoTrackInset*scale)
	if trackRight > trackLeft+int32(30*scale) {
		c.Track = frame.Rect4(trackLeft, box.Y1-barH, trackRight, box.Y1)
		c.Mute = frame.Rect4(trackRight, box.Y1-barH,
			trackRight+int32(videoMuteWidth*scale), box.Y1)
	}
	return c
}

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

type VideoControlPart uint8

const (
	VideoControlNone VideoControlPart = iota
	VideoControlPlay
	VideoControlTrack
	VideoControlMute
)

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

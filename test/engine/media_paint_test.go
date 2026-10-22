package engine_test

import (
	"context"
	"errors"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/paint"
)

// playGlyphY finds the y coordinate of the play button's glyph in a painted list,
// which is the one number that tells the two clipping cases apart.
func playGlyphY(cmds []paint.DisplayCmd) (int32, bool) {
	for _, c := range cmds {
		if c.Kind != paint.CmdText {
			continue
		}
		for _, g := range c.Text.Glyphs {
			if g.Rune == '►' {
				return g.Y, true
			}
		}
	}
	return 0, false
}

// TestMediaPaintsTheDecodedFrame pins the seam the whole feature rests on: a frame
// the backend produced reaches the display list as an image command carrying those
// exact pixels, and the control bar reports the real position rather than the
// quarter-way guess a placeholder draws.
func TestMediaPaintsTheDecodedFrame(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)

	want := solid(color.RGBA{R: 12, G: 34, B: 56, A: 255})
	p.set(func(s *media.Snapshot) {
		s.State = media.StatePlaying
		s.Duration = 8
		s.Position = 4
		s.Width, s.Height = 320, 180
		s.Pixels = want
		s.Seq = 1
	})
	pumpUntil(t, sess, func() bool { return c.Snapshot().Seq == 1 })
	// The control bar advances on the timeupdate cadence rather than every poll,
	// so the position painted here is the one a user would actually see.
	time.Sleep(timeUpdateInterval + 20*time.Millisecond)
	sess.PumpMedia()

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	cmds := dl.Build(1).All()

	var img *paint.DisplayCmd
	for i := range cmds {
		if cmds[i].Kind == paint.CmdImage && cmds[i].Image.Src == want {
			img = &cmds[i]
		}
	}
	if img == nil {
		t.Fatalf("the decoded frame never reached the display list (%d commands)", len(cmds))
	}
	if img.Rect.W() != 320 || img.Rect.H() != 180 {
		t.Fatalf("the frame was painted into %v, want the element's 320x180 box", img.Rect)
	}
	if !img.Clip.Empty() {
		t.Fatalf("an unclipped element's frame carried a clip of %v", img.Clip)
	}

	// Progress: half of an 8 second asset is played, so the blue fill has to cover
	// about half the track. A placeholder that always draws a quarter would pass
	// the "some blue exists" version of this and fail the one that matters.
	trackW, blueW := int32(0), int32(0)
	blue := frame.RGB(66, 133, 244)
	grey := frame.RGB(80, 80, 80)
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		switch cmd.Color {
		case grey:
			if cmd.Rect.H() <= 8 {
				trackW = cmd.Rect.W()
			}
		case blue:
			blueW = cmd.Rect.W()
		}
	}
	if trackW == 0 {
		t.Fatal("no timeline track was painted")
	}
	if frac := float64(blueW) / float64(trackW); frac < 0.4 || frac > 0.6 {
		t.Fatalf("the progress fill covers %.2f of the track, want about half (position 4 of 8)", frac)
	}
}

// TestMediaPaintsNoFrameBeforeOneArrives pins the honest half of the same seam: an
// element that has not decoded anything paints a status message and no image, so a
// video that cannot play is visibly not-playing rather than showing a black
// rectangle that looks like a frame.
func TestMediaPaintsNoFrameBeforeOneArrives(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	sess.PumpMedia()

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdImage {
			t.Fatalf("an element with no decoded frame painted an image (%v)", cmd.Rect)
		}
	}
}

// TestMediaControlsStayPutWhenClipped is the regression the shared geometry exists
// for. A video half scrolled out of an overflow:hidden box used to have its
// controls derived from the clipped rectangle, which moved the play button as the
// page scrolled: the ink and the pointer target disagreed, and the bar slid around
// under the user. The controls belong to the element; only their ink is clipped.
func TestMediaControlsStayPutWhenClipped(t *testing.T) {
	unclipped := `<html><body><video id="v" controls width="320" height="180"></video></body></html>`
	// Ten pixels off the bottom, which cuts through the control bar without
	// hiding it. A clip that removed the bar entirely would prove nothing: there
	// is nothing to misposition when there is nothing to draw.
	clipped := `<html><body><div style="height:170px;overflow:hidden">` +
		`<video id="v" controls width="320" height="180"></video></div></body></html>`

	paintIt := func(t *testing.T, html string) (int32, bool) {
		t.Helper()
		// A backend, or the element paints its unsupported message and no bar at
		// all: the geometry this test pins only exists on a session that can play.
		sess, err := engine.NewSession(html, nil, 800,
			engine.WithMetrics(&fixedMetrics{advance: 7}),
			engine.WithMedia(newFakeBackend(),
				func(context.Context, string) (engine.MediaResponse, error) {
					return engine.MediaResponse{}, errors.New("this test fetches nothing")
				}, "http://example.test/"),
		)
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		defer sess.Close()
		dl, err := sess.PaintChecked(1.0)
		if err != nil {
			t.Fatalf("PaintChecked: %v", err)
		}
		return playGlyphY(dl.Build(1).All())
	}

	openY, openOK := paintIt(t, unclipped)
	clippedY, clippedOK := paintIt(t, clipped)
	if !openOK {
		t.Fatal("the unclipped video painted no play button")
	}
	if !clippedOK {
		t.Fatal("the clipped video painted no play button: its controls vanished with the clip")
	}
	// The wrapper adds no vertical offset of its own, so the button belongs at the
	// same document y in both. What must not happen is the bar being repositioned
	// to fit the 60px that survived the clip.
	if openY != clippedY {
		t.Fatalf("the play button moved from y=%d to y=%d when the element was clipped", openY, clippedY)
	}
}

// TestMediaWithoutABackendSaysSo pins the honest half of the control bar. A
// session with no playback backend paints its message and no bar at all: a bar
// that can never do anything is a control the user can click and that will
// never answer, which is worse than an admission that nothing will.
func TestMediaWithoutABackendSaysSo(t *testing.T) {
	sess, err := engine.NewSession(
		`<html><body><video id="v" controls width="320" height="180"></video></body></html>`,
		nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var text string
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		for _, g := range cmd.Text.Glyphs {
			if g.Rune == '►' {
				t.Error("a session with no backend painted a play button")
			}
			text += string(g.Rune)
		}
	}
	if !strings.Contains(text, "playback unsupported") {
		t.Errorf("the element painted %q, which does not say playback is unsupported", text)
	}
}

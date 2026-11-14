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

func TestMediaControlsStayPutWhenClipped(t *testing.T) {
	unclipped := `<html><body><video id="v" controls width="320" height="180"></video></body></html>`
	clipped := `<html><body><div style="height:170px;overflow:hidden">` +
		`<video id="v" controls width="320" height="180"></video></div></body></html>`

	paintIt := func(t *testing.T, html string) (int32, bool) {
		t.Helper()
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
	if openY != clippedY {
		t.Fatalf("the play button moved from y=%d to y=%d when the element was clipped", openY, clippedY)
	}
}

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

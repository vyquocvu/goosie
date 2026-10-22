package paint_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/paint"
)

// stubBackend makes the session a playable one. Without a backend the video
// paints its unsupported message and no control bar at all, and the bar is what
// this test is about. Nothing is ever opened: Open declines, which leaves the
// element failed rather than unsupported, and a failed element still owns its
// controls.
type stubBackend struct{}

func (stubBackend) Name() string { return "stub" }

func (stubBackend) Open(string) (media.Player, error) {
	return nil, errors.New("this test opens nothing")
}

func TestVideoAndAudioPainting(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<video width="320" height="240" controls></video>
<audio controls></audio>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithMedia(stubBackend{},
			func(context.Context, string) (engine.MediaResponse, error) {
				return engine.MediaResponse{}, errors.New("this test fetches nothing")
			}, "http://example.test/"),
	)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}

	list := dl.Build(1)
	if list == nil {
		t.Fatalf("Build returned nil display list")
	}

	commands := list.All()
	if len(commands) == 0 {
		t.Fatalf("expected non-empty display commands for video and audio")
	}

	var hasPlayGlyph bool
	var hasAudioBorder bool
	var hasTimelineTrack bool

	for _, cmd := range commands {
		if cmd.Kind == paint.CmdText {
			for _, g := range cmd.Text.Glyphs {
				if g.Rune == '►' {
					hasPlayGlyph = true
				}
			}
		}
		if cmd.Kind == paint.CmdBorder {
			if cmd.Border.Top.Width > 0 && cmd.Radius.TL.X > 0 {
				hasAudioBorder = true
			}
		}
		if cmd.Kind == paint.CmdFill {
			// Video or audio progress tracks/fills
			if cmd.Rect.H() == 4 || cmd.Rect.H() == 32 {
				hasTimelineTrack = true
			}
		}
	}

	if !hasPlayGlyph {
		t.Errorf("expected paint commands to contain play symbol '►'")
	}
	if !hasAudioBorder {
		t.Errorf("expected paint commands to contain rounded audio border")
	}
	if !hasTimelineTrack {
		t.Errorf("expected paint commands to contain timeline track fills")
	}
}

package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestVideoAndAudioPainting(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<video width="320" height="240" controls></video>
<audio controls></audio>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
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
				if g.Rune == '▶' {
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
		t.Errorf("expected paint commands to contain play symbol '▶'")
	}
	if !hasAudioBorder {
		t.Errorf("expected paint commands to contain rounded audio border")
	}
	if !hasTimelineTrack {
		t.Errorf("expected paint commands to contain timeline track fills")
	}
}

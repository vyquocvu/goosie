package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestCaretColorPainting(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body style="margin: 0;">
<input id="target" value="hello" style="caret-color: rgb(255, 0, 128); font-size: 16px; width: 200px; height: 30px;">
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	// Focus the input element
	sess.FocusNext()

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}

	list := dl.Build(1)
	if list == nil {
		t.Fatalf("Build returned nil display list")
	}

	commands := list.All()
	foundCaret := false
	for _, cmd := range commands {
		if cmd.Kind == paint.CmdFill {
			if cmd.Color.R() == 255 && cmd.Color.G() == 0 && cmd.Color.B() == 128 {
				foundCaret = true
				break
			}
		}
	}

	if !foundCaret {
		t.Errorf("caret fill command with rgb(255, 0, 128) not found in display list")
	}
}

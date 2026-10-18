package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestProgressAndMeterPainting(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body style="margin: 0;">
<progress value="50" max="100" style="width: 200px; height: 20px;"></progress>
<meter value="80" min="0" max="100" low="30" high="70" optimum="90" style="width: 100px; height: 16px;"></meter>
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
		t.Fatalf("expected non-empty display commands for progress and meter")
	}

	var hasProgressFill bool
	var hasMeterFill bool

	for _, cmd := range commands {
		if cmd.Kind == paint.CmdFill {
			// Progress fill bar has blue RGB(0, 117, 255)
			if cmd.Color.R() == 0 && cmd.Color.G() == 117 && cmd.Color.B() == 255 {
				hasProgressFill = true
			}
			// Meter optimum fill bar has green RGB(40, 167, 69)
			if cmd.Color.R() == 40 && cmd.Color.G() == 167 && cmd.Color.B() == 69 {
				hasMeterFill = true
			}
		}
	}

	if !hasProgressFill {
		t.Error("progress bar fill command not found in display list")
	}
	if !hasMeterFill {
		t.Error("meter bar fill command not found in display list")
	}
}

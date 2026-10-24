package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// The select control's indicator is a downward triangle: its widest row is the
// top one. The rows used to widen downward, drawing an upward caret.
func TestSelectArrowPointsDown(t *testing.T) {
	html := `<!doctype html><html><body style="margin:0"><select style="width:100px;height:20px"><option>X</option></select></body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}

	type row struct{ y, w int32 }
	var rows []row
	for _, cmd := range list.Build(1).All() {
		if cmd.Kind != paint.CmdFill || cmd.Color.R() != 80 || cmd.Color.G() != 80 || cmd.Color.B() != 80 {
			continue
		}
		rows = append(rows, row{y: cmd.Rect.Y0, w: cmd.Rect.X1 - cmd.Rect.X0})
	}
	if len(rows) != 3 {
		t.Fatalf("found %d arrow rows, want 3: %+v", len(rows), rows)
	}
	if rows[0].w <= rows[2].w {
		t.Errorf("arrow rows %+v widen downward; want the top row widest", rows)
	}
}

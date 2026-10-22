package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestExplicitZeroBorderDoesNotPaint(t *testing.T) {
	html := `<!doctype html>
<html><head><style>
body { margin: 0; }
.drawer { position: fixed; inset: 0 auto 0 0; width: 400px; height: 100%; border: 0 solid red; }
</style></head><body><div class="drawer"></div></body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	for _, cmd := range list.Build(1).All() {
		if cmd.Kind != paint.CmdBorder {
			continue
		}
		for _, side := range []paint.SideSpec{cmd.Border.Top, cmd.Border.Right, cmd.Border.Bottom, cmd.Border.Left} {
			if side.Width > 0 && side.Color == frame.RGB(255, 0, 0) {
				t.Fatalf("explicit zero-width border emitted a red border command: %+v", cmd.Border)
			}
		}
	}
}

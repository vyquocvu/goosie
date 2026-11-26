package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-330
func TestOuterBoxShadowUsesFloorSnappedBorderBox(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin:0;">
<div style="position:absolute;left:10.25px;top:10.25px;width:40.25px;height:20.25px;box-shadow:0 1px rgb(1,2,3);"></div>
</body></html>`, nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cmd := range list.All() {
		if cmd.Kind == paint.CmdFill && uint32(cmd.Color) == 0x010203ff &&
			cmd.Rect.Y0 == 30 && cmd.Rect.Y1 == 31 && cmd.Rect.X0 == 10 && cmd.Rect.X1 == 50 {
			found = true
			break
		}
	}
	if !found {
		t.Error("no 1px shadow strip on row 30 from the floor-snapped border box")
	}
}

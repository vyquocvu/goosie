package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-329
func TestPercentTranslateMovesTheWholeSubtree(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin:0;">
<div style="position:absolute;left:50%;width:400px;height:100px;transform:translateX(-50%);background:rgb(1,2,3);">
  <div style="width:100px;height:20px;background:rgb(4,5,6);"></div>
</div>
</body></html>`, nil, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	var outer, inner bool
	for _, cmd := range list.All() {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		switch uint32(cmd.Color) {
		case 0x010203ff:
			if cmd.Rect.X0 == 300 && cmd.Rect.Y0 == 0 && cmd.Rect.X1 == 700 {
				outer = true
			}
		case 0x040506ff:
			if cmd.Rect.X0 == 300 && cmd.Rect.Y0 == 0 && cmd.Rect.X1 == 400 {
				inner = true
			}
		}
	}
	if !outer || !inner {
		t.Errorf("percentage translate did not move outer/inner fills: outer=%v inner=%v", outer, inner)
	}
}

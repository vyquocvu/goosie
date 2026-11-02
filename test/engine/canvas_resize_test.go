package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

// TestCanvasResizeDropsOldFrameFromLayout is the BR-116 engine oracle: after a
// page draws at the declared size and then resizes the canvas, the replayed
// bitmap and the layout box must both follow the new width/height, and the
// previous frame must not survive. The script fills the whole 320x200 canvas,
// resizes to 200x120, then paints one small rect, so any surviving old ops are
// visible as an opaque pixel far from the new rect.
//
// qc:BR-116
func TestCanvasResizeDropsOldFrameFromLayout(t *testing.T) {
	html := `<html><body>
		<canvas id="board" width="320" height="200"></canvas>
		<script>
			var c = document.getElementById('board');
			var ctx = c.getContext('2d');
			ctx.fillRect(0, 0, 320, 200);
			c.width = 200;
			c.height = 120;
			ctx.fillRect(10, 10, 20, 20);
		</script>
	</body></html>`
	sess := canvasSession(t, html)

	img := canvasImage(t, sess, "board")
	if img == nil {
		t.Fatal("resized canvas produced no bitmap")
	}
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 120 {
		t.Fatalf("canvas bitmap after resize is %dx%d, want 200x120", b.Dx(), b.Dy())
	}

	node := sess.Doc.ElementByID("board")
	oid, ok := sess.Arena.ForNode(node.ID)
	if !ok {
		t.Fatal("canvas #board was not laid out")
	}
	if obj := sess.Arena.Get(oid); obj.W != 200 || obj.H != 120 {
		t.Fatalf("canvas layout box after resize is %.0fx%.0f, want 200x120", obj.W, obj.H)
	}

	var painted bool
	for _, cmd := range sess.Paint(1).All() {
		if cmd.Kind != paint.CmdImage || cmd.Image.Src == nil {
			continue
		}
		b := cmd.Image.Src.Bounds()
		if b.Dx() == 320 && b.Dy() == 200 {
			t.Fatal("paint still carried the pre-resize 320x200 canvas bitmap")
		}
		if b.Dx() == 200 && b.Dy() == 120 {
			painted = true
		}
	}
	if !painted {
		t.Fatal("no image command carried the 200x120 canvas bitmap")
	}

	got, err := sess.EvaluateJS(`(function(){
		var d = document.getElementById('board').getContext('2d').getImageData(0, 0, 200, 120);
		return [d.width, d.height,
			d.data[(15 * 200 + 15) * 4 + 3],
			d.data[(100 * 200 + 150) * 4 + 3]].join(',');
	})()`)
	if err != nil {
		t.Fatalf("getImageData: %v", err)
	}
	const want = "200,120,255,0"
	if got != want {
		t.Fatalf("canvas pixels after resize = %s, want %s (width, height, new rect alpha, old frame alpha)", got, want)
	}
}

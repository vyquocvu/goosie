package engine_test

import (
	"image"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

func canvasSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess
}

// canvasImage returns the pixels the engine attached to the canvas element, or
// nil when the element was not rasterized (JS absent or size rejected).
func canvasImage(t *testing.T, sess *engine.Session, id string) image.Image {
	t.Helper()
	node := sess.Doc.ElementByID(id)
	if node == nil {
		t.Fatalf("canvas #%s missing from the document", id)
	}
	oid, ok := sess.Arena.ForNode(node.ID)
	if !ok {
		t.Fatalf("canvas #%s was not laid out", id)
	}
	img, _ := sess.Arena.Get(oid).Image.(image.Image)
	return img
}

// TestCanvasPrimitivesRasterizeAtDeclaredSize is the BR-115 oracle: a document
// that draws rects, paths and arcs must produce a canvas bitmap whose intrinsic
// size is exactly the element's width/height, and the recorded primitives must
// reach those pixels. The engine replays canvas ops into a frame.Bitmap before
// paint; before that replay there was no oracle for the resulting size or
// contents, only for the op list.
//
// qc:BR-115
func TestCanvasPrimitivesRasterizeAtDeclaredSize(t *testing.T) {
	html := `<html><body>
		<canvas id="board" width="320" height="200"></canvas>
		<script>
			var ctx = document.getElementById('board').getContext('2d');
			ctx.fillRect(10, 20, 60, 30);
			ctx.strokeRect(100, 20, 40, 40);
			ctx.beginPath();
			ctx.moveTo(10, 100);
			ctx.lineTo(120, 160);
			ctx.lineTo(10, 160);
			ctx.closePath();
			ctx.fill();
			ctx.beginPath();
			ctx.arc(200, 100, 25, 0, Math.PI * 2);
			ctx.stroke();
			ctx.clearRect(0, 0, 5, 5);
		</script>
	</body></html>`
	sess := canvasSession(t, html)

	img := canvasImage(t, sess, "board")
	if img == nil {
		t.Fatal("canvas primitives produced no bitmap")
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 200 {
		t.Fatalf("canvas bitmap is %dx%d, want 320x200", b.Dx(), b.Dy())
	}

	var painted bool
	for _, cmd := range sess.Paint(1).All() {
		if cmd.Kind != paint.CmdImage || cmd.Image.Src == nil {
			continue
		}
		b := cmd.Image.Src.Bounds()
		if b.Dx() == 320 && b.Dy() == 200 {
			painted = true
		}
	}
	if !painted {
		t.Fatal("no image command carried the 320x200 canvas bitmap")
	}

	got, err := sess.EvaluateJS(`(function(){
		var c = document.getElementById('board');
		var d = c.getContext('2d').getImageData(0, 0, 320, 200);
		return [d.width, d.height, d.data.length,
			d.data[(20 * 320 + 10) * 4 + 3],
			d.data[(180 * 320 + 300) * 4 + 3],
			d.data[3]].join(',');
	})()`)
	if err != nil {
		t.Fatalf("getImageData: %v", err)
	}
	const want = "320,200,256000,255,0,0"
	if got != want {
		t.Fatalf("canvas pixels = %s, want %s (width,height,length, painted alpha, unpainted alpha, cleared alpha)", got, want)
	}
}

// TestCanvasOversizedDimensionIsNotRasterized keeps a declared canvas width
// beyond the device dimension cap from allocating a bitmap at all. The replay
// allocation is sized from the element's width/height, so without an admission
// check a hostile attribute value reserves the whole product in one make.
//
// qc:BR-115
func TestCanvasOversizedDimensionIsNotRasterized(t *testing.T) {
	// 17000 is just over MaxDeviceDimension (16384) but only one row tall, so
	// the pre-fix allocation stays small: the oracle is the admission check,
	// not memory pressure.
	html := `<html><body>
		<canvas id="huge" width="17000" height="1"></canvas>
		<script>
			document.getElementById('huge').getContext('2d').fillRect(0, 0, 10, 1);
		</script>
	</body></html>`
	sess := canvasSession(t, html)
	if img := canvasImage(t, sess, "huge"); img != nil {
		t.Fatalf("oversized canvas rasterized a %dx%d bitmap, want none", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

// TestCanvasOversizedPixelAreaIsNotRasterized checks the pixel budget rather
// than the per-dimension cap: 4097x4096 is one pixel past the 16 Mi-pixel
// device budget while both sides stay under MaxDeviceDimension.
//
// qc:BR-115
func TestCanvasOversizedPixelAreaIsNotRasterized(t *testing.T) {
	html := `<html><body>
		<canvas id="wide" width="4097" height="4096"></canvas>
		<script>
			document.getElementById('wide').getContext('2d').fillRect(0, 0, 10, 10);
		</script>
	</body></html>`
	sess := canvasSession(t, html)
	if img := canvasImage(t, sess, "wide"); img != nil {
		t.Fatalf("canvas over the pixel budget rasterized a %dx%d bitmap, want none", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

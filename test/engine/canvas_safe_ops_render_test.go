package engine_test

import (
	"image"
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

func TestCanvasSafeOperationsReachPagePaint(t *testing.T) {
	html := `<html><body><canvas id="board" width="96" height="96"></canvas><script>
		var ctx = document.getElementById('board').getContext('2d');
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(2, 3, 8, 9);
		ctx.globalAlpha = 0.5;
		ctx.fillStyle = '#0000ff';
		ctx.fillRect(4, 3, 8, 9);
		ctx.save();
		ctx.globalAlpha = 0.5;
		ctx.fillStyle = '#00ff00';
		ctx.translate(20, 4);
		ctx.fillRect(0, 0, 4, 4);
		ctx.restore();
		ctx.globalAlpha = 1;
		ctx.fillStyle = '#ff8000';
		ctx.beginPath();
		ctx.moveTo(42, 30);
		ctx.lineTo(52, 30);
		ctx.lineTo(42, 40);
		ctx.closePath();
		ctx.fill();
		ctx.save();
		ctx.beginPath();
		ctx.rect(60, 60, 20, 20);
		ctx.clip();
		ctx.fillStyle = '#ff00ff';
		ctx.fillRect(40, 40, 60, 60);
		ctx.restore();
		ctx.fillStyle = '#ffff00';
		ctx.fillRect(2, 70, 20, 20);
		ctx.clearRect(5, 73, 8, 8);
	</script></body></html>`
	sess := canvasSession(t, html)
	bitmap := canvasImage(t, sess, "board")
	if bitmap == nil {
		t.Fatal("canvas produced no bitmap")
	}
	want := map[[2]int][]byte{
		{5, 6}:   {127, 0, 128, 255},
		{21, 5}:  {0, 255, 0, 128},
		{44, 32}: {255, 128, 0, 255},
		{65, 65}: {255, 0, 255, 255},
		{59, 65}: {0, 0, 0, 0},
		{6, 74}:  {0, 0, 0, 0},
		{4, 74}:  {255, 255, 0, 255},
	}
	for point, expected := range want {
		if got := canvasRGBAAt(t, bitmap, point[0], point[1]); !reflect.DeepEqual(got, expected) {
			t.Errorf("canvas pixel (%d,%d) = %v, want %v", point[0], point[1], got, expected)
		}
	}

	var pageImage *image.RGBA
	for _, cmd := range sess.Paint(1).All() {
		if cmd.Kind != paint.CmdImage || cmd.Image.Src == nil {
			continue
		}
		if cmd.Image.Src.Bounds().Dx() != 96 || cmd.Image.Src.Bounds().Dy() != 96 {
			continue
		}
		pageImage, _ = cmd.Image.Src.(*image.RGBA)
		if pageImage != nil && cmd.Rect.W() == 96 && cmd.Rect.H() == 96 {
			break
		}
	}
	if pageImage == nil {
		t.Fatal("no page image command carried the 96x96 canvas bitmap")
	}
	for point, expected := range want {
		if got := canvasRGBAAt(t, pageImage, point[0], point[1]); !reflect.DeepEqual(got, expected) {
			t.Errorf("page paint pixel (%d,%d) = %v, want %v", point[0], point[1], got, expected)
		}
	}
}

func canvasRGBAAt(t *testing.T, img image.Image, x, y int) []byte {
	t.Helper()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("image is %T, want *image.RGBA", img)
	}
	offset := rgba.PixOffset(x, y)
	pixel := rgba.Pix[offset : offset+4 : offset+4]
	if pixel[3] == 255 || pixel[3] == 0 {
		return append([]byte(nil), pixel...)
	}
	out := []byte{pixel[0], pixel[1], pixel[2], pixel[3]}
	for channel := 0; channel < 3; channel++ {
		out[channel] = byte((int(pixel[channel])*255 + int(pixel[3])/2) / int(pixel[3]))
	}
	return out
}

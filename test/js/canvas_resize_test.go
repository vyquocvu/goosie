package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func canvasResizeRuntime(t *testing.T, width, height string) (*js.Runtime, *dom.Node) {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	if width != "" {
		canvas.SetAttribute("width", width)
	}
	if height != "" {
		canvas.SetAttribute("height", height)
	}
	body.AppendChild(canvas)

	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	r.SetDOM(doc)
	if err := r.Run(`var canvas = document.querySelector("canvas"); var ctx = canvas.getContext("2d");`, "setup.js"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return r, canvas
}

// qc:BR-116
func TestCanvasResizeClearsRecordedFrameAndFollowsNewSize(t *testing.T) {
	r, canvas := canvasResizeRuntime(t, "320", "200")
	if err := r.Run(`ctx.fillRect(0, 0, 320, 200);`, "draw.js"); err != nil {
		t.Fatalf("draw: %v", err)
	}
	if ops := r.CanvasOps(int(canvas.ID)); len(ops) == 0 {
		t.Fatal("precondition failed: fillRect recorded no ops")
	}

	if err := r.Run(`canvas.width = 200; canvas.height = 120;`, "resize.js"); err != nil {
		t.Fatalf("resize: %v", err)
	}

	if w, h := r.CanvasDimensions(int(canvas.ID)); w != 200 || h != 120 {
		t.Fatalf("canvas dimensions after IDL resize = %dx%d, want 200x120", w, h)
	}
	if ops := r.CanvasOps(int(canvas.ID)); len(ops) != 0 {
		t.Fatalf("IDL resize kept %d recorded ops, want the drawing state cleared", len(ops))
	}

	got, err := r.Evaluate(`(function(){
		var d = ctx.getImageData(0, 0, 200, 120);
		return [d.width, d.height, d.data.length, d.data[(50 * 200 + 50) * 4 + 3]].join(",");
	})()`)
	if err != nil {
		t.Fatalf("getImageData: %v", err)
	}
	if want := "200,120,96000,0"; got != want {
		t.Fatalf("canvas after IDL resize = %s, want %s (dimensions, byte length, cleared alpha)", got, want)
	}
}

// qc:BR-116
func TestCanvasAttributeResizeAlsoResets(t *testing.T) {
	r, canvas := canvasResizeRuntime(t, "320", "200")
	if err := r.Run(`ctx.fillRect(0, 0, 320, 200);`, "draw.js"); err != nil {
		t.Fatalf("draw: %v", err)
	}

	if err := r.Run(`canvas.setAttribute("width", "64"); canvas.setAttributeNS(null, "height", "48");`, "set-attribute.js"); err != nil {
		t.Fatalf("set attributes: %v", err)
	}
	if w, h := r.CanvasDimensions(int(canvas.ID)); w != 64 || h != 48 {
		t.Fatalf("canvas dimensions after setAttribute = %dx%d, want 64x48", w, h)
	}
	if ops := r.CanvasOps(int(canvas.ID)); len(ops) != 0 {
		t.Fatalf("setAttribute resize kept %d recorded ops, want the drawing state cleared", len(ops))
	}

	if err := r.Run(`ctx.fillRect(0, 0, 10, 10); canvas.removeAttribute("width");`, "remove-attribute.js"); err != nil {
		t.Fatalf("remove attribute: %v", err)
	}
	if w, h := r.CanvasDimensions(int(canvas.ID)); w != 300 || h != 48 {
		t.Fatalf("canvas dimensions after removeAttribute = %dx%d, want the 300x48 defaults", w, h)
	}
	if ops := r.CanvasOps(int(canvas.ID)); len(ops) != 0 {
		t.Fatalf("removeAttribute resize kept %d recorded ops, want the drawing state cleared", len(ops))
	}

	got, err := r.Evaluate(`(function(){
		var d = ctx.getImageData(0, 0, 300, 48);
		return [d.width, d.height, d.data.length, d.data[(5 * 300 + 5) * 4 + 3]].join(",");
	})()`)
	if err != nil {
		t.Fatalf("getImageData: %v", err)
	}
	if want := "300,48,57600,0"; got != want {
		t.Fatalf("canvas after removeAttribute = %s, want %s (default width, kept height, cleared alpha)", got, want)
	}
}

// qc:BR-116
func TestCanvasResizeToSameValueStillClears(t *testing.T) {
	r, canvas := canvasResizeRuntime(t, "320", "200")
	if err := r.Run(`ctx.fillRect(0, 0, 320, 200); canvas.width = 320;`, "same-size.js"); err != nil {
		t.Fatalf("same-size resize: %v", err)
	}
	if ops := r.CanvasOps(int(canvas.ID)); len(ops) != 0 {
		t.Fatalf("same-value resize kept %d recorded ops, want the drawing state cleared", len(ops))
	}
	got, err := r.Evaluate(`(function(){
		var d = ctx.getImageData(10, 10, 1, 1);
		return [d.data[0], d.data[1], d.data[2], d.data[3]].join(",");
	})()`)
	if err != nil {
		t.Fatalf("getImageData: %v", err)
	}
	if want := "0,0,0,0"; got != want {
		t.Fatalf("pixel after same-value resize = %s, want %s (transparent)", got, want)
	}
}

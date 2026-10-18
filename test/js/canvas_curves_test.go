package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasCurveOperations(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	canvas.SetAttribute("width", "200")
	canvas.SetAttribute("height", "200")
	body.AppendChild(canvas)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer r.Close()

	r.SetDOM(doc)

	script := `
		var canvas = document.querySelector("canvas");
		var ctx = canvas.getContext("2d");
		ctx.beginPath();
		ctx.moveTo(10, 10);
		ctx.bezierCurveTo(20, 100, 150, 100, 180, 20);
		ctx.quadraticCurveTo(100, 180, 20, 20);
		ctx.ellipse(100, 100, 50, 30, 0, 0, Math.PI * 2);
		ctx.stroke();
	`
	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ops := r.CanvasOps(int(canvas.ID))
	if len(ops) == 0 {
		t.Fatal("expected recorded canvas ops, got none")
	}

	hasBezier := false
	hasQuadratic := false
	hasEllipse := false

	var frameOps []frame.CanvasOp
	for _, op := range ops {
		frameOps = append(frameOps, frame.CanvasOp{
			Method: op.Method,
			Args:   op.Args,
		})
		switch op.Method {
		case "bezierCurveTo":
			hasBezier = true
		case "quadraticCurveTo":
			hasQuadratic = true
		case "ellipse":
			hasEllipse = true
		}
	}

	if !hasBezier {
		t.Error("missing bezierCurveTo in canvas ops")
	}
	if !hasQuadratic {
		t.Error("missing quadraticCurveTo in canvas ops")
	}
	if !hasEllipse {
		t.Error("missing ellipse in canvas ops")
	}

	// Replay operations into frame bitmap to ensure rasterizer path works
	bm := frame.NewBitmap(200, 200)
	frame.ReplayCanvasOps(bm, frameOps, 200, 200)

	// Verify that non-zero pixels were drawn
	hasDrawnPixels := false
	for _, b := range bm.RGBA {
		if b > 0 {
			hasDrawnPixels = true
			break
		}
	}
	if !hasDrawnPixels {
		t.Error("expected non-zero pixels drawn to bitmap after ReplayCanvasOps")
	}
}

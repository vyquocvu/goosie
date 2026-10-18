package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasRoundRectAndReset(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	canvas.SetAttribute("width", "400")
	canvas.SetAttribute("height", "300")
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

		// Mutate state
		ctx.fillStyle = "#ff0000";
		ctx.lineWidth = 5.0;
		ctx.filter = "blur(2px)";
		ctx.imageSmoothingEnabled = false;

		// Path with roundRect
		ctx.beginPath();
		ctx.roundRect(10, 10, 100, 80, 8);
		ctx.roundRect(120, 10, 100, 80, [4, 8, 12, 16]);
		ctx.fill();

		// Path2D with roundRect
		var p = new Path2D();
		p.roundRect(0, 0, 50, 50, 5);
		ctx.stroke(p);

		// Now reset
		ctx.reset();

		if (ctx.fillStyle !== "#000000") {
			throw new Error("expected fillStyle #000000 after reset, got " + ctx.fillStyle);
		}
		if (ctx.lineWidth !== 1.0) {
			throw new Error("expected lineWidth 1.0 after reset, got " + ctx.lineWidth);
		}
		if (ctx.filter !== "none") {
			throw new Error("expected filter none after reset, got " + ctx.filter);
		}
		if (ctx.imageSmoothingEnabled !== true) {
			throw new Error("expected imageSmoothingEnabled true after reset, got " + ctx.imageSmoothingEnabled);
		}
	`

	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ops := r.CanvasOps(int(canvas.ID))
	hasRoundRect := false
	hasReset := false
	for _, op := range ops {
		if op.Method == "roundRect" {
			hasRoundRect = true
		}
		if op.Method == "reset" {
			hasReset = true
		}
	}

	if !hasRoundRect {
		t.Errorf("roundRect op not found in recorded ops")
	}
	if !hasReset {
		t.Errorf("reset op not found in recorded ops")
	}
}

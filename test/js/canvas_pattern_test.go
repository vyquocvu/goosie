package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasEllipseAndPattern(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	canvas.SetAttribute("width", "300")
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

		// Test ellipse on context
		ctx.beginPath();
		ctx.ellipse(100, 100, 50, 75, Math.PI / 4, 0, 2 * Math.PI, false);
		ctx.fill();

		// Test ellipse on Path2D
		var p = new Path2D();
		p.ellipse(50, 50, 20, 30, 0, 0, Math.PI);
		ctx.stroke(p);

		// Test createPattern
		var pattern = ctx.createPattern(canvas, "repeat");
		if (!pattern || typeof pattern.setTransform !== "function") {
			throw new Error("Invalid CanvasPattern returned");
		}
		pattern.setTransform([1, 0, 0, 1, 0, 0]);
		ctx.fillStyle = pattern;
	`

	if err := r.Run(script, "ellipse_pattern_test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ops := r.CanvasOps(int(canvas.ID))
	hasEllipse := false
	for _, op := range ops {
		if op.Method == "ellipse" {
			hasEllipse = true
			if len(op.Args) < 8 {
				t.Errorf("ellipse op args len = %d, want at least 8", len(op.Args))
			}
		}
	}
	if !hasEllipse {
		t.Errorf("expected ellipse drawing operation recorded in canvas ops")
	}
}

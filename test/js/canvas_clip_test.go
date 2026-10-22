package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasClipMethod(t *testing.T) {
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

		// Clip with default nonzero rule
		ctx.beginPath();
		ctx.rect(10, 10, 100, 100);
		ctx.clip();

		// Clip with evenodd rule
		ctx.beginPath();
		ctx.arc(50, 50, 30, 0, Math.PI * 2);
		ctx.clip("evenodd");

		// Clip using Path2D object
		var p = new Path2D();
		p.rect(0, 0, 40, 40);
		ctx.clip(p, "nonzero");
	`

	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ops := r.CanvasOps(int(canvas.ID))
	clipCount := 0
	for _, op := range ops {
		if op.Method == "clip" {
			clipCount++
		}
	}

	if clipCount != 3 {
		t.Fatalf("expected 3 clip ops recorded, got %d", clipCount)
	}
}

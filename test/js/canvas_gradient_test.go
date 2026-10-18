package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasGradients(t *testing.T) {
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

		// Linear gradient
		var linGrad = ctx.createLinearGradient(0, 0, 200, 0);
		linGrad.addColorStop(0, "red");
		linGrad.addColorStop(1, "blue");

		// Radial gradient
		var radGrad = ctx.createRadialGradient(75, 75, 10, 75, 75, 50);
		radGrad.addColorStop(0, "#ff0000");
		radGrad.addColorStop(0.5, "#00ff00");
		radGrad.addColorStop(1, "#0000ff");

		// Conic gradient
		var conGrad = ctx.createConicGradient(0, 100, 100);
		conGrad.addColorStop(0, "yellow");
		conGrad.addColorStop(0.5, "magenta");
		conGrad.addColorStop(1, "cyan");

		// Ensure setting as fillStyle does not throw
		ctx.fillStyle = radGrad;
		ctx.fillRect(0, 0, 100, 100);

		ctx.fillStyle = conGrad;
		ctx.fillRect(100, 0, 100, 100);
	`

	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ops := r.CanvasOps(int(canvas.ID))
	if len(ops) < 2 {
		t.Fatalf("expected at least 2 canvas ops, got %d", len(ops))
	}
}

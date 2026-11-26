package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasTextAndPath2D(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	canvas.SetAttribute("width", "300")
	canvas.SetAttribute("height", "200")
	body.AppendChild(canvas)

	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer r.Close()
	r.SetDOM(doc)

	script := `
		var canvas = document.querySelector("canvas");
		var ctx = canvas.getContext("2d");
		ctx.font = "20px sans-serif";
		var metrics = ctx.measureText("Hello World");
		if (!metrics || typeof metrics.width !== "number" || metrics.width <= 0) {
			throw new Error("measureText failed: " + JSON.stringify(metrics));
		}
		var fillTextError = "";
		var strokeTextError = "";
		try { ctx.fillText("Hello", 10, 20, 100); } catch (e) { fillTextError = e.name; }
		try { ctx.strokeText("World", 10, 50, 100); } catch (e) { strokeTextError = e.name; }
		var p1 = new Path2D();
		p1.moveTo(0, 0);
		p1.lineTo(100, 100);
		var p2 = new Path2D(p1);
		p2.lineTo(200, 100);
		p2.closePath();
		ctx.stroke(p2);
		ctx.fill(p1);
	`
	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	for _, name := range []string{"fillTextError", "strokeTextError"} {
		value, ok := r.Global(name)
		if !ok || value.Str != "NotSupportedError" {
			t.Errorf("%s = %v, want NotSupportedError", name, value)
		}
	}

	ops := r.CanvasOps(int(canvas.ID))
	hasLineTo := false
	hasStroke := false
	hasFill := false
	for _, op := range ops {
		switch op.Method {
		case "lineTo":
			hasLineTo = true
		case "stroke":
			hasStroke = true
		case "fill":
			hasFill = true
		}
	}
	if !hasLineTo {
		t.Error("missing lineTo (from Path2D playback) in canvas ops")
	}
	if !hasStroke {
		t.Error("missing stroke in canvas ops")
	}
	if !hasFill {
		t.Error("missing fill in canvas ops")
	}
}

package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasTextMetrics(t *testing.T) {
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
		ctx.font = "20px sans-serif";
		var metrics = ctx.measureText("Hello World");
		if (typeof metrics.width !== "number" || metrics.width <= 0) {
			throw new Error("Invalid width: " + metrics.width);
		}
		if (typeof metrics.actualBoundingBoxAscent !== "number" || metrics.actualBoundingBoxAscent <= 0) {
			throw new Error("Invalid actualBoundingBoxAscent: " + metrics.actualBoundingBoxAscent);
		}
		if (typeof metrics.actualBoundingBoxDescent !== "number" || metrics.actualBoundingBoxDescent <= 0) {
			throw new Error("Invalid actualBoundingBoxDescent: " + metrics.actualBoundingBoxDescent);
		}
		if (typeof metrics.fontBoundingBoxAscent !== "number" || metrics.fontBoundingBoxAscent <= 0) {
			throw new Error("Invalid fontBoundingBoxAscent: " + metrics.fontBoundingBoxAscent);
		}
		if (typeof metrics.fontBoundingBoxDescent !== "number" || metrics.fontBoundingBoxDescent <= 0) {
			throw new Error("Invalid fontBoundingBoxDescent: " + metrics.fontBoundingBoxDescent);
		}
		if (typeof metrics.actualBoundingBoxLeft !== "number") {
			throw new Error("Invalid actualBoundingBoxLeft: " + metrics.actualBoundingBoxLeft);
		}
		if (typeof metrics.actualBoundingBoxRight !== "number" || metrics.actualBoundingBoxRight !== metrics.width) {
			throw new Error("Invalid actualBoundingBoxRight: " + metrics.actualBoundingBoxRight);
		}

		ctx.textAlign = "center";
		var centerMetrics = ctx.measureText("Hello World");
		if (centerMetrics.actualBoundingBoxLeft !== -centerMetrics.width / 2) {
			throw new Error("Invalid centered actualBoundingBoxLeft: " + centerMetrics.actualBoundingBoxLeft);
		}
		if (centerMetrics.actualBoundingBoxRight !== centerMetrics.width / 2) {
			throw new Error("Invalid centered actualBoundingBoxRight: " + centerMetrics.actualBoundingBoxRight);
		}
	`

	if err := r.Run(script, "metrics_test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

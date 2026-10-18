package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCanvasFilterAndSmoothing(t *testing.T) {
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

		// Test default properties
		if (ctx.imageSmoothingEnabled !== true) {
			throw new Error("expected default imageSmoothingEnabled === true, got " + ctx.imageSmoothingEnabled);
		}
		if (ctx.imageSmoothingQuality !== "low") {
			throw new Error("expected default imageSmoothingQuality === 'low', got " + ctx.imageSmoothingQuality);
		}
		if (ctx.filter !== "none") {
			throw new Error("expected default filter === 'none', got " + ctx.filter);
		}
		if (ctx.globalAlpha !== 1.0) {
			throw new Error("expected default globalAlpha === 1.0, got " + ctx.globalAlpha);
		}
		if (ctx.globalCompositeOperation !== "source-over") {
			throw new Error("expected default globalCompositeOperation === 'source-over', got " + ctx.globalCompositeOperation);
		}

		// Test property mutations
		ctx.imageSmoothingEnabled = false;
		if (ctx.imageSmoothingEnabled !== false) {
			throw new Error("failed to set imageSmoothingEnabled to false");
		}

		ctx.imageSmoothingQuality = "high";
		if (ctx.imageSmoothingQuality !== "high") {
			throw new Error("failed to set imageSmoothingQuality to high");
		}

		ctx.filter = "blur(4px)";
		if (ctx.filter !== "blur(4px)") {
			throw new Error("failed to set filter to blur(4px)");
		}

		// Test line dash methods
		var dash = ctx.getLineDash();
		if (!Array.isArray(dash) || dash.length !== 0) {
			throw new Error("expected empty line dash initial array");
		}
		ctx.setLineDash([5, 10]);
		dash = ctx.getLineDash();
		if (dash.length !== 2 || dash[0] !== 5 || dash[1] !== 10) {
			throw new Error("failed to set/get line dash");
		}

		// Test imageData creation
		var imgData = ctx.createImageData(20, 30);
		if (imgData.width !== 20 || imgData.height !== 30 || imgData.data.length !== 20 * 30 * 4) {
			throw new Error("createImageData failed");
		}

		var gotData = ctx.getImageData(0, 0, 10, 15);
		if (gotData.width !== 10 || gotData.height !== 15 || gotData.data.length !== 10 * 15 * 4) {
			throw new Error("getImageData failed");
		}

		ctx.putImageData(gotData, 0, 0);
		ctx.drawImage({ src: "test.png" }, 0, 0, 50, 50);
	`

	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	ops := r.CanvasOps(int(canvas.ID))
	hasPut := false
	hasDraw := false
	for _, op := range ops {
		if op.Method == "putImageData" {
			hasPut = true
		}
		if op.Method == "drawImage" {
			hasDraw = true
		}
	}
	if !hasPut {
		t.Errorf("expected putImageData op recorded")
	}
	if !hasDraw {
		t.Errorf("expected drawImage op recorded")
	}
}

package js_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func canvasLimitRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	canvas.SetAttribute("width", "320")
	canvas.SetAttribute("height", "200")
	body.AppendChild(canvas)

	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	r.SetDOM(doc)
	if err := r.Run(`var ctx = document.querySelector("canvas").getContext("2d");`, "setup.js"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return r
}

// qc:BR-115
func TestCanvasGetImageDataRejectsOversizedRegionBeforeAllocating(t *testing.T) {
	r := canvasLimitRuntime(t)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := r.Evaluate(`(function(){
		try { ctx.getImageData(0, 0, 4097, 4096); return "no-throw"; }
		catch (e) { return e.name; }
	})()`)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got != "IndexSizeError" {
		t.Fatalf("oversized getImageData returned %q, want an IndexSizeError", got)
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 128<<20 {
		t.Fatalf("oversized getImageData allocated %d MiB before rejecting the region, want less than 128 MiB", grown>>20)
	}
}

// qc:BR-115
func TestCanvasGetImageDataKeepsSmallRegionsWorking(t *testing.T) {
	r := canvasLimitRuntime(t)
	got, err := r.Evaluate(`(function(){
		var d = ctx.getImageData(0, 0, 4, 3);
		return d.width + "x" + d.height + ":" + d.data.length;
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got != "4x3:48" {
		t.Fatalf("small getImageData = %q, want 4x3:48", got)
	}
}

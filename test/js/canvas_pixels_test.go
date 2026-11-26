package js_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
)

func safeCanvasRuntime(t *testing.T) (*js.Runtime, *dom.Node) {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	canvas := doc.NewElement("canvas")
	canvas.SetAttribute("width", "96")
	canvas.SetAttribute("height", "96")
	body.AppendChild(canvas)
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.test"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	r.SetDOM(doc)
	return r, canvas
}

func TestCanvasSafeOperationsMatchChromiumPixels(t *testing.T) {
	r, canvas := safeCanvasRuntime(t)
	gotValue, err := r.Evaluate(`(function(){
		var ctx = document.querySelector('canvas').getContext('2d', { willReadFrequently: true });
		var out = {};
		var reset = function() { ctx.reset(); };
		var pixel = function(name, x, y) {
			var d = ctx.getImageData(x, y, 1, 1).data;
			out[name] = [d[0], d[1], d[2], d[3]];
		};
		var rect = function(name, x, y, w, h) {
			out[name] = Array.from(ctx.getImageData(x, y, w, h).data);
		};
		reset();
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(2, 3, 8, 9);
		ctx.strokeStyle = '#00ff00';
		ctx.lineWidth = 2;
		ctx.strokeRect(20, 3, 12, 10);
		pixel('style.fill.interior', 5, 6);
		pixel('style.fill.outside', 11, 6);
		pixel('style.stroke.top', 25, 3);
		pixel('style.stroke.inside', 25, 6);
		pixel('style.stroke.outside', 18, 6);
		reset();
		ctx.fillStyle = 'rgba(255, 0, 0, 0.5)';
		ctx.fillRect(0, 0, 4, 4);
		ctx.globalAlpha = 0.5;
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(4, 0, 4, 4);
		ctx.fillStyle = '#0000ff';
		ctx.fillRect(8, 0, 4, 4);
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(8, 0, 4, 4);
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(12, 0, 4, 4);
		ctx.fillRect(12, 0, 4, 4);
		pixel('alpha.style-half.transparent', 1, 1);
		pixel('alpha.global-half.transparent', 5, 1);
		pixel('alpha.global-half.over-blue', 9, 1);
		pixel('alpha.overlap.transparent', 13, 1);
		reset();
		ctx.fillStyle = '#123456';
		ctx.translate(10, 6);
		ctx.fillRect(1, 2, 5, 4);
		ctx.scale(2, 1);
		ctx.fillRect(10, 2, 3, 4);
		pixel('transform.translate.interior', 12, 9);
		pixel('transform.translate.outside', 8, 9);
		pixel('transform.scale.interior', 31, 9);
		pixel('transform.scale.outside', 38, 9);
		reset();
		ctx.fillStyle = '#abcdef';
		ctx.setTransform(1, 0, 0, 1, 20, 20);
		ctx.rotate(Math.PI / 2);
		ctx.fillRect(0, 0, 8, 4);
		pixel('transform.rotate.center', 17, 21);
		pixel('transform.rotate.old-origin', 2, 2);
		reset();
		ctx.fillStyle = '#ff8000';
		ctx.beginPath();
		ctx.moveTo(2, 2);
		ctx.lineTo(12, 2);
		ctx.lineTo(2, 12);
		ctx.closePath();
		ctx.fill();
		pixel('path.triangle.interior', 4, 4);
		pixel('path.triangle.outside', 10, 10);
		ctx.strokeStyle = '#0080ff';
		ctx.lineWidth = 2;
		ctx.beginPath();
		ctx.moveTo(20, 5);
		ctx.lineTo(32, 5);
		ctx.stroke();
		pixel('path.stroke.center', 26, 5);
		pixel('path.stroke.edge-top', 26, 4);
		pixel('path.stroke.outside', 26, 7);
		ctx.fillStyle = '#00ff80';
		ctx.beginPath();
		ctx.arc(50, 20, 10, 0, Math.PI * 2);
		ctx.fill();
		pixel('path.arc.center', 50, 20);
		pixel('path.arc.interior-cardinal', 58, 20);
		pixel('path.arc.outside', 50, 5);
		reset();
		ctx.beginPath();
		ctx.rect(4, 4, 20, 20);
		ctx.clip();
		ctx.fillStyle = '#ff00ff';
		ctx.fillRect(0, 0, 40, 40);
		pixel('clip.rect.inside', 10, 10);
		pixel('clip.rect.edge-inside', 4, 10);
		pixel('clip.rect.outside', 3, 10);
		reset();
		ctx.fillStyle = '#ff0000';
		ctx.globalAlpha = 1;
		ctx.save();
		ctx.fillStyle = '#00ff00';
		ctx.globalAlpha = 0.5;
		ctx.translate(40, 0);
		ctx.fillRect(0, 0, 4, 4);
		ctx.restore();
		ctx.fillRect(4, 0, 4, 4);
		pixel('save.restore.inside', 41, 1);
		pixel('save.restore.after', 5, 1);
		pixel('save.restore.transform-after', 41, 6);
		reset();
		ctx.fillStyle = '#ffff00';
		ctx.fillRect(2, 2, 20, 20);
		ctx.clearRect(5, 5, 8, 8);
		pixel('clear.interior', 6, 6);
		pixel('clear.edge-inside', 5, 6);
		pixel('clear.outside', 4, 6);
		reset();
		ctx.fillStyle = '#ffffff';
		ctx.fillRect(70, 70, 2, 2);
		rect('pending.region', 70, 70, 2, 2);
		return JSON.stringify(out);
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	var got map[string][]int
	if err := json.Unmarshal([]byte(gotValue), &got); err != nil {
		t.Fatalf("decode pixels: %v", err)
	}
	want := map[string][]int{
		"style.fill.interior":           {255, 0, 0, 255},
		"style.fill.outside":            {0, 0, 0, 0},
		"style.stroke.top":              {0, 255, 0, 255},
		"style.stroke.inside":           {0, 0, 0, 0},
		"style.stroke.outside":          {0, 0, 0, 0},
		"alpha.style-half.transparent":  {255, 0, 0, 128},
		"alpha.global-half.transparent": {255, 0, 0, 128},
		"alpha.global-half.over-blue":   {170, 0, 85, 192},
		"alpha.overlap.transparent":     {255, 0, 0, 192},
		"transform.translate.interior":  {18, 52, 86, 255},
		"transform.translate.outside":   {0, 0, 0, 0},
		"transform.scale.interior":      {18, 52, 86, 255},
		"transform.scale.outside":       {0, 0, 0, 0},
		"transform.rotate.center":       {171, 205, 239, 255},
		"transform.rotate.old-origin":   {0, 0, 0, 0},
		"path.triangle.interior":        {255, 128, 0, 255},
		"path.triangle.outside":         {0, 0, 0, 0},
		"path.stroke.center":            {0, 128, 255, 255},
		"path.stroke.edge-top":          {0, 128, 255, 255},
		"path.stroke.outside":           {0, 0, 0, 0},
		"path.arc.center":               {0, 255, 128, 255},
		"path.arc.interior-cardinal":    {0, 255, 128, 255},
		"path.arc.outside":              {0, 0, 0, 0},
		"clip.rect.inside":              {255, 0, 255, 255},
		"clip.rect.edge-inside":         {255, 0, 255, 255},
		"clip.rect.outside":             {0, 0, 0, 0},
		"save.restore.inside":           {0, 255, 0, 128},
		"save.restore.after":            {255, 0, 0, 255},
		"save.restore.transform-after":  {0, 0, 0, 0},
		"clear.interior":                {0, 0, 0, 0},
		"clear.edge-inside":             {0, 0, 0, 0},
		"clear.outside":                 {255, 255, 0, 255},
		"pending.region":                {255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
	}
	if !reflect.DeepEqual(got, want) {
		for name, expected := range want {
			if !reflect.DeepEqual(got[name], expected) {
				t.Errorf("%s = %v, want %v", name, got[name], expected)
			}
		}
	}

	ops := r.CanvasOps(int(canvas.ID))
	frameOps := make([]frame.CanvasOp, len(ops))
	for i, op := range ops {
		frameOps[i] = frame.CanvasOp{Method: op.Method, Args: op.Args}
	}
	bitmap := frame.NewBitmap(96, 96)
	frame.ReplayCanvasOps(bitmap, frameOps, 96, 96)
	for y := 70; y < 72; y++ {
		for x := 70; x < 72; x++ {
			if pixel := bitmap.Pix(x, y); !reflect.DeepEqual(pixel, []byte{255, 255, 255, 255}) {
				t.Errorf("frame pending pixel (%d,%d) = %v, want white", x, y, pixel)
			}
		}
	}
}

func TestCanvasFrameReplayMatchesChromiumPixels(t *testing.T) {
	r, canvas := safeCanvasRuntime(t)
	gotValue, err := r.Evaluate(`(function(){
		var ctx = document.querySelector('canvas').getContext('2d');
		var out = {};
		var pixel = function(name, x, y) { out[name] = Array.from(ctx.getImageData(x, y, 1, 1).data); };
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(2, 3, 8, 9);
		ctx.strokeStyle = '#00ff00';
		ctx.lineWidth = 2;
		ctx.strokeRect(20, 3, 12, 10);
		pixel('fill', 5, 6);
		pixel('stroke', 25, 3);
		ctx.fillStyle = 'rgba(255, 0, 0, 0.5)';
		ctx.fillRect(0, 20, 4, 4);
		ctx.globalAlpha = 0.5;
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(4, 20, 4, 4);
		ctx.fillStyle = '#0000ff';
		ctx.fillRect(8, 20, 4, 4);
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(8, 20, 4, 4);
		ctx.fillRect(12, 20, 4, 4);
		ctx.fillRect(12, 20, 4, 4);
		pixel('alpha', 9, 21);
		pixel('overlap', 13, 21);
		ctx.globalAlpha = 1;
		ctx.fillStyle = '#123456';
		ctx.translate(10, 30);
		ctx.fillRect(1, 2, 5, 4);
		ctx.scale(2, 1);
		ctx.fillRect(10, 2, 3, 4);
		pixel('translate', 12, 33);
		pixel('scale', 31, 33);
		ctx.setTransform(1, 0, 0, 1, 0, 0);
		ctx.fillStyle = '#ff8000';
		ctx.beginPath();
		ctx.moveTo(42, 30);
		ctx.lineTo(52, 30);
		ctx.lineTo(42, 40);
		ctx.closePath();
		ctx.fill();
		pixel('triangle', 44, 32);
		ctx.strokeStyle = '#0080ff';
		ctx.lineWidth = 2;
		ctx.beginPath();
		ctx.moveTo(60, 35);
		ctx.lineTo(72, 35);
		ctx.stroke();
		pixel('line', 66, 35);
		ctx.fillStyle = '#00ff80';
		ctx.beginPath();
		ctx.arc(50, 50, 10, 0, Math.PI * 2);
		ctx.fill();
		pixel('arc', 58, 50);
		ctx.save();
		ctx.beginPath();
		ctx.rect(60, 60, 20, 20);
		ctx.clip();
		ctx.fillStyle = '#ff00ff';
		ctx.fillRect(40, 40, 60, 60);
		pixel('clip-in', 65, 65);
		pixel('clip-out', 59, 65);
		ctx.restore();
		ctx.fillStyle = '#ff0000';
		ctx.save();
		ctx.fillStyle = '#00ff00';
		ctx.globalAlpha = 0.5;
		ctx.translate(80, 0);
		ctx.fillRect(0, 20, 4, 4);
		ctx.restore();
		ctx.fillRect(80, 24, 4, 4);
		pixel('save', 81, 21);
		pixel('restore', 81, 25);
		ctx.fillStyle = '#ffff00';
		ctx.fillRect(2, 70, 20, 20);
		ctx.clearRect(5, 73, 8, 8);
		pixel('clear', 6, 74);
		pixel('kept', 4, 74);
		return JSON.stringify(out);
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	var got map[string][]int
	if err := json.Unmarshal([]byte(gotValue), &got); err != nil {
		t.Fatalf("decode pixels: %v", err)
	}
	want := map[string][]int{
		"fill": {255, 0, 0, 255}, "stroke": {0, 255, 0, 255},
		"alpha": {170, 0, 85, 192}, "overlap": {255, 0, 0, 192},
		"translate": {18, 52, 86, 255}, "scale": {18, 52, 86, 255},
		"triangle": {255, 128, 0, 255}, "line": {0, 128, 255, 255},
		"arc": {0, 255, 128, 255}, "clip-in": {255, 0, 255, 255},
		"clip-out": {0, 0, 0, 0}, "save": {0, 255, 0, 128},
		"restore": {255, 0, 0, 255}, "clear": {0, 0, 0, 0}, "kept": {255, 255, 0, 255},
	}
	points := map[string][2]int{
		"fill": {5, 6}, "stroke": {25, 3}, "alpha": {9, 21}, "overlap": {13, 21},
		"translate": {12, 33}, "scale": {31, 33}, "triangle": {44, 32}, "line": {66, 35},
		"arc": {58, 50}, "clip-in": {65, 65}, "clip-out": {59, 65}, "save": {81, 21},
		"restore": {81, 25}, "clear": {6, 74}, "kept": {4, 74},
	}
	if !reflect.DeepEqual(got, want) {
		for name, expected := range want {
			if !reflect.DeepEqual(got[name], expected) {
				t.Errorf("readback %s = %v, want %v", name, got[name], expected)
			}
		}
	}
	ops := r.CanvasOps(int(canvas.ID))
	frameOps := make([]frame.CanvasOp, len(ops))
	for i, op := range ops {
		frameOps[i] = frame.CanvasOp{Method: op.Method, Args: op.Args}
	}
	bitmap := frame.NewBitmap(96, 96)
	frame.ReplayCanvasOps(bitmap, frameOps, 96, 96)
	for name, point := range points {
		pixel := bitmap.Pix(point[0], point[1])
		gotPixel := []int{int(pixel[0]), int(pixel[1]), int(pixel[2]), int(pixel[3])}
		if gotPixel[3] > 0 && gotPixel[3] < 255 {
			for channel := 0; channel < 3; channel++ {
				gotPixel[channel] = (gotPixel[channel]*255 + gotPixel[3]/2) / gotPixel[3]
			}
		}
		if !reflect.DeepEqual(gotPixel, want[name]) {
			t.Errorf("frame %s = %v, want %v", name, gotPixel, want[name])
		}
	}
}

func TestOffscreenCanvasSafeOperationsMatchReadback(t *testing.T) {
	r, _ := safeCanvasRuntime(t)
	got, err := r.Evaluate(`(function(){
		var canvas = new OffscreenCanvas(32, 24);
		var ctx = canvas.getContext('2d');
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(2, 3, 8, 9);
		ctx.globalAlpha = 0.5;
		ctx.fillStyle = '#0000ff';
		ctx.fillRect(4, 3, 8, 9);
		ctx.save();
		ctx.translate(20, 4);
		ctx.fillStyle = '#00ff00';
		ctx.fillRect(0, 0, 4, 4);
		ctx.restore();
		var sample = function(x, y) {
			return Array.from(ctx.getImageData(x, y, 1, 1).data);
		};
		return JSON.stringify([sample(5, 6), sample(21, 5), sample(5, 20)]);
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	var pixels [][]int
	if err := json.Unmarshal([]byte(got), &pixels); err != nil {
		t.Fatalf("decode pixels: %v", err)
	}
	want := [][]int{{127, 0, 128, 255}, {0, 255, 0, 128}, {0, 0, 0, 0}}
	if !reflect.DeepEqual(pixels, want) {
		t.Fatalf("OffscreenCanvas pixels = %v, want %v", pixels, want)
	}
}

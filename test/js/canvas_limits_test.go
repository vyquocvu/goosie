package js_test

import (
	"testing"
)

func TestCanvasRejectsNonFiniteAndOutOfRangeCoordinates(t *testing.T) {
	r, canvas := safeCanvasRuntime(t)
	before := len(r.CanvasOps(int(canvas.ID)))
	if err := r.Run(`
		var ctx = document.querySelector('canvas').getContext('2d');
		ctx.fillRect(NaN, 0, 1, 1);
		ctx.strokeRect(0, Infinity, 1, 1);
		ctx.clearRect(0, 0, -Infinity, 1);
		ctx.beginPath();
		ctx.moveTo(1048577, 0);
		ctx.lineTo(0, NaN);
		ctx.translate(Infinity, 0);
		ctx.scale(0, NaN);
		ctx.transform(1, 0, 0, 1, 1048577, 0);
	`, "coordinates.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	after := len(r.CanvasOps(int(canvas.ID)))
	if after != before+1 {
		t.Fatalf("coordinate ops = %d, want only beginPath (%d)", after, before+1)
	}
}

func TestCanvasBoundsOperationCountAndStateStack(t *testing.T) {
	r, canvas := safeCanvasRuntime(t)
	if err := r.Run(`var ctx = document.querySelector('canvas').getContext('2d');`, "setup.js"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := r.Run(`
		var opError = "";
		for (var i = 0; i < 100001; i++) {
			try { ctx.fillRect(0, 0, 1, 1); }
			catch (e) { opError = e.name; break; }
		}
		var stackError = "";
		for (var i = 0; i < 65; i++) {
			try { ctx.save(); }
			catch (e) { stackError = e.name; break; }
		}
	`, "limits.js"); err != nil {
		t.Fatalf("limits: %v", err)
	}
	for _, name := range []string{"opError", "stackError"} {
		value, ok := r.Global(name)
		if !ok || value.Str != "TypeError" {
			t.Errorf("%s = %v, want TypeError", name, value)
		}
	}
	if ops := len(r.CanvasOps(int(canvas.ID))); ops > 100000 {
		t.Fatalf("recorded %d ops, want at most 100000", ops)
	}
}

func TestOffscreenCanvasUsesTheSameStateAndOperationBounds(t *testing.T) {
	r, _ := safeCanvasRuntime(t)
	if err := r.Run(`
		var offscreen = new OffscreenCanvas(16, 16);
		var ctx = offscreen.getContext('2d');
		var stackError = "";
		for (var i = 0; i < 65; i++) {
			try { ctx.save(); }
			catch (e) { stackError = e.name; break; }
		}
		var textError = "";
		try { ctx.fillText('x', 0, 0); } catch (e) { textError = e.name; }
	`, "offscreen.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range []string{"stackError", "textError"} {
		value, ok := r.Global(name)
		if !ok || value.Str != "TypeError" && value.Str != "NotSupportedError" {
			t.Errorf("%s = %v, want a bounded or unsupported error", name, value)
		}
	}
}

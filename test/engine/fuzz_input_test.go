package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// ---------------------------------------------------------------------------
// Fuzz tests for the input path.
//
// These exercise ClickAt, SetHover, FocusControl, SelectAt, SelectTo, Edit,
// FireScrollEvent, and FireResizeEvent with arbitrary inputs to catch panics,
// crashes, and deadlocks. The goal is not correctness of results but stability
// of the engine under adversarial input.
// ---------------------------------------------------------------------------

const fuzzHTML = `<!DOCTYPE html>
<html><body>
<form id="f">
<input type="text" id="t" value="hello">
<input type="checkbox" id="c">
<input type="radio" name="r" id="r1">
<input type="radio" name="r" id="r2">
<button type="submit" id="s">Go</button>
<textarea id="ta">world</textarea>
</form>
<a href="https://example.com" id="link">Link</a>
<div id="box" style="width:100px;height:50px;">Box</div>
</body></html>`

func newFuzzSession(t *testing.T) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(fuzzHTML, nil, 800,
		engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

func FuzzClickAt(f *testing.F) {
	f.Add(float32(0), float32(0))
	f.Add(float32(400), float32(300))
	f.Add(float32(-1), float32(-1))
	f.Add(float32(math.MaxFloat32), float32(math.MaxFloat32))
	f.Add(float32(math.SmallestNonzeroFloat32), float32(math.SmallestNonzeroFloat32))

	f.Fuzz(func(t *testing.T, x, y float32) {
		sess := newFuzzSession(t)
		// Must not panic or deadlock.
		sess.ClickAt(x, y)
	})
}

func FuzzSetHover(f *testing.F) {
	f.Add(float32(0), float32(0))
	f.Add(float32(400), float32(300))
	f.Add(float32(-1), float32(-1))
	f.Add(float32(math.MaxFloat32), float32(math.MaxFloat32))

	f.Fuzz(func(t *testing.T, x, y float32) {
		sess := newFuzzSession(t)
		sess.SetHover(x, y)
	})
}

func FuzzFocusControl(f *testing.F) {
	f.Add(float32(0), float32(0))
	f.Add(float32(400), float32(300))
	f.Add(float32(-1), float32(-1))

	f.Fuzz(func(t *testing.T, x, y float32) {
		sess := newFuzzSession(t)
		sess.FocusControl(x, y)
	})
}

func FuzzSelectAtTo(f *testing.F) {
	f.Add(float32(0), float32(0), float32(100), float32(50))
	f.Add(float32(-1), float32(-1), float32(800), float32(600))
	f.Add(float32(math.MaxFloat32), float32(math.MaxFloat32), float32(0), float32(0))

	f.Fuzz(func(t *testing.T, x1, y1, x2, y2 float32) {
		sess := newFuzzSession(t)
		sess.SelectAt(x1, y1)
		sess.SelectTo(x2, y2)
	})
}

func FuzzEdit(f *testing.F) {
	f.Add(byte(engine.EditRune), 'a')
	f.Add(byte(engine.EditBackspace), rune(0))
	f.Add(byte(engine.EditEnter), rune(0))
	f.Add(byte(engine.EditEscape), rune(0))
	f.Add(byte(engine.EditLeft), rune(0))
	f.Add(byte(engine.EditRight), rune(0))
	f.Add(byte(engine.EditHome), rune(0))
	f.Add(byte(engine.EditEnd), rune(0))
	f.Add(byte(255), rune(0x110000))

	f.Fuzz(func(t *testing.T, actionByte byte, r rune) {
		sess := newFuzzSession(t)
		// Focus a text input first so Edit has something to work on.
		sess.FocusControl(10, 10)
		action := engine.EditAction(actionByte)
		sess.Edit(action, r)
	})
}

func FuzzInteractionSequence(f *testing.F) {
	f.Add(uint8(0), float32(10), float32(10), float32(50), float32(50))
	f.Add(uint8(1), float32(400), float32(300), float32(0), float32(0))
	f.Add(uint8(2), float32(-1), float32(-1), float32(math.MaxFloat32), float32(0))

	f.Fuzz(func(t *testing.T, seq uint8, x1, y1, x2, y2 float32) {
		sess := newFuzzSession(t)
		// Exercise a sequence of interactions. The order depends on seq.
		switch seq % 6 {
		case 0:
			sess.ClickAt(x1, y1)
			sess.SetHover(x2, y2)
		case 1:
			sess.FocusControl(x1, y1)
			sess.Edit(engine.EditRune, 'z')
		case 2:
			sess.SelectAt(x1, y1)
			sess.SelectTo(x2, y2)
			sess.ClickAt(x1, y1)
		case 3:
			sess.SetHover(x1, y1)
			sess.ClickAt(x1, y1)
			sess.FireScrollEvent()
		case 4:
			sess.FireScrollEvent()
			sess.FireResizeEvent()
			sess.SetHover(x1, y1)
		case 5:
			sess.FocusControl(x1, y1)
			sess.Edit(engine.EditBackspace, 0)
			sess.Edit(engine.EditRune, 'x')
			sess.ClickAt(x2, y2)
		}
	})
}

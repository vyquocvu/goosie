package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// ---------------------------------------------------------------------------
// Fuzz tests for the input path.
//
// These exercise input operations with arbitrary values and check observable
// invariants as well as stability under adversarial input.
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
		before := sess.Focused()
		handled := sess.ClickAt(x, y)
		after := sess.Focused()
		if !handled && after != before {
			t.Fatal("ClickAt returned false but changed focused control")
		}
		button := sess.Doc.ElementByID("s")
		if button == nil {
			t.Fatal("fuzz fixture submit button not found")
		}
		clicked := 0
		dom.AddEventListener(button, "click", func(*dom.Event) { clicked++ }, false)
		cx, cy, ok := centerForNode(sess, button)
		if !ok {
			t.Fatal("fuzz fixture submit button has no layout box")
		}
		handled = sess.ClickAt(cx, cy)
		if !handled || clicked != 1 {
			t.Fatalf("ClickAt at the fixture submit button returned %t after %d click events, want true and 1", handled, clicked)
		}
	})
}

func FuzzSetHover(f *testing.F) {
	f.Add(float32(0), float32(0))
	f.Add(float32(400), float32(300))
	f.Add(float32(-1), float32(-1))
	f.Add(float32(math.MaxFloat32), float32(math.MaxFloat32))

	f.Fuzz(func(t *testing.T, x, y float32) {
		sess := newFuzzSession(t)
		var enters, leaves int
		var listen func(*dom.Node)
		listen = func(n *dom.Node) {
			if n == nil {
				return
			}
			if n.Element() {
				dom.AddEventListener(n, "mouseenter", func(*dom.Event) { enters++ }, false)
				dom.AddEventListener(n, "mouseleave", func(*dom.Event) { leaves++ }, false)
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				listen(child)
			}
		}
		listen(&sess.Doc.Node)
		target := sess.Doc.ElementByID("box")
		if target == nil {
			t.Fatal("fuzz fixture hover target not found")
		}
		tx, ty, ok := centerForNode(sess, target)
		if !ok {
			t.Fatal("fuzz fixture hover target has no layout box")
		}
		sess.SetHover(float32(math.MaxFloat32), float32(math.MaxFloat32))
		enters, leaves = 0, 0
		if !sess.SetHover(tx, ty) || enters+leaves != 1 {
			t.Fatalf("SetHover on fixture target returned without exactly one enter/leave event (enters=%d leaves=%d)", enters, leaves)
		}
		enters, leaves = 0, 0
		if sess.SetHover(tx, ty) {
			t.Fatal("SetHover at the unchanged fixture target returned true")
		}
		if enters+leaves != 0 {
			t.Fatalf("SetHover at the unchanged fixture target fired %d events", enters+leaves)
		}
		enters, leaves = 0, 0
		handled := sess.SetHover(x, y)
		events := enters + leaves
		if enters > 1 || leaves > 1 {
			t.Fatalf("SetHover entered %d and left %d nodes, want at most one of each (goosie fires mouseenter/mouseleave on the hit node only)", enters, leaves)
		}
		if handled != (events > 0) {
			t.Fatalf("SetHover returned %t with %d mouseenter/mouseleave events, want handled to agree with an event having fired", handled, events)
		}
		enters, leaves = 0, 0
		if sess.SetHover(x, y) {
			t.Fatal("SetHover at the same point returned true")
		}
		if enters+leaves != 0 {
			t.Fatalf("repeated SetHover fired %d mouseenter/mouseleave events", enters+leaves)
		}
	})
}

func FuzzFocusControl(f *testing.F) {
	f.Add(float32(0), float32(0))
	f.Add(float32(400), float32(300))
	f.Add(float32(-1), float32(-1))

	f.Fuzz(func(t *testing.T, x, y float32) {
		sess := newFuzzSession(t)
		input := sess.Doc.ElementByID("t")
		if input == nil {
			t.Fatal("fuzz fixture text input not found")
		}
		cx, cy, ok := centerForNode(sess, input)
		if !ok {
			t.Fatal("fuzz fixture text input has no layout box")
		}
		if !sess.FocusControl(cx, cy) || sess.Focused() != input {
			t.Fatal("FocusControl did not report and establish focus on fixture input")
		}
		if sess.FocusControl(cx, cy) || sess.Focused() != input {
			t.Fatal("FocusControl at unchanged fixture input did not report stable focus")
		}
		before := sess.Focused()
		changed := sess.FocusControl(x, y)
		after := sess.Focused()
		if changed && after == before {
			t.Fatal("FocusControl returned true but focused control did not change")
		}
		if !changed && after != before {
			t.Fatal("FocusControl returned false but changed focused control")
		}
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
		before := sess.FocusedText()
		changed := sess.Edit(action, r)
		after := sess.FocusedText()
		if action != engine.EditRune || !sess.FocusedEditable() {
			return
		}
		// focus.go:360 refuses every C0 control: no keyboard path delivers one to a
		// form field, and CR/LF/TAB must stay out of a single-line value. Anything
		// else appends at the caret, which sits at the end after focusing, and
		// string(r) already folds invalid scalars and lone surrogates to U+FFFD.
		if r < 0x20 {
			if changed || after != before {
				t.Fatalf("Edit(EditRune, %q) returned %t and left %q -> %q, want refused and unchanged", r, changed, before, after)
			}
			return
		}
		if !changed {
			t.Fatalf("Edit(EditRune, %q) on a focused editable returned false; text stayed %q", r, before)
		}
		if after != before+string(r) {
			t.Fatalf("Edit(EditRune, %q) produced %q from %q, want %q", r, after, before, before+string(r))
		}
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
		list, err := sess.PaintChecked(1)
		if err != nil {
			t.Fatalf("PaintChecked after interaction sequence: %v", err)
		}
		if list == nil || list.Len() == 0 {
			t.Fatalf("PaintChecked after interaction sequence returned %v display commands", list)
		}
	})
}

func centerForNode(sess *engine.Session, node *dom.Node) (float32, float32, bool) {
	for _, obj := range sess.Arena.Objects {
		if obj.Node == node {
			x0, y0, x1, y1 := obj.BorderRect()
			return (x0 + x1) / 2, (y0 + y1) / 2, x0 < x1 && y0 < y1
		}
	}
	return 0, 0, false
}

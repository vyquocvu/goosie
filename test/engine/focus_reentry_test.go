package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// withinDeadline runs fn on another goroutine and fails the test when it does
// not finish in time. A page listener that reenters the engine while opMu is
// held self-deadlocks; the deadline turns that hang into a failure instead of
// stalling the whole suite.
func withinDeadline(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("deadlock: a page event listener reentered the engine while opMu was held")
	}
}

const reentryDeadline = 10 * time.Second

// cleanupReentry closes a session and runtime without letting a deadlocked
// opMu hang test cleanup: when a listener self-deadlocked, the session lock is
// never released and Close would block forever.
func cleanupReentry(s *engine.Session, rt *js.Runtime) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if s != nil {
			s.Close()
		}
		rt.Close()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func newReentrySession(t *testing.T, html string) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://test.example/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	s, err := engine.NewSession(html, nil, 800,
		engine.WithViewportH(600), engine.WithJS(rt))
	if err != nil {
		rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { cleanupReentry(s, rt) })
	return s, rt
}

// TestFocusListenerComputedStyleDuringParse reproduces the WPT suite hang: an
// inline parser script focuses a control whose focus listener reads
// getComputedStyle. The focus event must not be dispatched under opMu.
func TestFocusListenerComputedStyleDuringParse(t *testing.T) {
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://test.example/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}

	html := `<html><body><input id="field" value="ab">` +
		`<script>` +
		`var field = document.getElementById('field');` +
		`field.addEventListener('focus', function () {` +
		`  window.__focusColor = getComputedStyle(field).color;` +
		`});` +
		`field.focus();` +
		`</script></body></html>`

	var sess *engine.Session
	var sessErr error
	withinDeadline(t, reentryDeadline, func() {
		sess, sessErr = engine.NewSession(html, nil, 800,
			engine.WithViewportH(600), engine.WithJS(rt))
	})
	t.Cleanup(func() { cleanupReentry(sess, rt) })
	if sessErr != nil {
		t.Fatalf("NewSession: %v", sessErr)
	}

	field := sess.Doc.ElementByID("field")
	if sess.Focused() != field {
		t.Fatal("parser-script focus() should leave the input focused")
	}
	got, ok := rt.Global("__focusColor")
	if !ok || got.Str == "" {
		t.Fatalf("focus listener should have read a computed color, got %+v", got)
	}
}

// TestBlurListenerComputedStyleOnFocusChange checks the same hazard on the
// blur side of a focus change between two controls.
func TestBlurListenerComputedStyleOnFocusChange(t *testing.T) {
	html := `<html><body><input id="a" value="ab"><input id="b" value="cd">` +
		`<script>` +
		`document.getElementById('a').addEventListener('blur', function () {` +
		`  window.__blurColor = getComputedStyle(document.getElementById('a')).color;` +
		`});` +
		`</script></body></html>`
	s, rt := newReentrySession(t, html)
	a := s.Doc.ElementByID("a")
	b := s.Doc.ElementByID("b")
	if !s.FocusNode(a) {
		t.Fatal("input a should receive focus")
	}
	withinDeadline(t, reentryDeadline, func() { s.FocusNode(b) })
	if s.Focused() != b {
		t.Fatal("input b should hold focus after the change")
	}
	got, ok := rt.Global("__blurColor")
	if !ok || got.Str == "" {
		t.Fatalf("blur listener should have read a computed color, got %+v", got)
	}
}

// TestBlurListenerRefocusWins checks that a blur listener may move focus
// itself without deadlocking, and that its decision wins over the original
// focus change.
func TestBlurListenerRefocusWins(t *testing.T) {
	html := `<html><body><input id="a" value="ab"><input id="b" value="cd"><input id="c" value="ef">` +
		`<script>` +
		`document.getElementById('a').addEventListener('blur', function () {` +
		`  document.getElementById('c').focus();` +
		`});` +
		`</script></body></html>`
	s, _ := newReentrySession(t, html)
	a := s.Doc.ElementByID("a")
	b := s.Doc.ElementByID("b")
	c := s.Doc.ElementByID("c")
	if !s.FocusNode(a) {
		t.Fatal("input a should receive focus")
	}
	withinDeadline(t, reentryDeadline, func() { s.FocusNode(b) })
	if s.Focused() != c {
		t.Fatalf("the blur listener's focus decision should win, focused=%v", s.Focused())
	}
}

// TestSubmitListenerComputedStyle checks the submit event dispatch: the
// listener reads CSSOM, and preventDefault must still block the submission.
func TestSubmitListenerComputedStyle(t *testing.T) {
	html := `<html><body><form id="f" action="/go"><input name="q" value="x">` +
		`<script>` +
		`document.getElementById('f').addEventListener('submit', function (e) {` +
		`  window.__submitColor = getComputedStyle(document.getElementById('f')).color;` +
		`  e.preventDefault();` +
		`});` +
		`</script></form></body></html>`
	s, rt := newReentrySession(t, html)
	form := s.Doc.ElementByID("f")
	withinDeadline(t, reentryDeadline, func() { s.SubmitForm(form, nil) })
	if _, ok := s.PopSubmittedForm(); ok {
		t.Fatal("preventDefault in the submit listener must block the submission")
	}
	got, ok := rt.Global("__submitColor")
	if !ok || got.Str == "" {
		t.Fatalf("submit listener should have read a computed color, got %+v", got)
	}
}

// TestInvalidListenerComputedStyle checks the blocked-submission path: the
// invalid event listener reads CSSOM and the first invalid control gets focus.
func TestInvalidListenerComputedStyle(t *testing.T) {
	html := `<html><body><form id="f" action="/go"><input id="q" name="q" required>` +
		`<script>` +
		`document.getElementById('q').addEventListener('invalid', function () {` +
		`  window.__invalidColor = getComputedStyle(document.getElementById('q')).color;` +
		`});` +
		`</script></form></body></html>`
	s, rt := newReentrySession(t, html)
	form := s.Doc.ElementByID("f")
	q := s.Doc.ElementByID("q")
	withinDeadline(t, reentryDeadline, func() { s.SubmitForm(form, nil) })
	if _, ok := s.PopSubmittedForm(); ok {
		t.Fatal("a form with an empty required control must not submit")
	}
	if s.Focused() != q {
		t.Fatal("the first invalid control should receive focus")
	}
	got, ok := rt.Global("__invalidColor")
	if !ok || got.Str == "" {
		t.Fatalf("invalid listener should have read a computed color, got %+v", got)
	}
}

// TestDialogCancelListenerComputedStyle checks the dialog cancel gate: the
// listener reads CSSOM, and preventDefault must keep the dialog open.
func TestDialogCancelListenerComputedStyle(t *testing.T) {
	html := `<html><body><button id="opener">open</button>` +
		`<dialog id="d"><input id="di" value="x"></dialog>` +
		`<script>` +
		`var d = document.getElementById('d');` +
		`document.getElementById('opener').focus();` +
		`d.showModal();` +
		`d.addEventListener('cancel', function (e) {` +
		`  window.__cancelColor = getComputedStyle(d).color;` +
		`  e.preventDefault();` +
		`});` +
		`</script></body></html>`
	s, rt := newReentrySession(t, html)
	d := s.Doc.ElementByID("d")
	withinDeadline(t, reentryDeadline, func() { s.CloseTopmostModalDialog() })
	if !d.HasAttribute("open") {
		t.Fatal("preventDefault in the cancel listener must keep the dialog open")
	}
	got, ok := rt.Global("__cancelColor")
	if !ok || got.Str == "" {
		t.Fatalf("cancel listener should have read a computed color, got %+v", got)
	}
}

// TestEditEnterSubmitListenerComputedStyle checks implicit submission from the
// keyboard path: Enter in a text input fires submit with the listener reading
// CSSOM.
func TestEditEnterSubmitListenerComputedStyle(t *testing.T) {
	html := `<html><body><form id="f" action="/go"><input id="q" name="q" value="x">` +
		`<script>` +
		`document.getElementById('f').addEventListener('submit', function (e) {` +
		`  window.__enterColor = getComputedStyle(document.getElementById('f')).color;` +
		`  e.preventDefault();` +
		`});` +
		`</script></form></body></html>`
	s, rt := newReentrySession(t, html)
	q := s.Doc.ElementByID("q")
	if !s.FocusNode(q) {
		t.Fatal("input should receive focus")
	}
	withinDeadline(t, reentryDeadline, func() { s.Edit(engine.EditEnter, 0) })
	if _, ok := s.PopSubmittedForm(); ok {
		t.Fatal("preventDefault in the submit listener must block the submission")
	}
	got, ok := rt.Global("__enterColor")
	if !ok || got.Str == "" {
		t.Fatalf("submit listener should have read a computed color, got %+v", got)
	}
}

// TestEditInputListenerComputedStyle checks the locked edit tail: a number
// input is edited through the legacy insert path, whose input event must not
// be dispatched under opMu.
func TestEditInputListenerComputedStyle(t *testing.T) {
	html := `<html><body><input id="n" type="number" value="1">` +
		`<script>` +
		`document.getElementById('n').addEventListener('input', function () {` +
		`  window.__inputColor = getComputedStyle(document.getElementById('n')).color;` +
		`});` +
		`</script></body></html>`
	s, rt := newReentrySession(t, html)
	n := s.Doc.ElementByID("n")
	if !s.FocusNode(n) {
		t.Fatal("number input should receive focus")
	}
	withinDeadline(t, reentryDeadline, func() { s.Edit(engine.EditRune, '5') })
	got, ok := rt.Global("__inputColor")
	if !ok || got.Str == "" {
		t.Fatalf("input listener should have read a computed color, got %+v", got)
	}
}

// TestEscapeBlurListenerComputedStyle checks the Escape dismissal blur.
func TestEscapeBlurListenerComputedStyle(t *testing.T) {
	html := `<html><body><input id="a" value="ab">` +
		`<script>` +
		`document.getElementById('a').addEventListener('blur', function () {` +
		`  window.__escColor = getComputedStyle(document.getElementById('a')).color;` +
		`});` +
		`</script></body></html>`
	s, rt := newReentrySession(t, html)
	a := s.Doc.ElementByID("a")
	if !s.FocusNode(a) {
		t.Fatal("input should receive focus")
	}
	withinDeadline(t, reentryDeadline, func() { s.Edit(engine.EditEscape, 0) })
	if s.Focused() != nil {
		t.Fatal("Escape should blur the focused control")
	}
	got, ok := rt.Global("__escColor")
	if !ok || got.Str == "" {
		t.Fatalf("blur listener should have read a computed color, got %+v", got)
	}
}

// TestMoveSelectListenerComputedStyle checks the select keyboard path: input
// and change fire with the listener reading CSSOM.
func TestMoveSelectListenerComputedStyle(t *testing.T) {
	html := `<html><body><select id="s"><option value="1">one</option><option value="2">two</option></select>` +
		`<script>` +
		`document.getElementById('s').addEventListener('change', function () {` +
		`  window.__selectColor = getComputedStyle(document.getElementById('s')).color;` +
		`});` +
		`</script></body></html>`
	s, rt := newReentrySession(t, html)
	sel := s.Doc.ElementByID("s")
	if !s.FocusNode(sel) {
		t.Fatal("select should receive focus")
	}
	withinDeadline(t, reentryDeadline, func() { s.MoveSelect(engine.EditDown) })
	got, ok := rt.Global("__selectColor")
	if !ok || got.Str == "" {
		t.Fatalf("change listener should have read a computed color, got %+v", got)
	}
}

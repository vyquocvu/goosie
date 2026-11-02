package engine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Real parity comparison: Goosie vs Chromium reference.
//
// Each test loads the same HTML fixture used by the Playwright reference
// script (scripts/interaction-parity-reference.js), performs the same
// interaction sequence, and compares the captured event log against the
// Chromium reference committed at testdata/interaction-parity/.
//
// Regenerate the reference with: node scripts/interaction-parity-reference.js
// Mismatches are reported with full diff, not silently accepted.
// ---------------------------------------------------------------------------

var parityFixtureDir = filepath.Join("..", "..", "testdata", "interaction-parity")
var parityReferencePath = filepath.Join(parityFixtureDir, "reference-events.json")
var parityStatePath = filepath.Join(parityFixtureDir, "reference-state.json")

// parityStateExpr is evaluated on both sides: the Playwright reference script
// runs it after each interaction sequence and the Go tests run it through the
// document runtime. It records focus, checked controls, and the URL fragment.
const parityStateExpr = `(function(){
  var a = document.activeElement;
  var ids = ['cb1','cb2','r1','r2','r3'];
  var checked = [];
  for (var i = 0; i < ids.length; i++) {
    var e = document.getElementById(ids[i]);
    if (e && e.checked) checked.push(ids[i]);
  }
  var active = a ? (a.id ? a.id : a.tagName.toLowerCase()) : 'null';
  return 'active=' + active + ';checked=' + checked.join('+') + ';hash=' + location.hash;
})()`

// captureParity writes Goosie's own event log for a fixture into
// goosie-events.json when GOOSIE_CAPTURE_PARITY=1, so both sides of the
// comparison are persisted as reproducible artifacts.
func captureParity(fixture, got string) {
	if os.Getenv("GOOSIE_CAPTURE_PARITY") != "1" {
		return
	}
	path := filepath.Join(parityFixtureDir, "goosie-events.json")
	logs := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &logs)
	}
	logs[fixture] = got
	data, _ := json.MarshalIndent(logs, "", "  ")
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}

func loadParityReference(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(parityReferencePath)
	if err != nil {
		t.Fatalf("Cannot read reference file %s: %v\nRun: node scripts/interaction-parity-reference.js", parityReferencePath, err)
	}
	var ref map[string]string
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("Cannot parse reference: %v", err)
	}
	return ref
}

func loadParityState(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(parityStatePath)
	if err != nil {
		t.Fatalf("Cannot read reference state %s: %v\nRun: node scripts/interaction-parity-reference.js", parityStatePath, err)
	}
	var states map[string]string
	if err := json.Unmarshal(data, &states); err != nil {
		t.Fatalf("Cannot parse reference state: %v", err)
	}
	return states
}

func getParityState(t *testing.T, rt *js.Runtime) string {
	t.Helper()
	state, err := rt.Evaluate(parityStateExpr)
	if err != nil {
		t.Fatalf("evaluate parity state: %v", err)
	}
	return state
}

// clickCenterWithFocus mirrors the host pointer sequence: the press focuses the
// element under the cursor, then the click runs its default action. Chromium
// focuses buttons and links on mousedown, so the final-state comparison has to
// drive both steps to stay comparable.
func clickCenterWithFocus(t *testing.T, s *engine.Session, id string) bool {
	t.Helper()
	if s.Arena == nil {
		return false
	}
	for _, obj := range s.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			x, y := (x0+x1)/2, (y0+y1)/2
			s.FocusControl(x, y)
			s.ClickAt(x, y)
			return true
		}
	}
	return false
}

func loadParityFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parityFixtureDir, name))
	if err != nil {
		t.Fatalf("Cannot read fixture %s: %v", name, err)
	}
	return string(data)
}

func paritySession(t *testing.T, html string) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://test.example/",
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { rt.Close() })

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithViewportH(600),
		engine.WithJS(rt),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, rt
}

func getParityLog(t *testing.T, rt *js.Runtime) string {
	t.Helper()
	v, ok := rt.Global("__log")
	if !ok {
		return ""
	}
	return v.Str
}

func assertParityMatch(t *testing.T, fixture string, got, want string) {
	t.Helper()
	captureParity(fixture, got)
	gotParts := strings.Split(got, ",")
	wantParts := strings.Split(want, ",")
	if got != want {
		t.Errorf("MISMATCH in %s:\n  Goosie:   %v\n  Chromium: %v", fixture, gotParts, wantParts)
	}
}

// TestParityRef_EventOrder compares capture/bubble ordering against Chromium,
// including the focused element the click leaves behind.
//
// qc:BR-178
func TestParityRef_EventOrder(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "01-event-order.html")
	sess, rt := paritySession(t, html)

	clickCenterWithFocus(t, sess, "btn")

	got := getParityLog(t, rt)
	want := ref["01-event-order"]
	assertParityMatch(t, "01-event-order", got, want)
	if gotState, wantState := getParityState(t, rt), states["01-event-order"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 01-event-order:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

// TestParityRef_StopPropagation compares stopPropagation behavior.
//
// qc:BR-178
func TestParityRef_StopPropagation(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "02-stop-propagation.html")
	sess, rt := paritySession(t, html)

	clickCenterWithFocus(t, sess, "btn")

	got := getParityLog(t, rt)
	want := ref["02-stop-propagation"]
	assertParityMatch(t, "02-stop-propagation", got, want)
	if gotState, wantState := getParityState(t, rt), states["02-stop-propagation"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 02-stop-propagation:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

// TestParityRef_MultipleListeners compares multiple listener ordering.
//
// qc:BR-178
func TestParityRef_MultipleListeners(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "03-multiple-listeners.html")
	sess, rt := paritySession(t, html)

	clickCenterWithFocus(t, sess, "btn")

	got := getParityLog(t, rt)
	want := ref["03-multiple-listeners"]
	assertParityMatch(t, "03-multiple-listeners", got, want)
	if gotState, wantState := getParityState(t, rt), states["03-multiple-listeners"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 03-multiple-listeners:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

// TestParityRef_CheckboxRadio compares checkbox/radio change events and the
// checked set the sequence leaves behind.
//
// qc:BR-178
func TestParityRef_CheckboxRadio(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "04-checkbox-radio.html")
	sess, rt := paritySession(t, html)

	clickCenterWithFocus(t, sess, "cb1")
	clickCenterWithFocus(t, sess, "cb2")
	clickCenterWithFocus(t, sess, "r1")
	clickCenterWithFocus(t, sess, "r3")

	got := getParityLog(t, rt)
	want := ref["04-checkbox-radio"]
	assertParityMatch(t, "04-checkbox-radio", got, want)
	if gotState, wantState := getParityState(t, rt), states["04-checkbox-radio"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 04-checkbox-radio:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

// TestParityRef_FocusTab compares Tab-order focus traversal against Chromium.
// The reference clicks #a then presses Tab three times (a->b->c->d); Goosie
// mirrors this by focusing #a then calling FocusNext three times. focus/blur
// fire non-bubbling at the target, matching the DOM spec and Chromium. The
// final state must be the same focused element Chromium ends on.
//
// qc:BR-178
func TestParityRef_FocusTab(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "05-focus-tab.html")
	sess, rt := paritySession(t, html)

	focusCenter(t, sess, "a")
	pressTab(t, sess)
	pressTab(t, sess)
	pressTab(t, sess)

	got := getParityLog(t, rt)
	want := ref["05-focus-tab"]
	assertParityMatch(t, "05-focus-tab", got, want)
	if gotState, wantState := getParityState(t, rt), states["05-focus-tab"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 05-focus-tab:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

// TestParityRef_PreventDefault compares preventDefault behavior. Inline <a>
// boxes now carry a union rect, so clickCenter resolves the link and its
// preventDefault listener fires, matching Chromium. The final state also shows
// that neither the form submission nor the fragment navigation took effect.
//
// qc:BR-178
func TestParityRef_PreventDefault(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "06-prevent-default.html")
	sess, rt := paritySession(t, html)

	clickCenterWithFocus(t, sess, "btn")
	clickCenterWithFocus(t, sess, "link")

	got := getParityLog(t, rt)
	want := ref["06-prevent-default"]
	assertParityMatch(t, "06-prevent-default", got, want)
	if gotState, wantState := getParityState(t, rt), states["06-prevent-default"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 06-prevent-default:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

// TestParityRef_Keyboard compares the full keyboard-event sequence against
// Chromium. The reference clicks #a (focus) then presses Tab twice. Goosie
// focuses #a then drives PressKey("Tab"), which emits keydown on the old focus,
// runs the focus-move default action (blur/focus), then keyup on the new focus.
// event.key must be observable to JS as "Tab".
// Reference: a:focus,a:keydown:Tab,a:blur,b:focus,b:keyup:Tab,b:keydown:Tab,b:blur,c:focus,c:keyup:Tab
//
// qc:BR-178
func TestParityRef_Keyboard(t *testing.T) {
	ref := loadParityReference(t)
	states := loadParityState(t)
	html := loadParityFixture(t, "07-keyboard.html")
	sess, rt := paritySession(t, html)

	focusCenter(t, sess, "a")
	pressTabKey(t, sess)
	pressTabKey(t, sess)

	got := getParityLog(t, rt)
	want := ref["07-keyboard"]
	assertParityMatch(t, "07-keyboard", got, want)
	if gotState, wantState := getParityState(t, rt), states["07-keyboard"]; gotState != wantState {
		t.Errorf("STATE MISMATCH in 07-keyboard:\n  Goosie:   %v\n  Chromium: %v", gotState, wantState)
	}
}

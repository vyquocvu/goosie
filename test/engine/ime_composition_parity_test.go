package engine_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

// BR-111: Vietnamese typed through a host IME.
//
// An IME does not type one character at a time - it marks a passage and then
// REPLACES that same passage on every keystroke: `ti`, `tie`, `tiế`, `tiếng`.
// Both failure modes the catalog names live in that replacement: append instead
// of replace and the field reads `titietiếtiếng` (a duplicated base letter);
// drop the tone mark and it reads `tieng`.
//
// Every number below is quoted from scripts/verify-br111-ime-composition.js,
// which drives Chromium 153.0.8010.12 through CDP Input.imeSetComposition
// (2026-11-05). That seam cannot commit through a native candidate window, so
// nothing here asserts the inputType or the trust of the commit event; what it
// does assert is the part both paths share and the page actually reads.
//
// Chromium, per step, on an empty <input> and on a contenteditable alike:
//
//	compositionstart  data=""                     cancelable=true
//	compositionupdate data="ti"   value=""        cancelable=true
//	beforeinput       insertCompositionText "ti"  value=""       isComposing=true cancelable=false
//	input             insertCompositionText "ti"  value="ti"     isComposing=true cancelable=false  ss=0 se=2
//	compositionupdate data="tie"  value="ti"      cancelable=true
//	beforeinput       insertCompositionText "tie" value="ti"     cancelable=false
//	input             insertCompositionText "tie" value="tie"    ss=0 se=3
//	… same shape for "tiế" (se=3) and "tiếng" (se=5)
//	then input … and compositionend data="tiếng" cancelable=true, selection collapsed
//
// Two details only a measurement gives: while the passage is marked the
// control reports it as SELECTED (ss=0, se=5) yet selectionDirection stays
// "none"; and the composition events are cancelable while their beforeinput is
// not, because a page cannot veto text it never asked for.

// imeProbe records every composition and editing event the page sees, in the
// fields the Chromium script prints, together with the field state at that
// moment - which is how a replacement is told apart from an append.
const imeProbe = `(function(id){var el=document.getElementById(id);window.EVENTS=[];window.STATE=[];
window.rec=function(e){window.EVENTS.push(e.type+':'+(e.inputType||'-')+':'+JSON.stringify(e.data===undefined?null:e.data)+':comp='+(e.isComposing===undefined?'-':e.isComposing)+':cancel='+e.cancelable);
window.STATE.push({t:e.type,value:(el.value===undefined?el.textContent:el.value),ss:(el.selectionStart===undefined?-1:el.selectionStart),se:(el.selectionEnd===undefined?-1:el.selectionEnd),dir:(el.selectionDirection===undefined?'-':el.selectionDirection)});};
['compositionstart','compositionupdate','compositionend','beforeinput','input'].forEach(function(ty){el.addEventListener(ty,window.rec);});
return 'listening'})`

func installIMEProbe(t *testing.T, sess sessionEvaluator, id string) {
	t.Helper()
	if _, err := sess.EvaluateJS(imeProbe + "('" + id + "')"); err != nil {
		t.Fatalf("install IME probe on %q: %v", id, err)
	}
}

// sessionEvaluator is the slice's view of a session with a script runtime: the
// probes below read page state, so every test here needs one expression
// evaluated at a time.
type sessionEvaluator interface {
	EvaluateJS(string) (string, error)
}

func driveMarked(t *testing.T, s interface{ SetMarked(string) bool }, steps []string) {
	t.Helper()
	for _, st := range steps {
		if !s.SetMarked(st) {
			t.Fatalf("SetMarked(%q) reported no change", st)
		}
	}
}

func evalState(t *testing.T, s sessionEvaluator, expr string) string {
	t.Helper()
	got, err := s.EvaluateJS(expr)
	if err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	return got
}

// newCompositionSession builds the session a host IME really drives: a script
// runtime plus the invalidation batch that records DOM mutations. A marked
// first step into an empty contenteditable inserts the Text node its preview
// lands in, and an unstyled node is skipped by layout - so without the batch
// there is no mutation to pump, and the test would pass over a page the app
// renders empty.
func newCompositionSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://test.example/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { rt.Close() })
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithViewportH(600),
		engine.WithJS(rt),
		engine.WithInvalidation(&engine.Invalidation{}),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

// qc:BR-111
//
// The marked passage is selected, not merely caret-terminated: Chromium gives
// ss=0 se=2 for "ti", se=3 for "tie" and "tiế", se=5 for "tiếng", with
// selectionDirection "none" all the way through, and collapses to ss=se=5 only
// at the commit. Goosie reported a collapsed range at the start of the passage
// instead, which is why a page watching the selection - and a host asking where
// to put a candidate window - saw nothing.
func TestCompositionSelectsTheMarkedRangeInATextControl(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><input id="a"></body></html>`)
	installIMEProbe(t, s, "a")
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	for _, want := range []struct {
		text string
		se   int
	}{{"ti", 2}, {"tie", 3}, {"tiế", 3}, {"tiếng", 5}} {
		if !s.SetMarked(want.text) {
			t.Fatalf("SetMarked(%q) reported no change", want.text)
		}
		got := evalState(t, s, `JSON.stringify({value:document.getElementById('a').value,ss:document.getElementById('a').selectionStart,se:document.getElementById('a').selectionEnd,dir:document.getElementById('a').selectionDirection})`)
		wantJSON := `{"value":"` + want.text + `","ss":0,"se":` + strconv.Itoa(want.se) + `,"dir":"none"}`
		if got != wantJSON {
			t.Errorf("after SetMarked(%q): state = %s, want %s (Chromium selects the marked range and keeps selectionDirection none)", want.text, got, wantJSON)
		}
	}
	if !s.CommitText("tiếng") {
		t.Fatal("CommitText reported no change")
	}
	got := evalState(t, s, `JSON.stringify({value:document.getElementById('a').value,ss:document.getElementById('a').selectionStart,se:document.getElementById('a').selectionEnd})`)
	if want := `{"value":"tiếng","ss":5,"se":5}`; got != want {
		t.Errorf("after commit: state = %s, want %s (Chromium collapses the selection past the committed text)", got, want)
	}
}

// qc:BR-111
//
// The stream a page listens to: composition events cancelable, their
// beforeinput not, each step carrying the marked text as `data`, in the order
// compositionupdate -> beforeinput -> input, with the field value moving only
// at the last one.
func TestCompositionEventStreamCarriesTheMarkedTextOnce(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><input id="a"></body></html>`)
	installIMEProbe(t, s, "a")
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	driveMarked(t, s, []string{"ti", "tie", "tiế", "tiếng"})
	got := evalState(t, s, `window.EVENTS.join(",")`)
	want := ""
	for i, st := range []string{"ti", "tie", "tiế", "tiếng"} {
		if i == 0 {
			want += `compositionstart:-:"":comp=-:cancel=true,`
		}
		want += fmt.Sprintf(`compositionupdate:-:%q:comp=-:cancel=true,`, st)
		want += fmt.Sprintf(`beforeinput:insertCompositionText:%q:comp=true:cancel=false,`, st)
		want += fmt.Sprintf(`input:insertCompositionText:%q:comp=true:cancel=false`, st)
		if i != 3 {
			want += ","
		}
	}
	if got != want {
		t.Errorf("composition event stream =\n  %s\nwant, as Chromium 153 reported it\n  %s", got, want)
	}
	// The value the page holds at each event is what separates a replacement
	// from an append: still the previous passage at beforeinput, the whole new
	// one at input, never the two concatenated.
	states := evalState(t, s, `JSON.stringify(window.STATE.map(function(x){return x.t+"="+x.value}))`)
	wantStates := `["compositionstart=","compositionupdate=","beforeinput=","input=ti",` +
		`"compositionupdate=ti","beforeinput=ti","input=tie",` +
		`"compositionupdate=tie","beforeinput=tie","input=tiế",` +
		`"compositionupdate=tiế","beforeinput=tiế","input=tiếng"]`
	if states != wantStates {
		t.Errorf("value at each event = %s, want %s", states, wantStates)
	}
}

// qc:BR-111
//
// An empty contenteditable previews every step, in the DOM and on screen.
// Chromium's first input event on a fresh <div contenteditable> carried
// value="ti" with a Selection equal to it; Goosie needed a Text node to exist
// before it could mark anything, so Vietnamese typed into an empty editor
// showed nothing at all until the commit. The node the composition inserts is
// a DOM mutation like any other, so the preview reaches the screen through the
// same pump turn that styles a script-inserted node.
func TestCompositionPreviewsLiveInAnEmptyContenteditable(t *testing.T) {
	s := newCompositionSession(t, `<html><body style="margin:0"><div id="ce" contenteditable="true"></div></body></html>`)
	installIMEProbe(t, s, "ce")
	if !s.FocusNode(s.Doc.ElementByID("ce")) {
		t.Fatal("contenteditable should take focus")
	}
	driveMarked(t, s, []string{"ti", "tie", "tiế", "tiếng"})
	if got, want := evalState(t, s, `document.getElementById('ce').textContent`), "tiếng"; got != want {
		t.Errorf("contenteditable text during composition = %q, want %q (Chromium previews the marked passage live)", got, want)
	}
	inputs := evalState(t, s, `window.STATE.filter(function(x){return x.t==='input'}).map(function(x){return x.value}).join(",")`)
	if want := "ti,tie,tiế,tiếng"; inputs != want {
		t.Errorf("input events carried value %q, want %q", inputs, want)
	}
	if !s.CanDeferControlEditReflow() {
		t.Error("the composition that inserted the preview node reported no pending DOM mutation, so the host would reflow an unstyled tree and paint nothing")
	}
	// Seeing it is the point of the row: the preview has to be laid out and
	// inked, once, while the composition is still open.
	if !s.PumpJS(800) {
		t.Fatal("the pump ignored the DOM mutation the composition made")
	}
	if got := paintedRunes(t, s); got != "tiếng" {
		t.Errorf("painted runes during composition = %q, want %q", got, "tiếng")
	}
	if !s.CommitText("tiếng") {
		t.Fatal("commit reported no change")
	}
	if got, want := evalState(t, s, `document.getElementById('ce').textContent`), "tiếng"; got != want {
		t.Errorf("after commit textContent = %q, want %q", got, want)
	}
	s.PumpJS(800)
	if got := paintedRunes(t, s); got != "tiếng" {
		t.Errorf("painted runes after commit = %q, want %q (the preview must be replaced, not left behind)", got, "tiếng")
	}
}

// qc:BR-111
//
// The preview node is an observed DOM insertion. Chromium's MutationObserver on
// a fresh <div contenteditable> reported exactly:
//
//	childList #text | characterData "ti" | characterData "tie" | characterData "tiế" | characterData "tiếng"
//
// - one insertion for the first step, then a data change per rewrite
// (scripts/verify-br111-ime-composition.js, mutation records line). An editor
// built on the DOM (a page that re-renders, counts characters, or persists the
// document) only works if the composition announces itself there too, not just
// through the events.
func TestCompositionMutatesTheDocumentAsObserved(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><div id="ce" contenteditable="true"></div></body></html>`)
	if _, err := s.EvaluateJS(`(function(){window.MO=[];new MutationObserver(function(rs){for(var k=0;k<rs.length;k++){var r=rs[k];window.MO.push(r.type+(r.addedNodes.length?':'+r.addedNodes[0].nodeName:''));}}).observe(document.getElementById('ce'),{childList:true,characterData:true,subtree:true});return 'observing'})()`); err != nil {
		t.Fatalf("install mutation observer: %v", err)
	}
	if !s.FocusNode(s.Doc.ElementByID("ce")) {
		t.Fatal("contenteditable should take focus")
	}
	driveMarked(t, s, []string{"ti", "tie", "tiế", "tiếng"})
	// The values Chromium's callback read per step were "ti", "tie", "tiế",
	// "tiếng"; a characterData record carries the node rather than a snapshot,
	// and Goosie delivers the queue on the pump turn, so what is comparable in
	// both is the shape of the queue: one insertion, then one data change per
	// rewrite.
	s.PumpJS(800)
	want := "childList:#text,characterData,characterData,characterData,characterData"
	if got := evalState(t, s, `window.MO.join(",")`); got != want {
		t.Errorf("mutation records = %q, want %q as Chromium reported them", got, want)
	}
	// A step that inserted a node instead of rewriting the marked one would
	// leave the word twice in the editor - the duplication this catalog row is
	// about, seen from the page rather than from the paint.
	if got := evalState(t, s, `window.MO.filter(function(m){return m.indexOf('childList')===0}).length`); got != "1" {
		t.Errorf("insertion records = %s, want 1 (only the first step adds the preview node)", got)
	}
	if got, want := evalState(t, s, `document.getElementById('ce').textContent`), "tiếng"; got != want {
		t.Errorf("observed editor text = %q, want %q", got, want)
	}
}

// qc:BR-111
//
// Tone marks handed over as combining characters stay that way. Chromium stored
// the decomposed "tiếng" as 7 code points and reported the selection end as 7:
// it neither normalises to the precomposed form nor counts in another space.
func TestCompositionKeepsCombiningMarksVerbatim(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><input id="a"></body></html>`)
	installIMEProbe(t, s, "a")
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	driveMarked(t, s, []string{"ti", "tie", "tie\u0302\u0301", "tie\u0302\u0301ng"})
	got := evalState(t, s, `JSON.stringify({len:document.getElementById('a').value.length,se:document.getElementById('a').selectionEnd})`)
	if want := `{"len":7,"se":7}`; got != want {
		t.Errorf("combining marks: %s, want %s (Chromium keeps 7 code units and counts offsets in them)", got, want)
	}
}

// qc:BR-111
//
// The composed word is inked once. A control's value already carries the marked
// passage, so drawing the marked text again on top of it doubles every glyph -
// the "repeated combined character" the catalog row names, in the one place a
// user can see it.
func TestCompositionInksTheMarkedTextOnce(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><input id="a"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	driveMarked(t, s, []string{"ti", "tie", "tiế", "tiếng"})
	if got, want := paintedRunes(t, s), "tiếng"; got != want {
		t.Errorf("painted runes = %q, want %q (the marked passage lives in the value, so it must not be drawn twice)", got, want)
	}
}

// paintedRunes is the ink the frame path would place: every glyph in the frozen
// display list, in the order the builder emitted it.
func paintedRunes(t *testing.T, s *engine.Session) string {
	t.Helper()
	list, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var out []rune
	for _, cmd := range list.All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		for _, g := range cmd.Text.Glyphs {
			out = append(out, g.Rune)
		}
	}
	return string(out)
}

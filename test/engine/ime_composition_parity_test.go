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
func TestCompositionPreviewsInPrefilledContenteditableWithoutSelection(t *testing.T) {
	s := newCompositionSession(t, `<html><body style="margin:0"><div id="ce" contenteditable="true">hello</div></body></html>`)
	installIMEProbe(t, s, "ce")
	if !s.FocusNode(s.Doc.ElementByID("ce")) {
		t.Fatal("contenteditable should take focus")
	}
	if _, err := s.EvaluateJS(`getSelection().removeAllRanges()`); err != nil {
		t.Fatalf("clear selection: %v", err)
	}
	if !s.SetMarked("ti") {
		t.Fatal("SetMarked reported no change")
	}
	if got, want := evalState(t, s, `document.getElementById('ce').textContent`), "helloti"; got != want {
		t.Errorf("contenteditable text during composition = %q, want %q", got, want)
	}
	if !s.CommitText("ti") {
		t.Fatal("commit reported no change")
	}
	if got, want := evalState(t, s, `document.getElementById('ce').textContent`), "helloti"; got != want {
		t.Errorf("after commit textContent = %q, want %q", got, want)
	}
}

// qc:BR-111
func TestCompositionMutatesTheDocumentAsObserved(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><div id="ce" contenteditable="true"></div></body></html>`)
	if _, err := s.EvaluateJS(`(function(){window.MO=[];new MutationObserver(function(rs){for(var k=0;k<rs.length;k++){var r=rs[k];window.MO.push(r.type+(r.addedNodes.length?':'+r.addedNodes[0].nodeName:''));}}).observe(document.getElementById('ce'),{childList:true,characterData:true,subtree:true});return 'observing'})()`); err != nil {
		t.Fatalf("install mutation observer: %v", err)
	}
	if !s.FocusNode(s.Doc.ElementByID("ce")) {
		t.Fatal("contenteditable should take focus")
	}
	driveMarked(t, s, []string{"ti", "tie", "tiế", "tiếng"})
	s.PumpJS(800)
	want := "childList:#text,characterData,characterData,characterData,characterData"
	if got := evalState(t, s, `window.MO.join(",")`); got != want {
		t.Errorf("mutation records = %q, want %q as Chromium reported them", got, want)
	}
	if got := evalState(t, s, `window.MO.filter(function(m){return m.indexOf('childList')===0}).length`); got != "1" {
		t.Errorf("insertion records = %s, want 1 (only the first step adds the preview node)", got)
	}
	if got, want := evalState(t, s, `document.getElementById('ce').textContent`), "tiếng"; got != want {
		t.Errorf("observed editor text = %q, want %q", got, want)
	}
}

// qc:BR-111
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
func TestCompositionInksTheMarkedTextOnce(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><input id="a"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	driveMarked(t, s, []string{"ti", "tie", "tiế", "tiếng"})
	if got, want := paintedRunes(t, s), "tiếng"; got != want {
		t.Errorf("painted runes = %q, want %q (the marked passage lives in the value, so it must not be drawn twice)", got, want)
	}
}

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

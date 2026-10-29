package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

// IME composition contract: input and textarea values track each active
// composition update, and a commit replaces the active passage with its final
// text. Cancel and blur remove the marked passage; maxlength applies to the
// committed value, not to intermediate candidate text.

func TestSetMarkedWithoutFocus(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.SetMarked("に") {
		t.Fatal("SetMarked without focus should not report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked = %q, want empty", s.Marked())
	}
}

func TestSetMarkedAndCommit(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a") // value "ab", caret ends at 2
	s.FocusControl(x, y)
	if !s.SetMarked("ni") {
		t.Fatal("SetMarked should report change")
	}
	if s.Marked() != "ni" {
		t.Fatalf("Marked = %q, want %q", s.Marked(), "ni")
	}
	if got := s.Focused().InputControlValue(); got != "abni" {
		t.Fatalf("value during composition = %q, want live composition value %q", got, "abni")
	}
	if !s.CommitText("日") {
		t.Fatal("CommitText should report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked after commit = %q, want empty", s.Marked())
	}
	if got := s.Focused().InputControlValue(); got != "ab日" {
		t.Fatalf("value = %q, want %q", got, "ab日")
	}
}

func TestSetMarkedUnchangedNoChange(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("ni")
	if s.SetMarked("ni") {
		t.Fatal("SetMarked with identical text should not report change")
	}
	if !s.SetMarked("") {
		t.Fatal("clearing marked should report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked = %q, want empty", s.Marked())
	}
}

func TestCommitTextRespectsMaxlength(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" value="ab" maxlength="2"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("xyz")
	if s.CommitText("xyz") {
		t.Fatal("commit past maxlength should not report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked = %q, want cleared even on declined commit", s.Marked())
	}
	if got := s.Focused().InputControlValue(); got != "ab" {
		t.Fatalf("value = %q, want %q", got, "ab")
	}
}

func TestMaxlengthCountsUTF16CodeUnits(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" maxlength="2"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.InsertText("😀x") {
		t.Fatal("insertion should accept the supplementary character within the two-unit limit")
	}
	if got := s.FocusedText(); got != "😀" {
		t.Fatalf("value = %q, want exactly one supplementary character", got)
	}
	if s.SetFocusedText("😀x") {
		t.Fatal("SetFocusedText should reject a value longer than two UTF-16 code units")
	}
	if got := s.FocusedText(); got != "😀" {
		t.Fatalf("value after rejected SetFocusedText = %q, want unchanged", got)
	}
}

func TestMaxlengthZeroPreventsUserInsertionAndCompositionCommit(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" maxlength="0"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if s.InsertText("x") {
		t.Fatal("maxlength=0 should reject user text insertion")
	}
	if !s.SetMarked("x") {
		t.Fatal("composition preview should be accepted before the committed value is constrained")
	}
	if s.CommitText("x") {
		t.Fatal("maxlength=0 should reject composition commit")
	}
	if got := s.FocusedText(); got != "" {
		t.Fatalf("value = %q, want empty", got)
	}
}

func TestMaxlengthAbovePlatformIntegerRangeRemainsUsable(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" maxlength="999999999999999999999999999999"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.InsertText("x") {
		t.Fatal("large valid maxlength should not overflow into a negative limit")
	}
	if got := s.FocusedText(); got != "x" {
		t.Fatalf("value = %q, want %q", got, "x")
	}
}

func TestMaxlengthUsesHTMLNonNegativeIntegerParsing(t *testing.T) {
	for _, tc := range []struct {
		name, attribute, want string
	}{
		{name: "leading whitespace plus and trailing text", attribute: "  +2tail", want: "ab"},
		{name: "negative number is invalid", attribute: "-1", want: "abc"},
		{name: "nonnumeric value is invalid", attribute: "abc", want: "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFocusSession(t, `<html><body style="margin: 0;"><input id="a" maxlength="`+tc.attribute+`"></body></html>`)
			x, y := clickPoint(t, s, "a")
			s.FocusControl(x, y)
			if !s.InsertText("abc") {
				t.Fatal("insertion should be accepted")
			}
			if got := s.FocusedText(); got != tc.want {
				t.Fatalf("value = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMarkedClearsOnBlurAndSwitch(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("かな")
	s.FocusControl(790, 590) // blur
	if s.Marked() != "" {
		t.Fatalf("Marked after blur = %q, want empty", s.Marked())
	}
	s.FocusControl(x, y)
	s.SetMarked("かな")
	bx, by := clickPoint(t, s, "b")
	s.FocusControl(bx, by) // switch
	if s.Marked() != "" {
		t.Fatalf("Marked after switch = %q, want empty", s.Marked())
	}
}

func TestEditEscapeClearsMarked(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("かな")
	if !s.Edit(engine.EditEscape, 0) {
		t.Fatal("escape should report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked after escape = %q, want empty", s.Marked())
	}
}

func TestPaintMarkedPreviewAtCaret(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" value="ab"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y) // caret at end
	s.Edit(engine.EditHome, 0)
	s.SetMarked("か")
	dl := s.Paint(1).Build(1)
	runes := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(runes) != "かab" {
		t.Fatalf("painted value = %q, want marked preview spliced at caret: %q", string(runes), "かab")
	}
}

func TestContentEditableIMEPreviewAtSelectionAndCommit(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body style="margin:0"><div id="edit" contenteditable="true">ab</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	if !s.FocusNode(edit) || !s.FocusedEditable() {
		t.Fatal("contenteditable host should be focused and exposed as editable")
	}
	if _, err := s.EvaluateJS(`(function(){var t=document.getElementById("edit").firstChild,r=document.createRange();r.setStart(t,1);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r)})()`); err != nil {
		t.Fatalf("position composition caret: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){window.imeEvents=[];window.imeCancelable=[];window.imeValues=[];var host=document.getElementById("edit");["compositionstart","compositionupdate","compositionend"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted);imeCancelable.push(e.type+":"+e.cancelable);imeValues.push(e.type+":"+e.data+"@"+host.textContent)})});host.addEventListener("beforeinput",function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isTrusted);imeValues.push(e.type+":"+e.inputType+"@"+host.textContent)});host.addEventListener("input",function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isTrusted);imeValues.push(e.type+":"+e.inputType+"@"+host.textContent)})})()`); err != nil {
		t.Fatalf("install composition listeners: %v", err)
	}
	if got, err := s.EvaluateJS(`getSelection().anchorOffset`); err != nil || got != "1" {
		t.Fatalf("composition anchor offset = %q, err %v", got, err)
	}
	if !s.SetMarked("か") {
		t.Fatal("contenteditable composition preview should update")
	}
	if !s.SetMarked("かな") {
		t.Fatal("contenteditable composition update should replace the preview")
	}
	if got := edit.TextContent(); got != "aかなb" {
		t.Fatalf("DOM text during composition = %q, want active passage inserted: %q", got, "aかなb")
	}
	if err := s.Reflow(800); err != nil {
		t.Fatalf("reflow active composition passage: %v", err)
	}
	dl := s.Paint(1).Build(1)
	if got := string(textRunesIn(t, dl, frame.RGB(0, 0, 0))); got != "aかなb" {
		t.Fatalf("painted composition = %q, want %q", got, "aかなb")
	}
	if !s.CommitText("仮") {
		t.Fatal("committing composition should update contenteditable text")
	}
	if got := edit.TextContent(); got != "a仮b" {
		t.Fatalf("DOM text after composition commit = %q, want %q", got, "a仮b")
	}
	if got := s.Marked(); got != "" {
		t.Fatalf("Marked after composition commit = %q, want empty", got)
	}
	wantEvents := "compositionstart::true,compositionupdate:か:true," +
		"beforeinput:insertCompositionText:か:true,input:insertCompositionText:か:true," +
		"compositionupdate:かな:true,beforeinput:insertCompositionText:かな:true,input:insertCompositionText:かな:true," +
		"beforeinput:insertFromComposition:仮:true,input:insertFromComposition:仮:true,compositionend:仮:true"
	if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != wantEvents {
		t.Fatalf("composition event order/payload = %q, err %v", got, err)
	}
	wantCancelable := "compositionstart:true,compositionupdate:false,compositionupdate:false,compositionend:false"
	if got, err := s.EvaluateJS(`imeCancelable.join(",")`); err != nil || got != wantCancelable {
		t.Fatalf("composition cancelability = %q, err %v", got, err)
	}
	wantValues := "compositionstart:@ab,compositionupdate:か@ab,beforeinput:insertCompositionText@ab," +
		"input:insertCompositionText@aかb,compositionupdate:かな@aかb," +
		"beforeinput:insertCompositionText@aかb,input:insertCompositionText@aかなb," +
		"beforeinput:insertFromComposition@aかなb,input:insertFromComposition@a仮b,compositionend:仮@a仮b"
	if got, err := s.EvaluateJS(`imeValues.join(",")`); err != nil || got != wantValues {
		t.Fatalf("contenteditable composition DOM update sequence = %q, err %v", got, err)
	}
}

func TestContentEditableIMECompositionCancel(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">text</div></body></html>`, nil, 400, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	if !s.FocusNode(edit) {
		t.Fatal("contenteditable host should accept focus")
	}
	if _, err := s.EvaluateJS(`(function(){var text=document.getElementById("edit").firstChild,r=document.createRange();r.setStart(text,2);r.collapse(true);var selection=getSelection();selection.removeAllRanges();selection.addRange(r)})()`); err != nil {
		t.Fatalf("position composition caret: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){window.imeEvents=[];var host=document.getElementById("edit");["compositionstart","compositionupdate","compositionend"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted)})});["beforeinput","input"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isTrusted)})})})()`); err != nil {
		t.Fatalf("install composition listeners: %v", err)
	}
	if !s.SetMarked("候補") || !s.SetMarked("") {
		t.Fatal("starting and canceling composition should update marked text")
	}
	if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != "compositionstart::true,compositionupdate:候補:true,beforeinput:insertCompositionText:候補:true,input:insertCompositionText:候補:true,compositionupdate::true,beforeinput:deleteCompositionText:null:true,input:deleteCompositionText:null:true,compositionend::true" {
		t.Fatalf("canceled composition events = %q, err %v", got, err)
	}
	if got := edit.TextContent(); got != "text" {
		t.Fatalf("DOM text after composition cancellation = %q, want original %q", got, "text")
	}
}

func TestContentEditableIMECommitCanceledByBeforeInput(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">text</div></body></html>`, nil, 400, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	if !s.FocusNode(edit) {
		t.Fatal("contenteditable host should accept focus")
	}
	if _, err := s.EvaluateJS(`(function(){var text=document.getElementById("edit").firstChild,r=document.createRange();r.setStart(text,2);r.collapse(true);var selection=getSelection();selection.removeAllRanges();selection.addRange(r);window.imeEvents=[];var host=document.getElementById("edit");["compositionstart","compositionupdate","compositionend"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted)})});["beforeinput","input"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isTrusted);if(e.inputType==="insertFromComposition")e.preventDefault()})})})()`); err != nil {
		t.Fatalf("install cancellation listeners: %v", err)
	}
	if !s.SetMarked("候補") {
		t.Fatal("composition preview should start")
	}
	if s.CommitText("仮") {
		t.Fatal("canceled beforeinput must suppress composition commit")
	}
	if got := edit.TextContent(); got != "text" {
		t.Fatalf("DOM after canceled composition commit = %q, want %q", got, "text")
	}
	want := "compositionstart::true,compositionupdate:候補:true,beforeinput:insertCompositionText:候補:true," +
		"input:insertCompositionText:候補:true,beforeinput:insertFromComposition:仮:true," +
		"compositionupdate::true,beforeinput:deleteCompositionText:null:true," +
		"input:deleteCompositionText:null:true,compositionend::true"
	if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != want {
		t.Fatalf("canceled contenteditable commit events = %q, err %v", got, err)
	}
}

func TestTextControlIMECompositionEvents(t *testing.T) {
	for _, tc := range []struct {
		name string
		html string
		id   string
	}{
		{name: "input", html: `<input id="edit" value="ab">`, id: "edit"},
		{name: "textarea", html: `<textarea id="edit">ab</textarea>`, id: "edit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body style="margin:0">`+tc.html+`</body></html>`, nil, 400, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			cx, cy := clickPoint(t, s, tc.id)
			s.FocusControl(cx, cy)
			if _, err := s.EvaluateJS(`(function(){window.imeEvents=[];window.imeValues=[];var host=document.getElementById("edit"),value=function(){return host.value===undefined?host.textContent:host.value};["compositionstart","compositionupdate","compositionend"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted);imeValues.push(e.type+":"+e.data+"@"+value())})});["beforeinput","input"].forEach(function(type){host.addEventListener(type,function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isComposing+":"+e.cancelable+":"+e.isTrusted);imeValues.push(e.type+":"+e.inputType+"@"+value())})})})()`); err != nil {
				t.Fatalf("install composition listeners: %v", err)
			}
			if !s.SetMarked("か") || !s.SetMarked("かな") {
				t.Fatal("composition preview updates should be accepted")
			}
			if !s.CommitText("仮") {
				t.Fatal("composition commit should update the focused control")
			}
			wantEvents := "compositionstart::true,compositionupdate:か:true," +
				"beforeinput:insertCompositionText:か:true:false:true,input:insertCompositionText:か:true:false:true," +
				"compositionupdate:かな:true,beforeinput:insertCompositionText:かな:true:false:true," +
				"input:insertCompositionText:かな:true:false:true," +
				"beforeinput:insertFromComposition:仮:true:true:true,input:insertFromComposition:仮:true:false:true,compositionend:仮:true"
			if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != wantEvents {
				t.Fatalf("composition events = %q, err %v", got, err)
			}
			wantValues := "compositionstart:@ab,compositionupdate:か@ab,beforeinput:insertCompositionText@ab," +
				"input:insertCompositionText@abか,compositionupdate:かな@abか," +
				"beforeinput:insertCompositionText@abか,input:insertCompositionText@abかな," +
				"beforeinput:insertFromComposition@abかな,input:insertFromComposition@ab仮,compositionend:仮@ab仮"
			if got, err := s.EvaluateJS(`imeValues.join(",")`); err != nil || got != wantValues {
				t.Fatalf("composition DOM update sequence = %q, err %v", got, err)
			}
			if got := s.FocusedText(); got != "ab仮" {
				t.Fatalf("committed control value = %q, want %q", got, "ab仮")
			}
		})
	}
}

func TestTextControlIMECommitCanceledByBeforeInput(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body style="margin:0"><input id="edit" value="ab"></body></html>`, nil, 400, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	cx, cy := clickPoint(t, s, "edit")
	s.FocusControl(cx, cy)
	if _, err := s.EvaluateJS(`(function(){window.imeEvents=[];var host=document.getElementById("edit");host.addEventListener("compositionstart",function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted)});host.addEventListener("compositionupdate",function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted)});host.addEventListener("compositionend",function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted)});host.addEventListener("beforeinput",function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isComposing+":"+e.cancelable+":"+e.isTrusted);if(e.inputType==="insertFromComposition")e.preventDefault()});host.addEventListener("input",function(e){imeEvents.push(e.type+":"+e.inputType+":"+e.data+":"+e.isComposing+":"+e.cancelable+":"+e.isTrusted)})})()`); err != nil {
		t.Fatalf("install cancellation listeners: %v", err)
	}
	if !s.SetMarked("候補") {
		t.Fatal("composition preview should start")
	}
	if s.CommitText("仮") {
		t.Fatal("canceled beforeinput must suppress the committed text")
	}
	if got := s.FocusedText(); got != "ab" {
		t.Fatalf("control value after canceled composition = %q, want %q", got, "ab")
	}
	wantEvents := "compositionstart::true,compositionupdate:候補:true," +
		"beforeinput:insertCompositionText:候補:true:false:true,input:insertCompositionText:候補:true:false:true," +
		"beforeinput:insertFromComposition:仮:true:true:true,compositionend::true"
	if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != wantEvents {
		t.Fatalf("canceled composition events = %q, err %v", got, err)
	}
}

func TestTextControlIMEStartCanceledByCompositionStart(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><input id="edit" value="ab"></body></html>`, nil, 400, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	cx, cy := clickPoint(t, s, "edit")
	s.FocusControl(cx, cy)
	if _, err := s.EvaluateJS(`(function(){window.imeEvents=[];document.getElementById("edit").addEventListener("compositionstart",function(e){imeEvents.push(e.type+":"+e.cancelable+":"+e.isTrusted);e.preventDefault()})})()`); err != nil {
		t.Fatalf("install compositionstart listener: %v", err)
	}
	if s.SetMarked("候補") {
		t.Fatal("canceled compositionstart should prevent starting composition")
	}
	if got := s.FocusedText(); got != "ab" {
		t.Fatalf("value after canceled compositionstart = %q, want unchanged %q", got, "ab")
	}
	if got := s.Marked(); got != "" {
		t.Fatalf("marked text after canceled compositionstart = %q, want empty", got)
	}
	if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != "compositionstart:true:true" {
		t.Fatalf("canceled compositionstart event = %q, err %v", got, err)
	}
}

func TestTextControlIMEEndsBeforeBlur(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body style="margin:0"><input id="edit"><input id="next"></body></html>`, nil, 400, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	next := s.Doc.ElementByID("next")
	cx, cy := clickPoint(t, s, "edit")
	s.FocusControl(cx, cy)
	if _, err := s.EvaluateJS(`(function(){window.imeEvents=[];var host=document.getElementById("edit");host.addEventListener("compositionend",function(e){imeEvents.push(e.type+":"+e.data+":"+e.isTrusted)});host.addEventListener("blur",function(e){imeEvents.push(e.type)})})()`); err != nil {
		t.Fatalf("install blur listeners: %v", err)
	}
	if !s.SetMarked("候補") {
		t.Fatal("composition preview should start")
	}
	if !s.BlurNode(edit) {
		t.Fatal("active control should blur")
	}
	if got, err := s.EvaluateJS(`imeEvents.join(",")`); err != nil || got != "compositionend::true,blur" {
		t.Fatalf("composition/blur order = %q, err %v", got, err)
	}
	if got := s.Marked(); got != "" {
		t.Fatalf("marked text after blur = %q, want empty", got)
	}
	if got := s.FocusedText(); got != "" {
		t.Fatalf("value after canceled blur = %q, want empty", got)
	}
	if !s.FocusNode(next) {
		t.Fatal("second input should accept programmatic focus")
	}
}

func TestPaintMarkedOnEmptyControlSuppressesPlaceholder(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="empty" placeholder="hint"></body></html>`)
	x, y := clickPoint(t, s, "empty")
	s.FocusControl(x, y)
	s.SetMarked("か")
	dl := s.Paint(1).Build(1)
	grey := frame.RGB(117, 117, 117)
	hints := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == grey {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'h' {
					hints++
				}
			}
		}
	}
	if hints != 0 {
		t.Fatalf("placeholder glyphs while composing = %d, want 0", hints)
	}
	marked := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'か' {
					marked++
				}
			}
		}
	}
	if marked == 0 {
		t.Fatal("marked preview not painted on empty control")
	}
}

func TestPaintPasswordMasksMarked(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="pw" type="password" value="ab"></body></html>`)
	x, y := clickPoint(t, s, "pw")
	s.FocusControl(x, y)
	s.SetMarked("xyz")
	dl := s.Paint(1).Build(1)
	got := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(got) != "•••••" {
		t.Fatalf("password glyphs = %q, want dots masking value and marked text", string(got))
	}
}

func TestCommitTextarea(t *testing.T) {
	s := newFocusSession(t, textareaHTML)
	x, y := clickPoint(t, s, "ta")
	s.FocusControl(x, y)
	s.SetMarked("に")
	if !s.CommitText("日") {
		t.Fatal("CommitText should report change")
	}
	if got := s.Focused().InputControlValue(); got != "hi日" {
		t.Fatalf("textarea live value = %q, want %q", got, "hi日")
	}
	if got := s.Focused().TextContent(); got != "hi" {
		t.Fatalf("textarea default text = %q, want unchanged %q", got, "hi")
	}
}

func TestPaintMarkedTextareaPreview(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<textarea id="ta">hi</textarea></body></html>`)
	x, y := clickPoint(t, s, "ta")
	s.FocusControl(x, y)
	s.Edit(engine.EditEnd, 0)
	s.SetMarked("か")
	if err := s.Reflow(800); err != nil {
		t.Fatalf("reflow composition value: %v", err)
	}
	dl := s.Paint(1).Build(1)
	runes := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(runes) != "hiか" {
		t.Fatalf("textarea painted text = %q, want %q", string(runes), "hiか")
	}
}

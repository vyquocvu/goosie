package engine_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// GOOSIE-VALIDATE-002 asked whether a submit button carrying `formnovalidate`
// still runs constraint validation. Every expectation in this file is the
// number Chromium 131.0.6778.33 produced, not a reading of the spec:
// /tmp/v2/probe1.html was served by /tmp/v2/run1.py against a local server that
// records navigation requests, so "submitted" below means the server actually
// saw `/sN?qN=` and "blocked" means it saw nothing at all. Measured 2026-11-13:
//
//	click #b1plain (no formnovalidate)  -> hits []
//	click #b1skip (formnovalidate)      -> hits ["/s1?q1="]
//	Enter in #q2 (first button is skip) -> hits ["/s2?q2="]
//	Enter in #q1 (first button is plain)-> hits []
//	Enter in #q3 (only button is skip)  -> hits ["/s3?q3="]
//	click formnovalidate=""             -> hits ["/s3?q3="]
//	click FORMNOVALIDATE (uppercase)    -> hits ["/s4?q4="]
//	click formnovalidate="false"        -> hits ["/s5?q5="]
//	click <button type=button formnovalidate> -> hits [] (no activation at all)
//	click <input type=submit formnovalidate>  -> hits ["/s8?q8="]
//	requestSubmit(f1, skip)             -> hits ["/s1?q1="]
//	requestSubmit(f1, plain) / ()       -> hits []
//	form.submit()                       -> hits ["/s1?q1="] (no validation, no event)
//
// A blocked click leaves the page at the button Chromium focused
// (activeElement=b1plain) and promotes the control into :invalid and
// :user-invalid; Goosie's own focus answer on that path is recorded in
// TestBlockedSubmissionStillPromotesTheControl, which pins the promotion it
// shares with Chromium and the focus site where the two engines differ.
//
// The same measurement session then asked the second half of the law: what a
// disabled control does with an activation. /tmp/v2/probe4.html + run4.py,
// probe5.html + run5.py/run5b.py, probe7.html + run7.py and probe8.html +
// run8b.py (Chromium 131.0.6778.33, 2026-11-13) recorded, verbatim:
//
//	mouse on a disabled button/checkbox/text/reset -> pointerdown only;
//	  no mousedown, no mouseup, no click anywhere, no submit, the checkbox
//	  stays unchecked, activeElement unchanged
//	mouse on <fieldset disabled>                 -> click:fs2 (dispatched)
//	mouse on <div disabled>                       -> click:ddiv (dispatched)
//	mouse on a <span> inside a disabled submit button -> click:ins only; the
//	  button's own listener, the form's listener and the document's capture
//	  listener never fire, and no request reaches the server
//	disabledButton.click()                        -> nothing at all
//	innerSpanOfDisabledButton.click()             -> click:ins,click:bsub,click:f1
//	  (a simulated click ignores the boundary) and no submission
//	f2.requestSubmit(dbtn2)  (disabled submitter) -> submit event with
//	  submitter dbtn2, navigation "/s2?t2=keep": the disabled submitter
//	  contributes no name/value pair
//	f2.requestSubmit()                            -> submitter null, same query
//	Enter with the first submit button disabled   -> no request, no invalid
//	  event; remove that button and Enter submits "/s2?t2=t2v"
//	dbtn2.formNoValidate false; skip.formNoValidate true; write true ->
//	  [true,"",true]; write false -> [false,false]; form[novalidate].noValidate true
//
// On engine HEAD b10fffbe the same shapes gave Goosie: a disabled submit
// button click queued "/s1?q1=" with its click listener and the form's
// listener both firing, a disabled checkbox toggled to checked, a disabled
// text input and reset button received clicks, a press on the inner span went
// to the BUTTON (click:boff fired, click:ins did not), Enter with a disabled
// first button submitted "t2=keep&e2=1" through the enabled second button,
// and `<div disabled>`.click()` dispatched nothing. Every expectation below
// is Chromium's number; the requestSubmit and IDL rows already matched Goosie
// before the change and are pinned so they cannot drift.

const vnFormHTML = `<html><body style="margin:0">` +
	`<form id="f" action="/send" method="get">` +
	`<input id="q" name="q" required>` +
	`<button id="plain" type="submit" name="go" value="1">plain</button>` +
	`<button id="skip" type="submit" name="skip" value="1" formnovalidate>skip</button>` +
	`</form></body></html>`

// qc:BR-279
func TestClickFormNoValidateSubmitterSkipsValidation(t *testing.T) {
	s := newFormSession(t, vnFormHTML)
	skipX, skipY := clickPoint(t, s, "skip")
	plainX, plainY := clickPoint(t, s, "plain")

	// Chromium: the plain button's click produces no request at all.
	if !s.ClickAt(plainX, plainY) {
		t.Fatal("click on the plain submit button was not handled")
	}
	assertNoForm(t, s)

	// Chromium: the same form, the same invalid control, one attribute more.
	if !s.ClickAt(skipX, skipY) {
		t.Fatal("click on the formnovalidate submit button was not handled")
	}
	sub := popForm(t, s)
	if sub.Encoded != "q=&skip=1" {
		t.Fatalf("formnovalidate click = %q, want q=&skip=1", sub.Encoded)
	}
}

// qc:BR-279
func TestImplicitEnterSubmissionUsesTheFirstSubmitButton(t *testing.T) {
	t.Run("first button carries formnovalidate", func(t *testing.T) {
		s := newFormSession(t, `<html><body style="margin:0"><form id="f" action="/send" method="get">`+
			`<input id="q" name="q" required>`+
			`<button id="skip" type="submit" name="skip" value="1" formnovalidate>skip</button>`+
			`<button id="plain" type="submit" name="go" value="1">plain</button>`+
			`</form></body></html>`)
		q := s.Doc.ElementByID("q")
		if !s.FocusNode(q) || !s.Edit(engine.EditEnter, 0) {
			t.Fatal("Enter in a text control did not submit")
		}
		// Chromium hit /s2?q2=: the implicit submitter is the first submit
		// button in tree order, and its novalidate state governs validation.
		if got := popForm(t, s).Encoded; got != "q=&skip=1" {
			t.Fatalf("Enter with a formnovalidate first button = %q, want q=&skip=1", got)
		}
	})
	t.Run("first button is plain", func(t *testing.T) {
		s := newFormSession(t, vnFormHTML)
		q := s.Doc.ElementByID("q")
		if !s.FocusNode(q) || !s.Edit(engine.EditEnter, 0) {
			t.Fatal("Enter in a text control was not handled")
		}
		// Chromium recorded no request for /s1 in this shape.
		assertNoForm(t, s)
	})
}

// qc:BR-279
func TestFormNoValidateIsABooleanAttribute(t *testing.T) {
	for _, tc := range []struct{ name, attr string }{
		{"empty value", `formnovalidate=""`},
		{"the string false", `formnovalidate="false"`},
		{"uppercase name in markup", `FORMNOVALIDATE`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newFormSession(t, `<html><body style="margin:0"><form id="f" action="/send" method="get">`+
				`<input id="q" name="q" required>`+
				`<button id="skip" type="submit" name="skip" value="1" `+tc.attr+`>skip</button>`+
				`</form></body></html>`)
			x, y := clickPoint(t, s, "skip")
			if !s.ClickAt(x, y) {
				t.Fatalf("click on the %s button was not handled", tc.name)
			}
			// Chromium submitted for all three shapes: presence decides, the
			// value never does.
			if got := popForm(t, s).Encoded; got != "q=&skip=1" {
				t.Fatalf("%s = %q, want q=&skip=1", tc.name, got)
			}
		})
	}
}

// qc:BR-279
func TestFormNoValidateOnANonSubmittingOrDisabledButtonDoesNothing(t *testing.T) {
	t.Run("type=button", func(t *testing.T) {
		s := newFormSession(t, `<html><body style="margin:0"><form id="f" action="/send" method="get">`+
			`<input id="q" name="q" required>`+
			`<button id="skip" type="button" name="skip" value="1" formnovalidate>not a submit button</button>`+
			`</form></body></html>`)
		x, y := clickPoint(t, s, "skip")
		s.ClickAt(x, y)
		assertNoForm(t, s)
	})
	t.Run("disabled submit button", func(t *testing.T) {
		s := newFormSession(t, `<html><body style="margin:0"><form id="f" action="/send" method="get">`+
			`<input id="q" name="q" required>`+
			`<button id="skip" type="submit" name="skip" value="1" formnovalidate disabled>skip</button>`+
			`</form></body></html>`)
		x, y := clickPoint(t, s, "skip")
		s.ClickAt(x, y)
		// Chromium would not even deliver the click: the server saw nothing.
		assertNoForm(t, s)
	})
}

// qc:BR-279
func TestInputSubmitWithFormNoValidateSubmits(t *testing.T) {
	s := newFormSession(t, `<html><body style="margin:0"><form id="f" action="/send" method="get">`+
		`<input id="q" name="q" required>`+
		`<input id="skip" type="submit" name="skip" value="1" formnovalidate>`+
		`</form></body></html>`)
	x, y := clickPoint(t, s, "skip")
	if !s.ClickAt(x, y) {
		t.Fatal("click on the input[type=submit] formnovalidate control was not handled")
	}
	if got := popForm(t, s).Encoded; got != "q=&skip=1" {
		t.Fatalf("input[type=submit] formnovalidate = %q, want q=&skip=1", got)
	}
}

// qc:BR-279
func TestBlockedSubmissionStillPromotesTheControl(t *testing.T) {
	s := newFormSession(t, vnFormHTML)
	q := s.Doc.ElementByID("q")
	invalid := 0
	dom.AddEventListener(q, "invalid", func(*dom.Event) { invalid++ }, false)
	x, y := clickPoint(t, s, "plain")
	if !s.ClickAt(x, y) {
		t.Fatal("click on the plain submit button was not handled")
	}
	assertNoForm(t, s)
	// Chromium fires one invalid event and leaves :user-invalid matching the
	// control, which is the half Goosie agrees with.
	if invalid != 1 {
		t.Fatalf("invalid events = %d, want 1", invalid)
	}
	if !q.UserInteracted {
		t.Fatal("a blocked submission must promote the control into :user-invalid")
	}
	// Where the two engines differ, recorded rather than smoothed over:
	// Chromium keeps focus on the clicked button (activeElement=b1plain), while
	// Goosie moves focus to the first invalid control - the behaviour BR-105
	// pinned on purpose.
	if s.Focused() != q {
		t.Fatalf("Goosie focus on a blocked submission = %v, want the invalid control (Chromium leaves it on the button)", s.Focused())
	}
}

// disabledProbeHTML carries every disabled shape the measurement session
// produced a number for, in the layout the probe used.
const disabledProbeHTML = `<html><body style="margin:0">` +
	`<form id="fa" action="/send" method="get">` +
	`<input id="q" name="q" required>` +
	`<button id="dskip" type="submit" name="dskip" value="dv" formnovalidate disabled>dskip</button>` +
	`<button id="dplain" type="submit" name="dplain" value="dp" disabled>dplain</button>` +
	`<input id="dcheck" type="checkbox" name="dc" disabled>` +
	`<input id="ec" type="checkbox" name="ec">` +
	`<input id="dtext" name="dtext" value="hello" disabled>` +
	`<input id="dreset" type="reset" name="dr" value="1" disabled>` +
	`<button id="bins" type="submit" name="bins" value="1" disabled>` +
	`<span id="ins" style="display:inline-block;width:60px;height:20px">press</span></button>` +
	`<div id="ddiv" disabled style="width:60px;height:20px">stray</div>` +
	`<fieldset id="dfs" disabled style="width:90px;height:24px">` +
	`<span id="fsspan" style="display:inline-block;width:40px;height:16px">in fieldset</span></fieldset>` +
	`</form></body></html>`

// qc:BR-279
func TestDisabledFormControlReceivesNoClick(t *testing.T) {
	s := newFormSession(t, disabledProbeHTML)
	seen := map[string]int{}
	count := func(id string) func(*dom.Event) {
		return func(*dom.Event) { seen[id]++ }
	}
	for _, id := range []string{"dskip", "dplain", "dcheck", "dtext", "dreset", "bins", "ins", "fa"} {
		dom.AddEventListener(s.Doc.ElementByID(id), "click", count(id), false)
	}
	dom.AddEventListener(s.Doc.ElementByID("q"), "invalid", func(*dom.Event) { seen["invalid"]++ }, false)

	// Every shape here is a control whose own box is the hit target, which is
	// the shape probe4/probe5 measured as "pointerdown only, no click". The
	// button that carries a span child is never hit on its own box, so it is
	// the boundary test's subject instead.
	for _, id := range []string{"dskip", "dplain", "dcheck", "dtext", "dreset"} {
		t.Run(id, func(t *testing.T) {
			for k := range seen {
				seen[k] = 0
			}
			x, y := clickPoint(t, s, id)
			if s.ClickAt(x, y) {
				t.Fatalf("ClickAt(%s) reported an activatable control; Chromium dispatches no mouse event at all", id)
			}
			if seen[id] != 0 || seen["ins"] != 0 || seen["fa"] != 0 {
				t.Fatalf("click listeners fired %v, want none: the press landed on a disabled control", seen)
			}
			assertNoForm(t, s)
			if seen["invalid"] != 0 {
				t.Fatalf("invalid events = %d, want 0: a disabled submitter never reaches constraint validation", seen["invalid"])
			}
			if s.Doc.ElementByID("dcheck").InputChecked() {
				t.Fatal("a disabled checkbox must not toggle")
			}
		})
	}
}

// qc:BR-279
func TestClickInsideADisabledControlStopsAtItsBoundary(t *testing.T) {
	s := newFormSession(t, disabledProbeHTML)
	seen := map[string]int{}
	for _, id := range []string{"ins", "bins", "fa", "fsspan", "dfs", "ddiv"} {
		n := s.Doc.ElementByID(id)
		seen[id] = 0
		c := id
		dom.AddEventListener(n, "click", func(*dom.Event) { seen[c]++ }, false)
	}

	// Chromium: the inner span gets the click, the disabled button and the
	// form above it do not, and nothing submits.
	x, y := clickPoint(t, s, "ins")
	if s.ClickAt(x, y) {
		t.Fatal("ClickAt inside a disabled control reported activation")
	}
	if seen["ins"] != 1 || seen["bins"] != 0 || seen["fa"] != 0 {
		t.Fatalf("click delivery = %v, want ins only", seen)
	}
	assertNoForm(t, s)

	// A fieldset is disableable but is not a control: its own box and the
	// span inside it both keep their clicks, bubbling normally.
	seen["fsspan"], seen["dfs"] = 0, 0
	x, y = clickPoint(t, s, "fsspan")
	s.ClickAt(x, y)
	if seen["fsspan"] != 1 || seen["dfs"] != 1 {
		t.Fatalf("fieldset subtree delivery = %v, want fsspan and dfs", seen)
	}

	// A stray `disabled` attribute on a non-control changes nothing.
	seen["ddiv"] = 0
	x, y = clickPoint(t, s, "ddiv")
	s.ClickAt(x, y)
	if seen["ddiv"] != 1 {
		t.Fatalf("<div disabled> click = %d, want 1", seen["ddiv"])
	}
}

// qc:BR-279
func TestDisabledFirstSubmitButtonBlocksImplicitSubmission(t *testing.T) {
	doc := `<html><body style="margin:0"><form id="f2" action="/send" method="get">` +
		`<input id="t2" name="t2" value="keep">` +
		`<button id="dbtn2" type="submit" disabled>d2</button>` +
		`<button id="ebtn2" type="submit" formnovalidate>e2</button>` +
		`</form></body></html>`
	t.Run("first submit button is disabled", func(t *testing.T) {
		s := newFormSession(t, doc)
		if !s.FocusNode(s.Doc.ElementByID("t2")) || !s.Edit(engine.EditEnter, 0) {
			t.Fatal("Enter in a text control was not handled")
		}
		// Chromium recorded no request for /s2 in this shape even though an
		// enabled formnovalidate button follows it.
		assertNoForm(t, s)
	})
	t.Run("same button enabled is the submitter", func(t *testing.T) {
		s := newFormSession(t, strings.Replace(doc, `type="submit" disabled`, `type="submit"`, 1))
		if !s.FocusNode(s.Doc.ElementByID("t2")) {
			t.Fatal("focus failed")
		}
		if !s.Edit(engine.EditEnter, 0) {
			t.Fatal("Enter in a text control did not submit")
		}
		// Measured: the same document with the attribute removed submits
		// /s2?t2=t2v - the button is unnamed, so it contributes no pair.
		if got := popForm(t, s).Encoded; got != "t2=keep" {
			t.Fatalf("Enter with an enabled first button = %q, want t2=keep", got)
		}
	})
}

// qc:BR-279
func TestRequestSubmitWithDisabledSubmitterKeepsTheMeasuredShape(t *testing.T) {
	doc := `<html><body style="margin:0">` +
		`<form id="f1" action="/send" method="get"><input id="q1" name="q1" value="v">` +
		`<button id="dskip" type="submit" name="dskip" value="dv" formnovalidate disabled>dskip</button>` +
		`<button id="dbtn2" type="submit" name="d2" value="1" disabled>d2</button>` +
		`<button id="ebtn2" type="submit" name="e2" value="1">e2</button>` +
		`<button id="bsub" type="submit" name="bs" value="1" disabled>` +
		`<span id="ins" style="display:inline-block;width:60px;height:20px">press</span></button>` +
		`</form>` +
		`<div id="ddiv" disabled style="width:60px;height:20px">stray</div></body></html>`
	s, rt := newJSSession(t, doc)
	form := s.Doc.ElementByID("f1")

	got, err := rt.Evaluate(`(function(){
	  var f = document.getElementById('f1');
	  var log = [];
	  f.addEventListener('submit', function(e){ log.push('submit:'+(e.submitter?e.submitter.id:'null')); e.preventDefault(); });
	  for (const id of ['ins','bsub','f1','ddiv']) {
	    document.getElementById(id).addEventListener('click', function(){ log.push('click:'+id); }, false);
	  }
	  var out = {};
	  f.requestSubmit(document.getElementById('dbtn2'));
	  out.disabledSubmitter = log.join(','); log.length = 0;
	  f.requestSubmit();
	  out.noSubmitter = log.join(','); log.length = 0;
	  document.getElementById('ins').click();
	  out.innerSpanClick = log.join(','); log.length = 0;
	  document.getElementById('bsub').click();
	  out.disabledButtonClick = log.join(',') || 'none'; log.length = 0;
	  document.getElementById('ddiv').click();
	  out.strayDivClick = log.join(',') || 'none'; log.length = 0;
	  var b = document.getElementById('dbtn2');
	  out.idlRead = String(b.formNoValidate);
	  b.formNoValidate = true;
	  out.idlWriteTrue = JSON.stringify([b.hasAttribute('formnovalidate'), b.getAttribute('formnovalidate'), b.formNoValidate]);
	  b.formNoValidate = false;
	  out.idlWriteFalse = JSON.stringify([b.hasAttribute('formnovalidate'), b.formNoValidate]);
	  var fr = document.createElement('form'); fr.setAttribute('novalidate','');
	  out.formIdl = JSON.stringify([fr.noValidate, fr.hasAttribute('novalidate')]);
	  return JSON.stringify(out);
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	var res map[string]string
	if err := json.Unmarshal([]byte(got), &res); err != nil {
		t.Fatalf("unmarshal %q: %v", got, err)
	}
	want := map[string]string{
		// Chromium: submit fires with the disabled button as submitter, and
		// requestSubmit() with no argument reports a null submitter.
		"disabledSubmitter":   "submit:dbtn2",
		"noSubmitter":         "submit:null",
		"innerSpanClick":      "click:ins,click:bsub,click:f1",
		"disabledButtonClick": "none",
		"strayDivClick":       "click:ddiv",
		"idlRead":             "false",
		"idlWriteTrue":        `[true,"",true]`,
		"idlWriteFalse":       "[false,false]",
		"formIdl":             "[true,true]",
	}
	for k, v := range want {
		if res[k] != v {
			t.Errorf("%s = %q, want %q (measured in Chromium)", k, res[k], v)
		}
	}

	// The disabled submitter contributes no name/value pair, which is the
	// query string Chromium's server saw for "/s2?t2=keep".
	dbtn := s.Doc.ElementByID("dbtn2")
	var pairs []string
	for _, d := range engine.CollectFormData(form, dbtn) {
		pairs = append(pairs, d.Name+"="+d.Value)
	}
	if got := strings.Join(pairs, "&"); got != "q1=v" {
		t.Fatalf("FormData with a disabled submitter = %q, want q1=v", got)
	}
	ebtn := s.Doc.ElementByID("ebtn2")
	pairs = nil
	for _, d := range engine.CollectFormData(form, ebtn) {
		pairs = append(pairs, d.Name+"="+d.Value)
	}
	if got := strings.Join(pairs, "&"); got != "q1=v&e2=1" {
		t.Fatalf("FormData with an enabled submitter = %q, want q1=v&e2=1", got)
	}
}

// qc:BR-279
func TestToggleControlRefusesADisabledCheckbox(t *testing.T) {
	// The primitive carries the law, not only its callers: the pointer path,
	// the keyboard path and `element.click()` each guard separately, so the
	// guard inside ToggleControl is observable only through this call. The
	// two numbers are the measured outcomes for the same shapes - a disabled
	// checkbox stays unchecked for a press and for `element.click()`, an
	// enabled one toggles (/tmp/v2/run4.py, run5.py, 2026-11-13).
	s := newFormSession(t, disabledProbeHTML)
	disabled := s.Doc.ElementByID("dcheck")
	if s.ToggleControl(disabled) {
		t.Fatal("ToggleControl reported a toggle for a disabled checkbox")
	}
	if disabled.InputChecked() {
		t.Fatal("a disabled checkbox changed its checked state")
	}
	enabled := s.Doc.ElementByID("ec")
	if !s.ToggleControl(enabled) {
		t.Fatal("ToggleControl refused an enabled checkbox")
	}
	if !enabled.InputChecked() {
		t.Fatal("an enabled checkbox did not toggle")
	}
}

// qc:BR-279
func TestCheckValidityAndReportValidityOnAnInvalidForm(t *testing.T) {
	// The document probe3.html measured, row for row: a required control that
	// is empty makes both methods report false whether or not a
	// formnovalidate button follows it.
	s, rt := newJSSession(t, `<html><body style="margin:0">`+
		`<form id="f1" action="/send" method="get"><input id="q1" name="q1" required>`+
		`<button id="plain" type="submit">plain</button>`+
		`<button id="skip" type="submit" formnovalidate>skip</button>`+
		`<button id="dskip" type="submit" formnovalidate disabled>dskip</button>`+
		`</form></body></html>`)
	got, err := rt.Evaluate(`JSON.stringify([
	  document.getElementById('f1').checkValidity(),
	  document.getElementById('f1').reportValidity()
	])`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got != "[false,false]" {
		t.Fatalf("checkValidity/reportValidity = %s, want [false,false]", got)
	}
	if sub, ok := s.PopSubmittedForm(); ok || sub != nil {
		t.Fatalf("a validity query must not queue a submission: %#v", sub)
	}
}

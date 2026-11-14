package engine_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

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

	if !s.ClickAt(plainX, plainY) {
		t.Fatal("click on the plain submit button was not handled")
	}
	assertNoForm(t, s)

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
	if invalid != 1 {
		t.Fatalf("invalid events = %d, want 1", invalid)
	}
	if !q.UserInteracted {
		t.Fatal("a blocked submission must promote the control into :user-invalid")
	}
	if s.Focused() != q {
		t.Fatalf("Goosie focus on a blocked submission = %v, want the invalid control (Chromium leaves it on the button)", s.Focused())
	}
}

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

	x, y := clickPoint(t, s, "ins")
	if s.ClickAt(x, y) {
		t.Fatal("ClickAt inside a disabled control reported activation")
	}
	if seen["ins"] != 1 || seen["bins"] != 0 || seen["fa"] != 0 {
		t.Fatalf("click delivery = %v, want ins only", seen)
	}
	assertNoForm(t, s)

	seen["fsspan"], seen["dfs"] = 0, 0
	x, y = clickPoint(t, s, "fsspan")
	s.ClickAt(x, y)
	if seen["fsspan"] != 1 || seen["dfs"] != 1 {
		t.Fatalf("fieldset subtree delivery = %v, want fsspan and dfs", seen)
	}

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

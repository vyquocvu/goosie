package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// --- Focus/blur/change event tests ---

func TestFocusFiresFocusEvent(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="a" value="hi"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	input := s.Doc.ElementByID("a")
	if input == nil {
		t.Fatal("input#a not found")
	}
	fired := false
	dom.AddEventListener(input, "focus", func(e *dom.Event) {
		fired = true
	}, false)

	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)

	if !fired {
		t.Fatal("focus event was not fired")
	}
}

func TestBlurFiresBlurEvent(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="a" value="hi"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	input := s.Doc.ElementByID("a")
	blurFired := false
	dom.AddEventListener(input, "blur", func(e *dom.Event) {
		blurFired = true
	}, false)

	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	// Blur by clicking empty space.
	s.FocusControl(790, 590)

	if !blurFired {
		t.Fatal("blur event was not fired")
	}
}

func TestBlurFiresChangeEventWhenValueModified(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="a" value="hi"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	input := s.Doc.ElementByID("a")
	changeFired := false
	dom.AddEventListener(input, "change", func(e *dom.Event) {
		changeFired = true
	}, false)

	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.Edit(engine.EditRune, '!') // value changes from "hi" to "hi!"
	s.FocusControl(790, 590)     // blur

	if !changeFired {
		t.Fatal("change event was not fired after value modification and blur")
	}
}

func TestBlurNoChangeEventWhenValueUnmodified(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="a" value="hi"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	input := s.Doc.ElementByID("a")
	changeFired := false
	dom.AddEventListener(input, "change", func(e *dom.Event) {
		changeFired = true
	}, false)

	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	// Do not edit.
	s.FocusControl(790, 590) // blur

	if changeFired {
		t.Fatal("change event should not fire when value was not modified")
	}
}

func TestCheckboxToggleFiresChangeEvent(t *testing.T) {
	doc := dom.NewDocument()
	cb := doc.NewElement("input")
	cb.SetAttribute("type", "checkbox")
	doc.Node.AppendChild(cb)

	changeFired := false
	dom.AddEventListener(cb, "change", func(e *dom.Event) {
		changeFired = true
	}, false)

	s := &engine.Session{}
	// Use the exported constructor to get a valid session with Doc set.
	html := `<html><body><input id="cb" type="checkbox"></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	cbNode := findNodeByTag(&sess.Doc.Node, "input")
	if cbNode == nil {
		t.Fatal("checkbox not found")
	}
	changeFired = false
	dom.AddEventListener(cbNode, "change", func(e *dom.Event) {
		changeFired = true
	}, false)

	sess.ToggleControl(cbNode)
	if !changeFired {
		t.Fatal("change event was not fired on checkbox toggle")
	}
	_ = s // unused, just to verify Session is usable
	_ = doc
}

func TestCheckboxToggleFiresInputEvent(t *testing.T) {
	html := `<html><body><input id="cb" type="checkbox"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	cb := findNodeByTag(&s.Doc.Node, "input")
	if cb == nil {
		t.Fatal("checkbox not found")
	}
	inputFired := false
	dom.AddEventListener(cb, "input", func(e *dom.Event) {
		inputFired = true
	}, false)

	s.ToggleControl(cb)
	if !inputFired {
		t.Fatal("input event was not fired on checkbox toggle")
	}
}

func TestTextInputFiresInputEvent(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="a" value="ab"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	input := s.Doc.ElementByID("a")
	if input == nil {
		t.Fatal("input#a not found")
	}
	inputFired := false
	dom.AddEventListener(input, "input", func(e *dom.Event) {
		inputFired = true
	}, false)

	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.Edit(engine.EditRune, 'x')

	if !inputFired {
		t.Fatal("input event was not fired on text edit")
	}
}

func TestBackspaceFiresInputEvent(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="a" value="ab"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	input := s.Doc.ElementByID("a")
	inputFired := false
	dom.AddEventListener(input, "input", func(e *dom.Event) {
		inputFired = true
	}, false)

	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.Edit(engine.EditBackspace, 0)

	if !inputFired {
		t.Fatal("input event was not fired on backspace")
	}
}

// --- Form submission event tests ---

func TestSubmitFormFiresSubmitEvent(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	form := doc.NewElement("form")
	form.SetAttribute("action", "/submit")
	doc.Node.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(form)

	submitFired := false
	dom.AddEventListener(form, "submit", func(e *dom.Event) {
		submitFired = true
	}, false)

	s, err := engine.NewSession(`<html><body><form id="f" action="/s"></form></body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	formNode := findNodeByTag(&s.Doc.Node, "form")
	if formNode == nil {
		t.Fatal("form not found")
	}
	submitFired = false
	dom.AddEventListener(formNode, "submit", func(e *dom.Event) {
		submitFired = true
	}, false)

	s.SubmitForm(formNode)
	if !submitFired {
		t.Fatal("submit event was not fired")
	}
	_ = doc
}

func TestSubmitEventCancelable(t *testing.T) {
	s, err := engine.NewSession(`<html><body><form id="f" action="/s"><input id="inp" type="text" name="q" value="hello"></form></body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	formNode := findNodeByTag(&s.Doc.Node, "form")
	if formNode == nil {
		t.Fatal("form not found")
	}

	// Register a listener that prevents default.
	dom.AddEventListener(formNode, "submit", func(e *dom.Event) {
		e.PreventDefault()
	}, false)

	// Track whether form data would be collected by wrapping SubmitForm
	// behavior: if preventDefault works, SubmitForm returns early.
	// We verify by checking that the event was indeed prevented.
	ev := dom.NewEvent("submit", true, true)
	result := dom.DispatchEvent(formNode, ev)
	if result {
		t.Fatal("submit event should have been cancelled by preventDefault")
	}
	if !ev.DefaultPrevented() {
		t.Fatal("event should report DefaultPrevented")
	}
}

// --- Tabbable controls tests ---

func TestTabbableControls(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	form := doc.NewElement("form")

	text := doc.NewElement("input")
	text.SetAttribute("type", "text")
	text.SetAttribute("name", "q")

	hidden := doc.NewElement("input")
	hidden.SetAttribute("type", "hidden")
	hidden.SetAttribute("name", "token")

	disabled := doc.NewElement("input")
	disabled.SetAttribute("type", "text")
	disabled.SetAttribute("disabled", "")

	ta := doc.NewElement("textarea")
	sel := doc.NewElement("select")
	btn := doc.NewElement("button")
	link := doc.NewElement("a")
	link.SetAttribute("href", "/page")
	noLink := doc.NewElement("a") // no href

	form.AppendChild(text)
	form.AppendChild(hidden)
	form.AppendChild(disabled)
	form.AppendChild(ta)
	form.AppendChild(sel)
	form.AppendChild(btn)
	form.AppendChild(link)
	form.AppendChild(noLink)
	body.AppendChild(form)
	html.AppendChild(body)
	doc.Node.AppendChild(html)

	controls := engine.TabbableControls(doc)

	// Expected tabbable: text input, textarea, select, button, a[href].
	// NOT tabbable: hidden input, disabled input, a without href.
	wantTags := []string{"input", "textarea", "select", "button", "a"}
	if len(controls) != len(wantTags) {
		t.Fatalf("tabbable controls = %d, want %d", len(controls), len(wantTags))
	}
	for i, c := range controls {
		if c.Data != wantTags[i] {
			t.Errorf("controls[%d] = %q, want %q", i, c.Data, wantTags[i])
		}
	}
}

func TestFocusNextCycles(t *testing.T) {
	html := `<html><body style="margin: 0;">` +
		`<input id="a" value="a">` +
		`<input id="b" value="b">` +
		`<input id="c" value="c">` +
		`</body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Focus the first input via click.
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if got := s.Focused().GetAttribute("id"); got != "a" {
		t.Fatalf("focused = %q, want a", got)
	}

	// Tab to next.
	s.FocusNext()
	if got := s.Focused().GetAttribute("id"); got != "b" {
		t.Fatalf("after FocusNext: focused = %q, want b", got)
	}

	s.FocusNext()
	if got := s.Focused().GetAttribute("id"); got != "c" {
		t.Fatalf("after 2nd FocusNext: focused = %q, want c", got)
	}

	// Wrap around.
	s.FocusNext()
	if got := s.Focused().GetAttribute("id"); got != "a" {
		t.Fatalf("after wrap: focused = %q, want a", got)
	}
}

func TestFocusPrevCycles(t *testing.T) {
	html := `<html><body style="margin: 0;">` +
		`<input id="a" value="a">` +
		`<input id="b" value="b">` +
		`<input id="c" value="c">` +
		`</body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Focus the first input via click.
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)

	// Tab prev should wrap to last.
	s.FocusPrev()
	if got := s.Focused().GetAttribute("id"); got != "c" {
		t.Fatalf("after FocusPrev from a: focused = %q, want c", got)
	}

	s.FocusPrev()
	if got := s.Focused().GetAttribute("id"); got != "b" {
		t.Fatalf("after 2nd FocusPrev: focused = %q, want b", got)
	}
}

func TestHiddenInputNotTabbable(t *testing.T) {
	doc := dom.NewDocument()
	inp := doc.NewElement("input")
	inp.SetAttribute("type", "hidden")
	doc.Node.AppendChild(inp)

	controls := engine.TabbableControls(doc)
	for _, c := range controls {
		if c.GetAttribute("type") == "hidden" {
			t.Fatal("hidden input should not be tabbable")
		}
	}
	if len(controls) != 0 {
		t.Fatalf("tabbable controls = %d, want 0", len(controls))
	}
}

func TestDisabledInputNotTabbable(t *testing.T) {
	doc := dom.NewDocument()
	inp := doc.NewElement("input")
	inp.SetAttribute("type", "text")
	inp.SetAttribute("disabled", "")
	doc.Node.AppendChild(inp)

	controls := engine.TabbableControls(doc)
	if len(controls) != 0 {
		t.Fatalf("tabbable controls = %d, want 0 (disabled input)", len(controls))
	}
}

func TestAnchorWithHrefIsTabbable(t *testing.T) {
	doc := dom.NewDocument()
	a := doc.NewElement("a")
	a.SetAttribute("href", "/page")
	doc.Node.AppendChild(a)

	controls := engine.TabbableControls(doc)
	if len(controls) != 1 {
		t.Fatalf("tabbable controls = %d, want 1", len(controls))
	}
	if controls[0].Data != "a" {
		t.Fatalf("controls[0] = %q, want a", controls[0].Data)
	}
}

func TestAnchorWithoutHrefNotTabbable(t *testing.T) {
	doc := dom.NewDocument()
	a := doc.NewElement("a")
	doc.Node.AppendChild(a)

	controls := engine.TabbableControls(doc)
	if len(controls) != 0 {
		t.Fatalf("tabbable controls = %d, want 0 (a without href)", len(controls))
	}
}

package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func buildTestForm() *dom.Node {
	doc := dom.NewDocument()
	form := doc.NewElement("form")
	form.SetAttribute("action", "/submit")
	form.SetAttribute("method", "post")

	// Text input
	text := doc.NewElement("input")
	text.SetAttribute("type", "text")
	text.SetAttribute("name", "username")
	text.SetAttribute("value", "alice")
	form.AppendChild(text)

	// Hidden input
	hidden := doc.NewElement("input")
	hidden.SetAttribute("type", "hidden")
	hidden.SetAttribute("name", "token")
	hidden.SetAttribute("value", "abc123")
	form.AppendChild(hidden)

	// Checkbox (checked)
	check := doc.NewElement("input")
	check.SetAttribute("type", "checkbox")
	check.SetAttribute("name", "agree")
	check.SetAttribute("checked", "")
	form.AppendChild(check)

	// Checkbox (unchecked)
	check2 := doc.NewElement("input")
	check2.SetAttribute("type", "checkbox")
	check2.SetAttribute("name", "newsletter")
	form.AppendChild(check2)

	// Textarea
	ta := doc.NewElement("textarea")
	ta.SetAttribute("name", "bio")
	ta.AppendChild(doc.NewText("Hello world"))
	form.AppendChild(ta)

	// Select
	sel := doc.NewElement("select")
	sel.SetAttribute("name", "color")
	opt1 := doc.NewElement("option")
	opt1.SetAttribute("value", "red")
	opt1.AppendChild(doc.NewText("Red"))
	sel.AppendChild(opt1)
	opt2 := doc.NewElement("option")
	opt2.SetAttribute("value", "blue")
	opt2.SetAttribute("selected", "")
	opt2.AppendChild(doc.NewText("Blue"))
	sel.AppendChild(opt2)
	form.AppendChild(sel)

	// Disabled input (should be skipped)
	dis := doc.NewElement("input")
	dis.SetAttribute("type", "text")
	dis.SetAttribute("name", "disabled_field")
	dis.SetAttribute("value", "nope")
	dis.SetAttribute("disabled", "")
	form.AppendChild(dis)

	return form
}

func TestCollectFormDataText(t *testing.T) {
	form := buildTestForm()
	data := engine.CollectFormData(form)

	found := false
	for _, d := range data {
		if d.Name == "username" {
			if d.Value != "alice" {
				t.Errorf("username = %q, want %q", d.Value, "alice")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("username field not found")
	}
}

func TestCollectFormDataHidden(t *testing.T) {
	form := buildTestForm()
	data := engine.CollectFormData(form)

	found := false
	for _, d := range data {
		if d.Name == "token" {
			if d.Value != "abc123" {
				t.Errorf("token = %q, want %q", d.Value, "abc123")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("token field not found")
	}
}

func TestCollectFormDataCheckbox(t *testing.T) {
	form := buildTestForm()
	data := engine.CollectFormData(form)

	// Should find checked checkbox
	foundChecked := false
	for _, d := range data {
		if d.Name == "agree" {
			foundChecked = true
			break
		}
	}
	if !foundChecked {
		t.Error("checked checkbox 'agree' not found")
	}

	// Should not find unchecked checkbox
	for _, d := range data {
		if d.Name == "newsletter" {
			t.Error("unchecked checkbox 'newsletter' should not be collected")
		}
	}
}

func TestCollectFormDataTextarea(t *testing.T) {
	form := buildTestForm()
	data := engine.CollectFormData(form)

	found := false
	for _, d := range data {
		if d.Name == "bio" {
			if d.Value != "Hello world" {
				t.Errorf("bio = %q, want %q", d.Value, "Hello world")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("bio field not found")
	}
}

func TestCollectFormDataSelect(t *testing.T) {
	form := buildTestForm()
	data := engine.CollectFormData(form)

	found := false
	for _, d := range data {
		if d.Name == "color" {
			if d.Value != "blue" {
				t.Errorf("color = %q, want %q", d.Value, "blue")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("color field not found")
	}
}

func TestCollectFormDataSkipsDisabled(t *testing.T) {
	form := buildTestForm()
	data := engine.CollectFormData(form)

	for _, d := range data {
		if d.Name == "disabled_field" {
			t.Error("disabled field should not be collected")
		}
	}
}

func TestCollectFormDataSkipsUnnamed(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")

	// Input without name
	input := doc.NewElement("input")
	input.SetAttribute("type", "text")
	input.SetAttribute("value", "no_name")
	form.AppendChild(input)

	// Input with name
	input2 := doc.NewElement("input")
	input2.SetAttribute("type", "text")
	input2.SetAttribute("name", "has_name")
	input2.SetAttribute("value", "yes")
	form.AppendChild(input2)

	data := engine.CollectFormData(form)

	if len(data) != 1 {
		t.Errorf("len(data) = %d, want 1", len(data))
	}
	if len(data) > 0 && data[0].Name != "has_name" {
		t.Errorf("data[0].Name = %q, want %q", data[0].Name, "has_name")
	}
}

func TestCollectFormDataMaxControls(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")

	// Add more than MaxFormControls inputs
	for i := 0; i < engine.MaxFormControls+10; i++ {
		input := doc.NewElement("input")
		input.SetAttribute("type", "text")
		input.SetAttribute("name", "field")
		input.SetAttribute("value", "val")
		form.AppendChild(input)
	}

	data := engine.CollectFormData(form)

	if len(data) > engine.MaxFormControls {
		t.Errorf("len(data) = %d, want <= %d", len(data), engine.MaxFormControls)
	}
}

func TestEncodeFormURLEncoded(t *testing.T) {
	data := []engine.FormData{
		{Name: "username", Value: "alice"},
		{Name: "token", Value: "abc123"},
	}

	result := engine.EncodeFormURLEncoded(data)
	expected := "username=alice&token=abc123"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestEncodeFormURLEncodedSpecialChars(t *testing.T) {
	data := []engine.FormData{
		{Name: "q", Value: "hello world"},
		{Name: "special", Value: "a&b=c"},
	}

	result := engine.EncodeFormURLEncoded(data)
	expected := "q=hello+world&special=a%26b%3Dc"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestFormMethod(t *testing.T) {
	doc := dom.NewDocument()

	// POST method
	form1 := doc.NewElement("form")
	form1.SetAttribute("method", "post")
	if got := engine.FormMethod(form1); got != "POST" {
		t.Errorf("FormMethod(post) = %q, want POST", got)
	}

	// GET method (explicit)
	form2 := doc.NewElement("form")
	form2.SetAttribute("method", "get")
	if got := engine.FormMethod(form2); got != "GET" {
		t.Errorf("FormMethod(get) = %q, want GET", got)
	}

	// Default (no method attribute)
	form3 := doc.NewElement("form")
	if got := engine.FormMethod(form3); got != "GET" {
		t.Errorf("FormMethod(default) = %q, want GET", got)
	}
}

func TestFormAction(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")
	form.SetAttribute("action", "/submit")

	if got := engine.FormAction(form); got != "/submit" {
		t.Errorf("FormAction = %q, want %q", got, "/submit")
	}

	// No action
	form2 := doc.NewElement("form")
	if got := engine.FormAction(form2); got != "" {
		t.Errorf("FormAction(empty) = %q, want empty", got)
	}
}

func TestFormEnctype(t *testing.T) {
	doc := dom.NewDocument()

	// Default
	form1 := doc.NewElement("form")
	if got := engine.FormEnctype(form1); got != "application/x-www-form-urlencoded" {
		t.Errorf("FormEnctype(default) = %q", got)
	}

	// multipart/form-data
	form2 := doc.NewElement("form")
	form2.SetAttribute("enctype", "multipart/form-data")
	if got := engine.FormEnctype(form2); got != "multipart/form-data" {
		t.Errorf("FormEnctype(multipart) = %q", got)
	}

	// text/plain
	form3 := doc.NewElement("form")
	form3.SetAttribute("enctype", "text/plain")
	if got := engine.FormEnctype(form3); got != "text/plain" {
		t.Errorf("FormEnctype(text) = %q", got)
	}
}

func TestFindForm(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")
	input := doc.NewElement("input")
	form.AppendChild(input)

	found := engine.FindForm(input)
	if found == nil {
		t.Error("FindForm returned nil, want form")
	}
	if found != form {
		t.Error("FindForm returned wrong form")
	}
}

func TestFindFormNoForm(t *testing.T) {
	doc := dom.NewDocument()
	input := doc.NewElement("input")

	found := engine.FindForm(input)
	if found != nil {
		t.Errorf("FindForm = %v, want nil", found)
	}
}

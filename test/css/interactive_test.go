package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestHoverPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.NewElement("div")
	doc.Node.AppendChild(div)

	// Initially not hovered.
	if css.IsHovered(div) {
		t.Error("element should not be hovered initially")
	}

	// Set hover state.
	css.SetHover(div, true)
	if !css.IsHovered(div) {
		t.Error("element should be hovered after SetHover(true)")
	}

	// Test selector matching.
	sel := css.ParseSelector("div:hover")
	if !sel.Matches(div) {
		t.Error("div:hover should match hovered div")
	}

	// Remove hover state.
	css.SetHover(div, false)
	if css.IsHovered(div) {
		t.Error("element should not be hovered after SetHover(false)")
	}

	// Selector should no longer match.
	if sel.Matches(div) {
		t.Error("div:hover should not match unhovered div")
	}
}

func TestFocusPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	input := doc.NewElement("input")
	doc.Node.AppendChild(input)

	// Initially not focused.
	if css.IsFocused(input) {
		t.Error("element should not be focused initially")
	}

	// Set focus state.
	css.SetFocus(input, true)
	if !css.IsFocused(input) {
		t.Error("element should be focused after SetFocus(true)")
	}

	// Test selector matching.
	sel := css.ParseSelector("input:focus")
	if !sel.Matches(input) {
		t.Error("input:focus should match focused input")
	}

	// Remove focus state.
	css.SetFocus(input, false)
	if css.IsFocused(input) {
		t.Error("element should not be focused after SetFocus(false)")
	}

	// Selector should no longer match.
	if sel.Matches(input) {
		t.Error("input:focus should not match unfocused input")
	}
}

func TestHoverAndFocusCombined(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.NewElement("button")
	doc.Node.AppendChild(btn)

	// Test combined selector.
	sel := css.ParseSelector("button:hover:focus")

	// Neither hovered nor focused.
	if sel.Matches(btn) {
		t.Error("button:hover:focus should not match when neither state is set")
	}

	// Only hovered.
	css.SetHover(btn, true)
	if sel.Matches(btn) {
		t.Error("button:hover:focus should not match when only hovered")
	}

	// Both hovered and focused.
	css.SetFocus(btn, true)
	if !sel.Matches(btn) {
		t.Error("button:hover:focus should match when both states are set")
	}

	// Only focused.
	css.SetHover(btn, false)
	if sel.Matches(btn) {
		t.Error("button:hover:focus should not match when only focused")
	}
}

func TestClearInteractiveState(t *testing.T) {
	doc := dom.NewDocument()
	div1 := doc.NewElement("div")
	div2 := doc.NewElement("div")
	doc.Node.AppendChild(div1)
	doc.Node.AppendChild(div2)

	// Set various states.
	css.SetHover(div1, true)
	css.SetFocus(div2, true)

	if !css.IsHovered(div1) || !css.IsFocused(div2) {
		t.Fatal("states should be set")
	}

	// Clear all states.
	css.ClearInteractiveState()

	if css.IsHovered(div1) {
		t.Error("hover state should be cleared")
	}
	if css.IsFocused(div2) {
		t.Error("focus state should be cleared")
	}
}

func TestHoverOnNilNode(t *testing.T) {
	// Should not panic.
	css.SetHover(nil, true)
	if css.IsHovered(nil) {
		t.Error("nil node should not be hovered")
	}
}

func TestFocusOnNilNode(t *testing.T) {
	// Should not panic.
	css.SetFocus(nil, true)
	if css.IsFocused(nil) {
		t.Error("nil node should not be focused")
	}
}

func TestFormStatePseudoClasses(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")
	doc.Node.AppendChild(form)
	checked := doc.NewElement("input")
	checked.SetAttribute("type", "checkbox")
	checked.SetAttribute("checked", "")
	form.AppendChild(checked)
	unchecked := doc.NewElement("input")
	unchecked.SetAttribute("type", "radio")
	form.AppendChild(unchecked)
	textInput := doc.NewElement("input")
	textInput.SetAttribute("type", "text")
	form.AppendChild(textInput)

	for _, tc := range []struct {
		selector string
		node     *dom.Node
		want     bool
	}{
		{"input:checked", checked, true},
		{"input:checked", unchecked, false},
		{"input:checked", textInput, false},
		{"input:enabled", textInput, true},
		{"input:disabled", textInput, false},
	} {
		if got := css.ParseSelector(tc.selector).Matches(tc.node); got != tc.want {
			t.Errorf("%s match = %v, want %v", tc.selector, got, tc.want)
		}
	}

	fieldset := doc.NewElement("fieldset")
	fieldset.SetAttribute("disabled", "")
	form.AppendChild(fieldset)
	legend := doc.NewElement("legend")
	fieldset.AppendChild(legend)
	legendInput := doc.NewElement("input")
	legend.AppendChild(legendInput)
	disabledInput := doc.NewElement("input")
	fieldset.AppendChild(disabledInput)
	if !css.ParseSelector("input:enabled").Matches(legendInput) {
		t.Error("input inside the first legend of a disabled fieldset should remain enabled")
	}
	if !css.ParseSelector("input:disabled").Matches(disabledInput) {
		t.Error("input outside the first legend of a disabled fieldset should be disabled")
	}
	if css.ParseSelector("input:enabled").Matches(disabledInput) {
		t.Error("disabled fieldset descendant also matched :enabled")
	}

	selectNode := doc.NewElement("select")
	firstOption := doc.NewElement("option")
	secondOption := doc.NewElement("option")
	selectNode.AppendChild(firstOption)
	selectNode.AppendChild(secondOption)
	form.AppendChild(selectNode)
	if !css.ParseSelector("option:checked").Matches(firstOption) {
		t.Error("first option should be checked when a single select has no selected option")
	}
	secondOption.SetAttribute("selected", "")
	if css.ParseSelector("option:checked").Matches(firstOption) || !css.ParseSelector("option:checked").Matches(secondOption) {
		t.Error("selected option state was not reflected by :checked")
	}

	placeholder := doc.NewElement("input")
	placeholder.SetAttribute("placeholder", "Search")
	form.AppendChild(placeholder)
	if !css.ParseSelector("input:placeholder-shown").Matches(placeholder) {
		t.Error("empty input with placeholder should match :placeholder-shown")
	}
	placeholder.SetAttribute("value", "query")
	if css.ParseSelector("input:placeholder-shown").Matches(placeholder) {
		t.Error("non-empty input should not match :placeholder-shown")
	}
	textarea := doc.NewElement("textarea")
	textarea.SetAttribute("placeholder", "Notes")
	form.AppendChild(textarea)
	if !css.ParseSelector("textarea:placeholder-shown").Matches(textarea) {
		t.Error("empty textarea with placeholder should match :placeholder-shown")
	}
	textarea.AppendChild(doc.NewText("saved"))
	if css.ParseSelector("textarea:placeholder-shown").Matches(textarea) {
		t.Error("textarea with text should not match :placeholder-shown")
	}
}

func TestFocusWithinPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	section := doc.NewElement("section")
	input := doc.NewElement("input")
	section.AppendChild(input)
	doc.Node.AppendChild(section)

	selector := css.ParseSelector("section:focus-within")
	if selector.Matches(section) {
		t.Fatal(":focus-within matched before a descendant received focus")
	}
	css.SetFocus(input, true)
	if !selector.Matches(section) {
		t.Fatal(":focus-within did not match an ancestor of the focused node")
	}
	css.SetFocus(input, false)
	if selector.Matches(section) {
		t.Fatal(":focus-within remained matched after focus was removed")
	}
}

func TestFocusVisiblePseudoClassRequiresFocusedVisibleState(t *testing.T) {
	doc := dom.NewDocument()
	button := doc.NewElement("button")
	doc.Node.AppendChild(button)
	selector := css.ParseSelector("button:focus-visible")

	if selector.Matches(button) {
		t.Fatal(":focus-visible matched an unfocused button")
	}
	css.SetFocus(button, true)
	if selector.Matches(button) {
		t.Fatal(":focus-visible matched pointer-style focus without a visible indicator")
	}
	css.SetFocusVisible(button, true)
	if !selector.Matches(button) {
		t.Fatal(":focus-visible did not match keyboard-style focus")
	}
	css.SetFocus(button, false)
	if selector.Matches(button) {
		t.Fatal(":focus-visible remained matched after blur")
	}
}

func TestActivePseudoClassTracksPointerActivation(t *testing.T) {
	doc := dom.NewDocument()
	button := doc.NewElement("button")
	doc.Node.AppendChild(button)
	selector := css.ParseSelector("button:active")

	if selector.Matches(button) {
		t.Fatal(":active matched before pointer activation")
	}
	css.SetActive(button, true)
	if !selector.Matches(button) {
		t.Fatal(":active did not match during pointer activation")
	}
	css.SetActive(button, false)
	if selector.Matches(button) {
		t.Fatal(":active remained matched after pointer release")
	}
}

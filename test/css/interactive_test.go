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

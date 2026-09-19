package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

// `padding-inline` and friends are the only way several modern sheets say it:
// MDN sets its page gutter with `padding-inline: max(1rem, calc(50vw - 720px +
// 1rem))` and no physical equivalent, so an engine that ignores the logical name
// renders the whole document flush against both edges.
func logical(t *testing.T, html, sheet string) *style.ComputedStyle {
	t.Helper()
	doc := dom.Parse(html)
	var sheets []*css.Stylesheet
	if sheet != "" {
		sheets = append(sheets, css.Parse(sheet))
	}
	styles := style.ResolveViewport(doc, sheets, style.Viewport{W: 1600, H: 900})
	var found *style.ComputedStyle
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n; c != nil && found == nil; c = c.NextSibling {
			if c.Element() && c.Data == "div" {
				if s, ok := styles[c.ID]; ok {
					found = s
					return
				}
			}
			walk(c.FirstChild)
		}
	}
	walk(doc.Node.FirstChild)
	if found == nil {
		t.Fatalf("no computed style for the div")
	}
	return found
}

func TestLogicalBoxPropertiesMapToThePhysicalOnes(t *testing.T) {
	cases := []struct{ name, sheet string }{
		{"padding-inline", "div { padding-inline: 20px }"},
		{"padding-left", "div { padding-left: 20px } div { padding-right: 20px }"},
	}
	for _, c := range cases {
		s := logical(t, "<div>x</div>", c.sheet)
		if s.PaddingLeft != 20 || s.PaddingRight != 20 {
			t.Errorf("%s: PaddingLeft/Right = %v/%v, want 20/20", c.name, s.PaddingLeft, s.PaddingRight)
		}
		if s.PaddingTop != 0 || s.PaddingBottom != 0 {
			t.Errorf("%s: PaddingTop/Bottom = %v/%v, want 0/0", c.name, s.PaddingTop, s.PaddingBottom)
		}
	}
	s := logical(t, "<div>x</div>", "div { padding-inline: 10px 30px; padding-block: 5px 7px; margin-inline-start: 4px; margin-block-end: 9px }")
	if s.PaddingLeft != 10 || s.PaddingRight != 30 {
		t.Errorf("two-value padding-inline = %v/%v, want 10/30", s.PaddingLeft, s.PaddingRight)
	}
	if s.PaddingTop != 5 || s.PaddingBottom != 7 {
		t.Errorf("padding-block = %v/%v, want 5/7", s.PaddingTop, s.PaddingBottom)
	}
	if s.MarginLeft != 4 {
		t.Errorf("margin-inline-start = %v, want 4", s.MarginLeft)
	}
	if s.MarginBottom != 9 {
		t.Errorf("margin-block-end = %v, want 9", s.MarginBottom)
	}
}

// The value the property carries is the whole reason for the function support:
// one side is a share of the viewport, the other a floor in `rem`, and the frame
// is 1600px here.
func TestLogicalPaddingEvaluatesAFunctionOfViewportUnits(t *testing.T) {
	s := logical(t, "<div>x</div>", "div { padding-inline: max(1rem, calc(50vw - 720px + 1rem)) }")
	if s.PaddingLeft != 96 || s.PaddingRight != 96 {
		t.Errorf("padding-inline = %v/%v, want 96/96 (50vw - 720px + 1rem at a 1600px frame)", s.PaddingLeft, s.PaddingRight)
	}
}

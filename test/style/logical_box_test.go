package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// `padding-inline` and friends are the only way several modern sheets say it:
// MDN sets its page gutter with `padding-inline: max(1rem, calc(50vw - 720px +
// 1rem))` and no physical equivalent, so an engine that ignores the logical name
// renders the whole document flush against both edges.
func logical(t *testing.T, html, sheet string) *style.ComputedStyle {
	t.Helper()
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	if sheet != "" {
		sheets = append(sheets, css.Parse(sheet))
	}
	styles := style.ResolveViewport(doc, sheets, style.Viewport{W: 1600, H: 900}, nil)
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

func TestLogicalSizingProperties(t *testing.T) {
	s := logical(t, "<div>x</div>", "div { inline-size: 250px; block-size: 150px; min-inline-size: 100px; max-inline-size: 400px; min-block-size: 50px; max-block-size: 300px; }")
	if s.Width != 250 {
		t.Errorf("inline-size = %v, want 250", s.Width)
	}
	if s.Height != 150 {
		t.Errorf("block-size = %v, want 150", s.Height)
	}
	if s.MinWidth != 100 {
		t.Errorf("min-inline-size = %v, want 100", s.MinWidth)
	}
	if s.MaxWidth != 400 {
		t.Errorf("max-inline-size = %v, want 400", s.MaxWidth)
	}
	if s.MinHeight != 50 {
		t.Errorf("min-block-size = %v, want 50", s.MinHeight)
	}
	if s.MaxHeight != 300 {
		t.Errorf("max-block-size = %v, want 300", s.MaxHeight)
	}
}

func TestLogicalBordersAndInsets(t *testing.T) {
	s := logical(t, "<div>x</div>", "div { border-inline: 2px solid red; border-block: 4px dashed blue; inset-inline: 15px 25px; inset-block: 5px 10px; }")
	if s.BorderLeftWidth != 2 || s.BorderRightWidth != 2 {
		t.Errorf("border-inline width = %v/%v, want 2/2", s.BorderLeftWidth, s.BorderRightWidth)
	}
	if s.BorderTopWidth != 4 || s.BorderBottomWidth != 4 {
		t.Errorf("border-block width = %v/%v, want 4/4", s.BorderTopWidth, s.BorderBottomWidth)
	}
	if s.Left != 15 || s.Right != 25 {
		t.Errorf("inset-inline = %v/%v, want 15/25", s.Left, s.Right)
	}
	if s.Top != 5 || s.Bottom != 10 {
		t.Errorf("inset-block = %v/%v, want 5/10", s.Top, s.Bottom)
	}
}

func TestLogicalDirectionRTL(t *testing.T) {
	// LTR
	sLTR := logical(t, "<div>x</div>", "div { direction: ltr; margin-inline-start: 10px; margin-inline-end: 20px; text-align: start; }")
	if sLTR.Direction != "ltr" {
		t.Errorf("direction = %q, want 'ltr'", sLTR.Direction)
	}
	if sLTR.MarginLeft != 10 || sLTR.MarginRight != 20 {
		t.Errorf("LTR margin-inline-start/end = %v/%v, want 10/20", sLTR.MarginLeft, sLTR.MarginRight)
	}
	if sLTR.TextAlign != style.TextAlignLeft {
		t.Errorf("LTR text-align: start = %v, want TextAlignLeft", sLTR.TextAlign)
	}

	// RTL
	sRTL := logical(t, "<div>x</div>", "div { direction: rtl; margin-inline-start: 10px; margin-inline-end: 20px; text-align: start; }")
	if sRTL.Direction != "rtl" {
		t.Errorf("direction = %q, want 'rtl'", sRTL.Direction)
	}
	if sRTL.MarginRight != 10 || sRTL.MarginLeft != 20 {
		t.Errorf("RTL margin-inline-start/end = %v/%v, want MarginRight=10 MarginLeft=20", sRTL.MarginRight, sRTL.MarginLeft)
	}
	if sRTL.TextAlign != style.TextAlignRight {
		t.Errorf("RTL text-align: start = %v, want TextAlignRight", sRTL.TextAlign)
	}
}

func TestGridPlacementLonghands(t *testing.T) {
	s := logical(t, "<div>x</div>", "div { grid-column-start: 2; grid-column-end: span 3; grid-row-start: 1; grid-row-end: 4; }")
	if s.GridColumn != "2 / span 3" {
		t.Errorf("grid-column = %q, want '2 / span 3'", s.GridColumn)
	}
	if s.GridColumnSpan != 3 {
		t.Errorf("grid-column-span = %d, want 3", s.GridColumnSpan)
	}
	if s.GridRow != "1 / 4" {
		t.Errorf("grid-row = %q, want '1 / 4'", s.GridRow)
	}
	if s.GridRowSpan != 3 {
		t.Errorf("grid-row-span = %d, want 3", s.GridRowSpan)
	}
}

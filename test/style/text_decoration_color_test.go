package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func decoDoc(t *testing.T, sheetText string) (map[dom.NodeID]*style.ComputedStyle, *dom.Node, *dom.Node) {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	p := doc.NewElement("p")
	p.SetAttribute("id", "p1")
	body.AppendChild(p)
	span := doc.NewElement("span")
	span.SetAttribute("id", "span1")
	p.AppendChild(span)
	return style.Resolve(doc, []*css.Stylesheet{css.Parse(sheetText)}, nil), p, span
}

func rgba(c css.Color) [4]uint8 { return [4]uint8{c.R, c.G, c.B, c.A} }

// qc:BR-324
func TestTextDecorationColorFromShorthandAndLonghand(t *testing.T) {
	styles, p, _ := decoDoc(t, `
		#p1 {
			text-decoration: underline solid rgb(255, 0, 0);
			text-decoration-thickness: 2px;
		}
	`)
	cs := styles[p.ID]
	if got := rgba(cs.TextDecorationColor); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("shorthand colour = %v, want {255 0 0 255}", got)
	}
	if cs.TextDecoration&style.TextDecorationUnderline == 0 {
		t.Errorf("shorthand lost the underline flag while reading its colour")
	}

	styles, p, _ = decoDoc(t, `
		#p1 {
			text-decoration: underline;
			text-decoration-color: rgb(0, 0, 255);
		}
	`)
	if got := rgba(styles[p.ID].TextDecorationColor); got != [4]uint8{0, 0, 255, 255} {
		t.Errorf("longhand colour = %v, want {0 0 255 255}", got)
	}

	styles, p, _ = decoDoc(t, `#p1 { text-decoration: underline solid auto; }`)
	if a := styles[p.ID].TextDecorationColor.A; a != 0 {
		t.Errorf("shorthand with an explicit auto colour kept alpha %d, want 0", a)
	}

	styles, p, _ = decoDoc(t, `#p1 { text-decoration: underline solid RGB(255, 0, 0); }`)
	if got := rgba(styles[p.ID].TextDecorationColor); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("upper-case RGB() colour = %v, want {255 0 0 255}", got)
	}
}

// qc:BR-324
func TestTextDecorationShorthandResetsTheColorBySourceOrder(t *testing.T) {
	styles, p, _ := decoDoc(t, `
		#p1 { text-decoration-color: rgb(0, 0, 255); text-decoration: underline solid rgb(255, 0, 0); }
	`)
	if got := rgba(styles[p.ID].TextDecorationColor); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("shorthand after the longhand = %v, want {255 0 0 255} (Chromium: rgb(255,0,0))", got)
	}

	styles, p, _ = decoDoc(t, `
		#p1 { text-decoration: underline solid rgb(255, 0, 0); text-decoration-color: rgb(0, 0, 255); }
	`)
	if got := rgba(styles[p.ID].TextDecorationColor); got != [4]uint8{0, 0, 255, 255} {
		t.Errorf("longhand after the shorthand = %v, want {0 0 255 255} (Chromium: rgb(0,0,255))", got)
	}

	styles, p, _ = decoDoc(t, `
		#p1 { text-decoration: underline solid rgb(255, 0, 0); text-decoration: underline; }
	`)
	if a := styles[p.ID].TextDecorationColor.A; a != 0 {
		t.Errorf("bare shorthand kept alpha %d, want the auto sentinel (A == 0)", a)
	}
}

// qc:BR-324
func TestTextDecorationShorthandResetsThicknessAndLeavesTheOffset(t *testing.T) {
	styles, p, _ := decoDoc(t, `
		#p1 { text-decoration-thickness: 3px; text-decoration: underline solid rgb(255, 0, 0); }
	`)
	cs := styles[p.ID]
	if cs.TextDecorationThickness != 0 {
		t.Errorf("thickness after the shorthand = %v, want the auto sentinel 0 (Chromium computes auto here)", cs.TextDecorationThickness)
	}
	if got := rgba(cs.TextDecorationColor); got != [4]uint8{255, 0, 0, 255} {
		t.Errorf("shorthand colour = %v while resetting thickness, want {255 0 0 255}", got)
	}

	styles, p, _ = decoDoc(t, `
		#p1 { text-decoration: underline solid rgb(255, 0, 0); text-decoration-thickness: 3px; }
	`)
	if th := styles[p.ID].TextDecorationThickness; th != 3 {
		t.Errorf("longhand after the shorthand = %v, want 3 (Chromium computes 3px here and inks one saturated row at dy=+17)", th)
	}

	styles, p, _ = decoDoc(t, `
		#p1 { text-underline-offset: 2px; text-decoration: underline solid rgb(255, 0, 0); }
	`)
	if off := styles[p.ID].TextUnderlineOffset; off != 2 {
		t.Errorf("offset after the shorthand = %v, want 2 kept: the reference does not reset text-underline-offset (computed off=2px, bar at dy=+17)", off)
	}
}

// qc:BR-324
func TestTextDecorationColorInheritsWithTheDecorationFlags(t *testing.T) {
	styles, p, span := decoDoc(t, `
		#p1 { text-decoration: underline solid rgb(200, 0, 200); }
	`)
	parent, child := styles[p.ID], styles[span.ID]
	if got := rgba(child.TextDecorationColor); got != [4]uint8{200, 0, 200, 255} {
		t.Errorf("child inherited %v, want the parent's {200 0 200 255}", got)
	}
	if parent.TextDecorationColor != child.TextDecorationColor {
		t.Errorf("parent %v and child %v differ", parent.TextDecorationColor, child.TextDecorationColor)
	}
}

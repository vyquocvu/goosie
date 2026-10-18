package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestInsetShorthand(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	b1 := doc.NewElement("div")
	b1.SetAttribute("id", "b1")
	body.AppendChild(b1)

	b2 := doc.NewElement("div")
	b2.SetAttribute("id", "b2")
	body.AppendChild(b2)

	b3 := doc.NewElement("div")
	b3.SetAttribute("id", "b3")
	body.AppendChild(b3)

	sheet := css.Parse(`
		#b1 {
			position: absolute;
			inset: 10px 20px 30px 40px;
		}
		#b2 {
			position: absolute;
			inset: 5px auto;
		}
		#b3 {
			position: absolute;
			direction: rtl;
			inset-inline: 15px 25px;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[b1.ID]
	if s1.Top != 10 || s1.Right != 20 || s1.Bottom != 30 || s1.Left != 40 {
		t.Errorf("b1 Inset: want (10, 20, 30, 40), got (%v, %v, %v, %v)", s1.Top, s1.Right, s1.Bottom, s1.Left)
	}
	if !s1.HasTop || !s1.HasRight || !s1.HasBottom || !s1.HasLeft {
		t.Errorf("b1 HasInsets: want all true, got (%v, %v, %v, %v)", s1.HasTop, s1.HasRight, s1.HasBottom, s1.HasLeft)
	}

	s2 := styles[b2.ID]
	if s2.Top != 5 || s2.Bottom != 5 {
		t.Errorf("b2 Top/Bottom: want 5, got T=%v B=%v", s2.Top, s2.Bottom)
	}
	if !s2.HasTop || !s2.HasBottom {
		t.Errorf("b2 HasTop/HasBottom: want true, got T=%v B=%v", s2.HasTop, s2.HasBottom)
	}
	if s2.HasRight || s2.HasLeft {
		t.Errorf("b2 HasRight/HasLeft: want false for auto, got R=%v L=%v", s2.HasRight, s2.HasLeft)
	}

	s3 := styles[b3.ID]
	// In RTL, start is Right and end is Left
	if s3.Right != 15 || s3.Left != 25 {
		t.Errorf("b3 RTL InsetInline: want Right=15 Left=25, got Right=%v Left=%v", s3.Right, s3.Left)
	}
}

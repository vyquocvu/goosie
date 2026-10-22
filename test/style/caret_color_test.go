package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestCaretColorCascade(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	parent := doc.NewElement("div")
	parent.SetAttribute("id", "p")
	body.AppendChild(parent)

	input1 := doc.NewElement("input")
	input1.SetAttribute("id", "i1")
	parent.AppendChild(input1)

	input2 := doc.NewElement("input")
	input2.SetAttribute("id", "i2")
	parent.AppendChild(input2)

	sheet := css.Parse(`
		#p { caret-color: rgb(255, 0, 0); }
		#i1 { }
		#i2 { caret-color: auto; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	pStyle := styles[parent.ID]
	if pStyle == nil || pStyle.CaretColor.R != 255 || pStyle.CaretColor.A != 255 {
		t.Fatalf("parent CaretColor = %+v, want rgb(255, 0, 0)", pStyle.CaretColor)
	}

	i1Style := styles[input1.ID]
	if i1Style == nil || i1Style.CaretColor.R != 255 || i1Style.CaretColor.A != 255 {
		t.Fatalf("input1 inherited CaretColor = %+v, want rgb(255, 0, 0)", i1Style.CaretColor)
	}

	i2Style := styles[input2.ID]
	if i2Style == nil || i2Style.CaretColor.A != 0 {
		t.Fatalf("input2 auto CaretColor = %+v, want zero alpha", i2Style.CaretColor)
	}
}

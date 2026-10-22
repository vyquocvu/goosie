package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestLogicalBorderRadius(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	ltrBox := doc.NewElement("div")
	ltrBox.SetAttribute("id", "ltrBox")
	body.AppendChild(ltrBox)

	rtlBox := doc.NewElement("div")
	rtlBox.SetAttribute("id", "rtlBox")
	body.AppendChild(rtlBox)

	sheet := css.Parse(`
		#ltrBox {
			direction: ltr;
			border-start-start-radius: 10px;
			border-start-end-radius: 20px;
			border-end-end-radius: 30px;
			border-end-start-radius: 40px;
		}
		#rtlBox {
			direction: rtl;
			border-start-start-radius: 10px;
			border-start-end-radius: 20px;
			border-end-end-radius: 30px;
			border-end-start-radius: 40px;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	// In LTR:
	// start-start = top-left (0) = 10
	// start-end   = top-right (1) = 20
	// end-end     = bottom-right (2) = 30
	// end-start   = bottom-left (3) = 40
	sLTR := styles[ltrBox.ID]
	if sLTR.BorderRadius[0][0] != 10 || sLTR.BorderRadius[1][0] != 20 || sLTR.BorderRadius[2][0] != 30 || sLTR.BorderRadius[3][0] != 40 {
		t.Errorf("LTR BorderRadius want [10, 20, 30, 40], got [%v, %v, %v, %v]",
			sLTR.BorderRadius[0][0], sLTR.BorderRadius[1][0], sLTR.BorderRadius[2][0], sLTR.BorderRadius[3][0])
	}

	// In RTL:
	// start-start = top-right (1) = 10
	// start-end   = top-left (0) = 20
	// end-end     = bottom-left (3) = 30
	// end-start   = bottom-right (2) = 40
	sRTL := styles[rtlBox.ID]
	if sRTL.BorderRadius[0][0] != 20 || sRTL.BorderRadius[1][0] != 10 || sRTL.BorderRadius[2][0] != 40 || sRTL.BorderRadius[3][0] != 30 {
		t.Errorf("RTL BorderRadius want [20, 10, 40, 30], got [%v, %v, %v, %v]",
			sRTL.BorderRadius[0][0], sRTL.BorderRadius[1][0], sRTL.BorderRadius[2][0], sRTL.BorderRadius[3][0])
	}
}

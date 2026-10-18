package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestBackgroundClipAndWebkitAlias(t *testing.T) {
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

	sheet := css.Parse(`
		#b1 {
			background-clip: text;
		}
		#b2 {
			-webkit-background-clip: text;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[b1.ID]
	if s1.BackgroundClip != style.BgBoxText {
		t.Errorf("b1 BackgroundClip: want BgBoxText, got %v", s1.BackgroundClip)
	}

	s2 := styles[b2.ID]
	if s2.BackgroundClip != style.BgBoxText {
		t.Errorf("b2 -webkit-background-clip: want BgBoxText, got %v", s2.BackgroundClip)
	}
}

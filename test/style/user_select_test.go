package style_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestUserSelectCascade(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	parent := doc.NewElement("div")
	parent.SetAttribute("id", "p")
	body.AppendChild(parent)

	c1 := doc.NewElement("div")
	c1.SetAttribute("id", "c1")
	parent.AppendChild(c1)

	c2 := doc.NewElement("div")
	c2.SetAttribute("id", "c2")
	parent.AppendChild(c2)

	w := doc.NewElement("div")
	w.SetAttribute("id", "w")
	body.AppendChild(w)

	sheet := css.Parse(`
		#p { user-select: none; }
		#c1 { }
		#c2 { user-select: text; }
		#w { -webkit-user-select: all; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	pStyle := styles[parent.ID]
	if pStyle == nil || pStyle.UserSelect != "none" {
		t.Fatalf("parent UserSelect = %q, want none", pStyle.UserSelect)
	}

	c1Style := styles[c1.ID]
	if c1Style == nil || c1Style.UserSelect != "none" {
		t.Fatalf("child1 inherited UserSelect = %q, want none", c1Style.UserSelect)
	}

	c2Style := styles[c2.ID]
	if c2Style == nil || c2Style.UserSelect != "text" {
		t.Fatalf("child2 overridden UserSelect = %q, want text", c2Style.UserSelect)
	}

	wStyle := styles[w.ID]
	if wStyle == nil || wStyle.UserSelect != "all" {
		t.Fatalf("-webkit-user-select = %q, want all", wStyle.UserSelect)
	}
}

func TestUserSelectSelectionExclusion(t *testing.T) {
	// Text in an unselectable container should not be detected as selectable text
	html := `<html><body style="margin: 0;">
<p style="user-select: none;">Unselectable paragraph text</p>
<p style="user-select: text;">Selectable paragraph text</p>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	sess.SelectAll()
	selected := sess.SelectionText()
	if strings.Contains(selected, "Unselectable") {
		t.Fatalf("SelectionText() contains unselectable text: %q", selected)
	}
	if !strings.Contains(selected, "Selectable") {
		t.Fatalf("SelectionText() expected to contain selectable text: %q", selected)
	}
}

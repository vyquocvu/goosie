package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestUnicodeBidiRoundTrip(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	mk := func(id string) *dom.Node {
		el := doc.NewElement("div")
		el.SetAttribute("id", id)
		body.AppendChild(el)
		return el
	}
	d1 := mk("d1")
	d2 := mk("d2")
	d3 := mk("d3")
	d4 := mk("d4")
	d5 := mk("d5")
	d6 := mk("d6")
	d7 := mk("d7")

	sheet := css.Parse(`
		#d1 { unicode-bidi: embed; }
		#d2 { unicode-bidi: isolate; }
		#d3 { unicode-bidi: bidi-override; }
		#d4 { unicode-bidi: plaintext; }
		#d5 { unicode-bidi: normal; }
		#d6 { unicode-bidi: bogus; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	cases := []struct {
		node *dom.Node
		want string
	}{
		{d1, "embed"},
		{d2, "isolate"},
		{d3, "bidi-override"},
		{d4, "plaintext"},
		{d5, "normal"},
		// An unrecognized keyword leaves the declaration ineffective and the
		// computed value at its initial empty form, as does no declaration
		// at all (d7).
		{d6, ""},
		{d7, ""},
	}
	for _, c := range cases {
		s := styles[c.node.ID]
		if s == nil {
			t.Fatalf("node %d has no computed style", c.node.ID)
		}
		if s.UnicodeBidi != c.want {
			t.Errorf("UnicodeBidi = %q, want %q", s.UnicodeBidi, c.want)
		}
	}

	if def := style.DefaultStyle().UnicodeBidi; def != "" {
		t.Errorf("DefaultStyle().UnicodeBidi = %q, want empty (initial normal)", def)
	}
}

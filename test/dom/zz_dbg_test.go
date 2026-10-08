package dom_test

import (
	"os"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestDbgClipBodyDOM(t *testing.T) {
	raw, _ := os.ReadFile("/tmp/goosie-wpt/css/css-backgrounds/background-clip/clip-border-area-on-body-propagated-to-root.html")
	doc := domtest.Parse(string(raw))
	t.Logf("HTML=%v Head=%v Body=%v", doc.HTML != nil, doc.Head != nil, doc.Body != nil)
	var walk func(n *dom.Node, d int)
	walk = func(n *dom.Node, d int) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			t.Logf("%*s%s", d*2, "", c.Data)
			walk(c, d+1)
		}
	}
	_ = walk
	_ = dom.NewDocument
}

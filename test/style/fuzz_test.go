package style_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

// cascadeLimits are loose enough that a fuzzing input is almost always a document
// rather than a refusal, which is what the matcher needs to see.
var cascadeLimits = dom.ParseLimits{Nodes: 128, Depth: 16, Attributes: 8, AttributeBytes: 256}

// FuzzResolveCascade drives the selector matcher with an arbitrary tree and two
// arbitrary stylesheets, and requires that resolving [UA, a] is unchanged by an
// intervening resolve of [UA, b] against the same document. That second property
// is the reason this target exists: the rule index dedupes a query with a stamp
// counter, and a stamp that outlived its index would show up here and nowhere else.
func FuzzResolveCascade(f *testing.F) {
	f.Add(`<html><body><p class="a" id="b">text`, `p.a#b { color: red }`, `* { margin: 0 }`)
	f.Add(`<div><span>x</span></div>`, `@media (min-width: 100px){ div > span:hover::before{content:""} }`, `:not(p) em { color: rgb(1,2,3) }`)
	f.Add(`<svg><rect></svg><table><tr><td>`, `tr td:nth-child(2n+1) { background: #fff }`, `[lang|=en], a[href^="http"] { font-size: 1em }`)
	f.Add(`<<<p>>>`+"\x00", `p{`, `}}}}{{`)
	f.Fuzz(func(t *testing.T, html, a, b string) {
		doc, err := dom.ParseBounded(html, cascadeLimits)
		if err != nil {
			return
		}
		ua := style.UserAgentStylesheet()
		first := resolve(doc, ua, a)
		resolve(doc, ua, b)
		again := resolve(doc, ua, a)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("resolving %q changed after an intervening resolve of %q", a, b)
		}
		var walk func(n *dom.Node)
		walk = func(n *dom.Node) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == dom.NodeElement {
					if first[c.ID] == nil {
						t.Errorf("element %q got no computed style", c.Data)
					}
				}
				walk(c)
			}
		}
		walk(&doc.Node)
	})
}

func resolve(doc *dom.Document, ua *css.Stylesheet, src string) map[dom.NodeID]*style.ComputedStyle {
	sheet := css.ParseForViewport(src, 1440)
	return style.ResolveViewport(doc, []*css.Stylesheet{ua, sheet}, style.Viewport{W: 1440, H: 900}, nil)
}

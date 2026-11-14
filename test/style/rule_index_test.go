package style_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func computedFor(t *testing.T, html, sheet, tag string) *style.ComputedStyle {
	t.Helper()
	doc := domtest.Parse(html)
	styles := style.Resolve(doc, []*css.Stylesheet{css.Parse(sheet)}, nil)
	var found *style.ComputedStyle
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n; c != nil && found == nil; c = c.NextSibling {
			if c.Element() && c.Data == tag {
				if s, ok := styles[c.ID]; ok {
					found = s
					return
				}
			}
			walk(c.FirstChild)
		}
	}
	walk(&doc.Node)
	if found == nil {
		t.Fatalf("no <%s> styled in %s", tag, html)
	}
	return found
}

func col(r, g, b, a uint8) css.Color { return css.Color{R: r, G: g, B: b, A: a} }

func TestRuleIndexPreservesCascadeOrder(t *testing.T) {
	t.Run("equal specificity keeps document order", func(t *testing.T) {
		got := computedFor(t, `<p class="y x">t</p>`, `.x{color:red}.y{color:green}`, "p")
		if got.Color != col(0, 128, 0, 255) {
			t.Fatalf(".y declared last should win, got %v", got.Color)
		}
	})
	t.Run("multi-class key applies once", func(t *testing.T) {
		got := computedFor(t, `<p class="a b">t</p>`, `.a.b{color:blue;padding-top:1px}`, "p")
		if got.Color != col(0, 0, 255, 255) {
			t.Fatalf("want blue, got %v", got.Color)
		}
		if got.PaddingTop != 1 {
			t.Fatalf("want padding 1px, got %v", got.PaddingTop)
		}
	})
	t.Run("descendant keyed on descendant", func(t *testing.T) {
		got := computedFor(t, `<div class="wrap"><p>t</p></div>`, `.wrap p{color:teal}`, "p")
		if got.Color != col(0, 128, 128, 255) {
			t.Fatalf("want teal, got %v", got.Color)
		}
		if wrong := computedFor(t, `<p>t</p>`, `.wrap p{color:teal}`, "p"); wrong.Color == col(0, 128, 128, 255) {
			t.Fatal("unrelated p must not match .wrap p")
		}
	})
	t.Run("id key with descendant", func(t *testing.T) {
		got := computedFor(t, `<div id="main"><span class="item">t</span></div>`, `#main .item{color:olive}`, "span")
		if got.Color != col(128, 128, 0, 255) {
			t.Fatalf("want olive, got %v", got.Color)
		}
	})
	t.Run("unkeyed selectors", func(t *testing.T) {
		got := computedFor(t, `<p data-x="1">t</p>`, `*{color:navy}[data-x]{padding-top:3px}`, "p")
		if got.Color != col(0, 0, 128, 255) {
			t.Fatalf("want navy from *, got %v", got.Color)
		}
		if got.PaddingTop != 3 {
			t.Fatalf("want padding 3px, got %v", got.PaddingTop)
		}
	})
	t.Run("missing key class does not match", func(t *testing.T) {
		got := computedFor(t, `<p class="a">t</p>`, `.a.b{color:red}`, "p")
		if got.Color == col(255, 0, 0, 255) {
			t.Fatal(".a.b must not match class=a")
		}
	})
}

func TestCustomPropertyExpansion(t *testing.T) {
	t.Run("chained definitions resolve", func(t *testing.T) {
		sheet := `p{--a:20px; --b:var(--a); --c:var(--b); --d:var(--c); padding-top:var(--d)}`
		if got := computedFor(t, `<p>t</p>`, sheet, "p").PaddingTop; got != 20 {
			t.Fatalf("want 20px through a 4-deep chain, got %v", got)
		}
	})
	t.Run("self reference falls back", func(t *testing.T) {
		sheet := `p{--a:var(--a); padding-top:var(--a, 9px)}`
		if got := computedFor(t, `<p>t</p>`, sheet, "p").PaddingTop; got != 9 {
			t.Fatalf("want the 9px fallback, got %v", got)
		}
	})
	t.Run("mutual reference terminates", func(t *testing.T) {
		sheet := `p{--a:var(--b); --b:var(--a); padding-top:var(--a, 7px)}`
		if got := computedFor(t, `<p>t</p>`, sheet, "p").PaddingTop; got != 7 {
			t.Fatalf("want the 7px fallback, got %v", got)
		}
	})
	t.Run("fan-out stays cheap", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("p{--base:5px;")
		for i := 0; i < 40; i++ {
			b.WriteString("padding-left:var(--base);margin-left:var(--base);")
		}
		b.WriteString("}")
		got := computedFor(t, `<p>t</p>`, b.String(), "p")
		if got.PaddingLeft != 5 || got.MarginLeft != 5 {
			t.Fatalf("want 5px, got pad=%v margin=%v", got.PaddingLeft, got.MarginLeft)
		}
	})
}

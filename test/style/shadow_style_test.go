package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func findDescendantByID(n *dom.Node, id string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.GetAttribute("id") == id {
		return n
	}
	if n.ShadowRoot != nil {
		for sc := n.ShadowRoot.FirstChild; sc != nil; sc = sc.NextSibling {
			if found := findDescendantByID(sc, id); found != nil {
				return found
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findDescendantByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func TestShadowStyle_ScopedStylesheet(t *testing.T) {
	// Styles declared inside <style> in shadow root apply to shadow tree elements
	html := `<!DOCTYPE html>
<html>
<head>
  <style>
    .title { color: #0000ff; } /* Document style: blue */
  </style>
</head>
<body>
  <div id="outer" class="title">Outer Title</div>
  <custom-element id="host">
    <template shadowrootmode="open">
      <style>
        .title { color: #ff0000; } /* Scoped shadow style: red */
      </style>
      <div id="inner" class="title">Shadow Title</div>
    </template>
  </custom-element>
</body>
</html>`

	doc := domtest.Parse(html)
	// Collect document stylesheets
	var docSheets []*css.Stylesheet
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Data == "html" {
			for hc := c.FirstChild; hc != nil; hc = hc.NextSibling {
				if hc.Data == "head" {
					for sc := hc.FirstChild; sc != nil; sc = sc.NextSibling {
						if sc.Element() && sc.Data == "style" {
							docSheets = append(docSheets, css.Parse(sc.TextContent()))
						}
					}
				}
			}
		}
	}

	vp := style.Viewport{W: 1280, H: 800}
	styles := style.ResolveViewport(doc, docSheets, vp, nil)

	outer := findDescendantByID(&doc.Node, "outer")
	inner := findDescendantByID(&doc.Node, "inner")

	if outer == nil || inner == nil {
		t.Fatal("outer or inner element not found")
	}

	outerStyle := styles[outer.ID]
	innerStyle := styles[inner.ID]

	if outerStyle == nil || innerStyle == nil {
		t.Fatal("styles not computed for outer or inner")
	}

	// Outer should be blue (#0000ff)
	if outerStyle.Color.B != 255 || outerStyle.Color.R != 0 {
		t.Fatalf("expected outer to be blue, got rgba(%d,%d,%d,%d)",
			outerStyle.Color.R, outerStyle.Color.G, outerStyle.Color.B, outerStyle.Color.A)
	}

	// Inner should be red (#ff0000) from scoped shadow stylesheet
	if innerStyle.Color.R != 255 || innerStyle.Color.B != 0 {
		t.Fatalf("expected inner to be red from scoped shadow style, got rgba(%d,%d,%d,%d)",
			innerStyle.Color.R, innerStyle.Color.G, innerStyle.Color.B, innerStyle.Color.A)
	}
}

func TestShadowStyle_HostSelector(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <custom-btn id="btn1" class="active">
    <template shadowrootmode="open">
      <style>
        :host { display: block; background-color: #00ff00; }
        :host(.active) { background-color: #ff00ff; }
      </style>
      <span>Button</span>
    </template>
  </custom-btn>

  <custom-btn id="btn2">
    <template shadowrootmode="open">
      <style>
        :host { display: block; background-color: #00ff00; }
        :host(.active) { background-color: #ff00ff; }
      </style>
      <span>Button</span>
    </template>
  </custom-btn>
</body>
</html>`

	doc := domtest.Parse(html)
	vp := style.Viewport{W: 1280, H: 800}
	styles := style.ResolveViewport(doc, nil, vp, nil)

	btn1 := findDescendantByID(&doc.Node, "btn1")
	btn2 := findDescendantByID(&doc.Node, "btn2")

	if btn1 == nil || btn2 == nil {
		t.Fatal("btn1 or btn2 not found")
	}

	s1 := styles[btn1.ID]
	s2 := styles[btn2.ID]

	if s1 == nil || s2 == nil {
		t.Fatal("styles not resolved for buttons")
	}

	// btn1 has .active -> should match :host(.active) -> magenta (#ff00ff: R=255, G=0, B=255)
	if s1.BackgroundColor.R != 255 || s1.BackgroundColor.G != 0 || s1.BackgroundColor.B != 255 {
		t.Fatalf("expected btn1 to be magenta, got rgba(%d,%d,%d,%d)",
			s1.BackgroundColor.R, s1.BackgroundColor.G, s1.BackgroundColor.B, s1.BackgroundColor.A)
	}

	// btn2 lacks .active -> matches :host -> green (#00ff00: R=0, G=255, B=0)
	if s2.BackgroundColor.R != 0 || s2.BackgroundColor.G != 255 || s2.BackgroundColor.B != 0 {
		t.Fatalf("expected btn2 to be green, got rgba(%d,%d,%d,%d)",
			s2.BackgroundColor.R, s2.BackgroundColor.G, s2.BackgroundColor.B, s2.BackgroundColor.A)
	}
}

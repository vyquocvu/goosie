package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func findNodeByID(n *dom.Node, id string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.GetAttribute("id") == id {
		return n
	}
	if n.ShadowRoot != nil {
		for sc := n.ShadowRoot.FirstChild; sc != nil; sc = sc.NextSibling {
			if found := findNodeByID(sc, id); found != nil {
				return found
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNodeByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func buildTestArena(html string, cssText string) (*dom.Document, *layout.Arena) {
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	if cssText != "" {
		sheets = append(sheets, css.Parse(cssText))
	}
	vp := style.Viewport{W: 1280, H: 800}
	styles := style.ResolveViewport(doc, sheets, vp, nil)
	arena := layout.Build(doc, styles, nil)
	return doc, arena
}

func TestShadowLayout_DefaultSlotProjection(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <custom-card id="host">
    <template shadowrootmode="open">
      <div id="container">
        <slot></slot>
      </div>
    </template>
    <div id="slotted">Slotted Content</div>
  </custom-card>
</body>
</html>`

	doc, arena := buildTestArena(html, "")
	container := findNodeByID(&doc.Node, "container")
	slotted := findNodeByID(&doc.Node, "slotted")

	if container == nil || slotted == nil {
		t.Fatal("container or slotted node not found")
	}

	containerID, ok := arena.ForNode(container.ID)
	if !ok {
		t.Fatal("no arena object for container")
	}
	slottedID, ok := arena.ForNode(slotted.ID)
	if !ok {
		t.Fatal("no arena object for slotted")
	}

	// Verify that slotted's parent in the layout arena is container!
	slottedObj := arena.Get(slottedID)
	if slottedObj == nil {
		t.Fatal("nil slotted object")
	}
	if slottedObj.Parent != containerID {
		t.Fatalf("expected slotted parent object to be containerID (%d), got %d", containerID, slottedObj.Parent)
	}
}

func TestShadowLayout_NamedSlotProjection(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <modal-dialog id="modal">
    <template shadowrootmode="open">
      <header id="hdr">
        <slot name="title"></slot>
      </header>
      <section id="bdy">
        <slot></slot>
      </section>
    </template>
    <h2 id="heading" slot="title">Dialog Title</h2>
    <p id="paragraph">Dialog description body</p>
  </modal-dialog>
</body>
</html>`

	doc, arena := buildTestArena(html, "")
	hdr := findNodeByID(&doc.Node, "hdr")
	bdy := findNodeByID(&doc.Node, "bdy")
	heading := findNodeByID(&doc.Node, "heading")
	paragraph := findNodeByID(&doc.Node, "paragraph")

	if hdr == nil || bdy == nil || heading == nil || paragraph == nil {
		t.Fatal("nodes not found in document")
	}

	hdrID, _ := arena.ForNode(hdr.ID)
	bdyID, _ := arena.ForNode(bdy.ID)
	headingID, _ := arena.ForNode(heading.ID)
	paraID, _ := arena.ForNode(paragraph.ID)

	headingObj := arena.Get(headingID)
	if headingObj.Parent != hdrID {
		t.Fatalf("expected heading's layout parent to be header (%d), got %d", hdrID, headingObj.Parent)
	}

	paraObj := arena.Get(paraID)
	if paraObj.Parent != bdyID {
		t.Fatalf("expected paragraph's layout parent to be section body (%d), got %d", bdyID, paraObj.Parent)
	}
}

func TestShadowLayout_SlotFallbackContent(t *testing.T) {
	// When host has no slotted children, slot fallback content is rendered into the arena
	html := `<!DOCTYPE html>
<html>
<body>
  <empty-host id="host">
    <template shadowrootmode="open">
      <div id="wrapper">
        <slot>
          <span id="fallback">Default Fallback Text</span>
        </slot>
      </div>
    </template>
  </empty-host>
</body>
</html>`

	doc, arena := buildTestArena(html, "")
	wrapper := findNodeByID(&doc.Node, "wrapper")
	fallback := findNodeByID(&doc.Node, "fallback")

	if wrapper == nil || fallback == nil {
		t.Fatal("wrapper or fallback node not found")
	}

	wrapperID, _ := arena.ForNode(wrapper.ID)
	fallbackID, ok := arena.ForNode(fallback.ID)
	if !ok {
		t.Fatal("expected fallback node to have an arena object when no light children are assigned")
	}

	fallbackObj := arena.Get(fallbackID)
	if fallbackObj.Parent != wrapperID {
		t.Fatalf("expected fallback parent object to be wrapperID (%d), got %d", wrapperID, fallbackObj.Parent)
	}
}

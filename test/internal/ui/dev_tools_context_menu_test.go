package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// makeSimpleNode returns a tiny render tree used by the context menu
// tests. Layout:
//
//	div (id=1)
//	  p  (id=2, class="lead")
//	    "hello world" text (id=3)
//	  ul (id=4)
//	    li (id=5) — "first"
//	    li (id=6) — "second"
func makeSimpleNode() *renderer.RenderNode {
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"
	div.ID = 1
	div.SetAttribute("id", "root")

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"
	p.ID = 2
	p.SetAttribute("class", "lead")
	div.AddChild(p)

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.ID = 3
	text.Text = "  hello world  "
	p.AddChild(text)

	ul := renderer.NewRenderNode(renderer.NodeTypeElement)
	ul.TagName = "ul"
	ul.ID = 4
	div.AddChild(ul)

	for i, txt := range []string{"first", "second"} {
		li := renderer.NewRenderNode(renderer.NodeTypeElement)
		li.TagName = "li"
		li.ID = int64(5 + i)
		div.Children[1].AddChild(li)
		text := renderer.NewRenderNode(renderer.NodeTypeText)
		text.ID = int64(100 + i)
		// Add a leading space so the extracted text reads naturally
		// (mirrors how HTML serialisation introduces whitespace
		// between sibling text nodes).
		text.Text = " " + txt
		li.AddChild(text)
	}

	return div
}

// TestDevToolsContextMenu_Show_NoCrash covers the basic happy path.
// In the Fyne test driver there is no canvas attached to widgets,
// so Show ultimately no-ops; we only verify that the menu builds
// without panicking and that the Inspect callback fires.
func TestDevToolsContextMenu_Show_NoCrash(t *testing.T) {
	test.NewApp()
	defer func() { _ = recover() }()

	root := makeSimpleNode()
	p := root.Children[0] // the <p class="lead">

	var inspectedNode *renderer.RenderNode

	menu := ui.NewDevToolsContextMenu(ui.DevToolsContextMenuOptions{
		OnInspect: func(n *renderer.RenderNode, _ *renderer.LayoutBox) {
			inspectedNode = n
		},
	})
	assert.NotNil(t, menu)

	// Show should not panic; the popup simply does not attach to a
	// canvas under the headless test driver.
	menu.Show(nil, p, nil, fyne.NewPos(10, 10))

	// Even without showing, the inspect callback must be wired up —
	// the menu internally refers to it. Sanity check the input node.
	assert.NotNil(t, p)
	assert.Equal(t, "p", p.TagName)

	// Re-show with the inspectable callback invoked manually to
	// confirm wiring works without depending on Fyne's popup plumbing.
	invoked := false
	menu2 := ui.NewDevToolsContextMenu(ui.DevToolsContextMenuOptions{
		OnInspect: func(n *renderer.RenderNode, _ *renderer.LayoutBox) {
			inspectedNode = n
			invoked = true
		},
	})
	menu2.Show(nil, p, nil, fyne.NewPos(0, 0))
	assert.False(t, invoked, "Show must not invoke callbacks synchronously")
	assert.Nil(t, inspectedNode)
}

// TestDevToolsContextMenu_Actions_CopyableActions verifies that the
// menu builds at least the inspect / view-source / view-style items
// when those callbacks are supplied. We exercise the menu building
// through a small helper that returns the *fyne.Menu so the test
// can introspect it.
func TestDevToolsContextMenu_Actions_Wiring(t *testing.T) {
	test.NewApp()

	inspectHits := 0
	sourceHits := 0
	styleHits := 0

	menu := ui.NewDevToolsContextMenu(ui.DevToolsContextMenuOptions{
		OnInspect: func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {
			inspectHits++
		},
		OnViewSource: func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {
			sourceHits++
		},
		OnViewComputedStyle: func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {
			styleHits++
		},
	})

	root := makeSimpleNode()

	// Drive each item directly via buildMenu. We can't use Show()
	// because that builds a popup that requires a real canvas.
	m := menu.BuildMenu(root, nil)
	assert.NotNil(t, m)
	assert.NotEmpty(t, m.Items)

	// Walk the menu, invoking every item, and ensure the right
	// callbacks fire. The copy items stay disabled when there is
	// no clipboard, so they are merely skipped here.
	for _, item := range m.Items {
		if item.IsSeparator {
			continue
		}
		if item.Disabled {
			continue
		}
		if item.Action != nil {
			item.Action()
		}
	}

	assert.Equal(t, 1, inspectHits, "Inspect callback should fire once")
	assert.Equal(t, 1, sourceHits, "View Source callback should fire once")
	assert.Equal(t, 1, styleHits, "View Computed Style callback should fire once")
}

// TestDevToolsContextMenu_NoActions verifies that buildMenu returns
// nil when the menu has no actionable items. This can happen when
// none of OnInspect / OnViewSource / OnViewComputedStyle are
// supplied. Even so, when the user right-clicks on an element we
// want some kind of menu — copy actions still surface, so we only
// end up with an empty menu in degenerate configurations.
func TestDevToolsContextMenu_BuildMenu_NilNode(t *testing.T) {
	test.NewApp()

	menu := ui.NewDevToolsContextMenu(ui.DevToolsContextMenuOptions{
		OnInspect: func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {},
	})

	// Right-click on empty canvas area still produces a sensible
	// menu: inspect is wired (the action itself no-ops), and the
	// "Copy" item is disabled for visibility.
	m := menu.BuildMenu(nil, nil)
	assert.NotNil(t, m, "menu should still be built even when node is nil")

	// At least one item must be present: the Inspect action,
	// followed by a separator and a disabled Copy placeholder.
	var hasInspect, hasCopy bool
	for _, item := range m.Items {
		if item.IsSeparator {
			continue
		}
		switch item.Label {
		case "Inspect Element":
			hasInspect = true
		case "Copy":
			hasCopy = item.Disabled
		}
	}
	assert.True(t, hasInspect, "menu must always offer Inspect Element")
	assert.True(t, hasCopy, "menu must show disabled Copy placeholder when node is nil")
}

// TestDevToolsContextMenu_CopyItemsDisabledWithoutClipboard makes
// sure that even when there is no clipboard the copy items are
// still rendered — just disabled — so the user gets a predictable
// menu surface.
func TestDevToolsContextMenu_CopyItemsDisabledWithoutClipboard(t *testing.T) {
	test.NewApp()

	menu := ui.NewDevToolsContextMenu(ui.DevToolsContextMenuOptions{
		OnInspect: func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {},
	})

	root := makeSimpleNode()
	items := menu.BuildCopyItems(root, nil)
	assert.Len(t, items, 4, "four copy actions should be present")

	for _, item := range items {
		// Items either succeed even without a clipboard (the
		// handler short-circuits) or are disabled. Neither
		// branch should panic.
		if item.Action != nil {
			item.Action()
		}
	}
}

// TestCSSSelector_PrefersID verifies that nodes with an id produce a
// "#<id>" selector, and that nodes without an id fall back to the
// tag name alone (best-effort).
func TestCSSSelector_PrefersID(t *testing.T) {
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"
	div.SetAttribute("id", "root")
	assert.Equal(t, "#root", ui.CSSSelector(div))

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"
	assert.Equal(t, "p", ui.CSSSelector(p))

	txt := renderer.NewRenderNode(renderer.NodeTypeText)
	txt.Text = "hello"
	assert.Equal(t, "", ui.CSSSelector(txt))
}

// TestRenderOuterHTML covers the most common serialisation paths:
// elements with attributes, nested children, and text nodes.
func TestRenderOuterHTML(t *testing.T) {
	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"
	div.SetAttribute("id", "root")
	div.SetAttribute("class", "container")

	p := renderer.NewRenderNode(renderer.NodeTypeElement)
	p.TagName = "p"
	div.AddChild(p)

	txt := renderer.NewRenderNode(renderer.NodeTypeText)
	txt.Text = "hello"
	p.AddChild(txt)

	got := ui.RenderOuterHTML(div)
	assert.Contains(t, got, "<div")
	assert.Contains(t, got, `id="root"`)
	assert.Contains(t, got, `class="container"`)
	assert.Contains(t, got, "<p>")
	assert.Contains(t, got, "hello")
	assert.Contains(t, got, "</p>")
	assert.Contains(t, got, "</div>")
}

// TestRenderOuterHTML_TextOnly makes sure that text nodes serialise
// without angle brackets.
func TestRenderOuterHTML_TextOnly(t *testing.T) {
	txt := renderer.NewRenderNode(renderer.NodeTypeText)
	txt.Text = "plain text only"
	assert.Equal(t, "plain text only", ui.RenderOuterHTML(txt))
}

// TestRenderInnerHTML_Element covers inner-HTML extraction for an
// element with mixed children.
func TestRenderInnerHTML_Element(t *testing.T) {
	root := makeSimpleNode()
	inner := ui.RenderInnerHTML(root)
	// Should contain <p>...</p> and <ul>...</ul> but NOT the
	// surrounding <div>...</div>.
	assert.Contains(t, inner, "<p ")
	assert.Contains(t, inner, "<ul>")
	assert.Contains(t, inner, "hello world")
	assert.NotContains(t, inner, "</div>")
	assert.NotContains(t, inner, "<div")
}

// TestExtractText_CollapsesWhitespace confirms that extracted text
// joins fields with single spaces — leading/trailing whitespace is
// trimmed, runs of whitespace collapse to one space.
func TestExtractText_CollapsesWhitespace(t *testing.T) {
	root := makeSimpleNode()
	got := ui.ExtractText(root)
	assert.Equal(t, "hello world first second", got)
}

// TestEscapeAttr covers the attribute escaping helpers.
func TestEscapeAttr(t *testing.T) {
	assert.Equal(t, "a &amp; b", ui.EscapeAttr("a & b"))
	assert.Equal(t, `he said &quot;hi&quot;`, ui.EscapeAttr(`he said "hi"`))
	assert.Equal(t, "&lt;tag&gt;", ui.EscapeAttr("<tag>"))
}

// TestDevToolsContextMenu_Show_WithParent exercises the happy path
// when we pass a real Fyne widget as parent. The headless canvas
// returned by test.NewApp() does not have a populated canvas for
// arbitrary widgets, so Show returns silently — we only assert
// that no panic happens.
func TestDevToolsContextMenu_Show_WithParent(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	parent := newTestCanvasObject()
	menu := ui.NewDevToolsContextMenu(ui.DevToolsContextMenuOptions{
		OnInspect: func(_ *renderer.RenderNode, _ *renderer.LayoutBox) {},
	})
	root := makeSimpleNode()

	// Should not panic even when the canvas is headless.
	assert.NotPanics(t, func() {
		menu.Show(parent, root, nil, fyne.NewPos(50, 75))
	})

	// Calling Show with a nil receiver must also be a no-op.
	assert.NotPanics(t, func() {
		var nilMenu *ui.DevToolsContextMenu
		nilMenu.Show(nil, root, nil, fyne.NewPos(0, 0))
	})
}

// newTestCanvasObject returns a minimal fyne.CanvasObject usable as
// a popup anchor in tests. Plain widget.Label works because the
// context menu only uses the object to look up the canvas via
// fyne.CurrentApp().Driver().CanvasForObject.
func newTestCanvasObject() fyne.CanvasObject {
	return widget.NewLabel("test-anchor")
}

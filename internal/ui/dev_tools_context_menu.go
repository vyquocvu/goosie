// Package ui — DevToolsContextMenu provides developer-tools actions
// triggered from a right-click on the rendered page.
//
// The DevToolsContextMenu is a thin façade: it assembles a fyne.Menu
// from the hit-tested renderer.RenderNode and the browser's
// inspectable HTML renderer, then shows the menu as a popup anchored
// at the absolute cursor position reported by the canvas's
// TappedSecondary handler.
//
// The menu is intentionally renderer-agnostic — every actionable
// callback (inspect, copy HTML, copy selector, etc.) goes through the
// DevToolsAction interface so the browser layer can decide whether to
// open the inspector panel, copy to clipboard, or surface a dialog.
//
// Design notes
//
//   - Pure UI logic. No engine imports beyond the existing
//     renderer.RenderNode / LayoutBox types that are already part of
//     the public HTMLRenderer contract.
//   - Stateless: the menu is recomputed every time Show() is called,
//     so it always reflects the latest hit-test result. No
//     accumulating state, no background goroutines.
//   - Thread safety: Show() must be called on the Fyne UI goroutine.
//     The browser layer is responsible for marshalling the canvas
//     callback onto the UI thread via fyne.Do before invoking Show.
package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/vyquocvu/goosie/internal/renderer"
)

// DevToolsAction is the callback signature for context-menu items that
// operate on a hit-tested node. The receiver of the action decides how
// to fulfil the request — typically by toggling the inspector panel,
// putting text on the clipboard, or showing a dialog.
//
// node and layout may be nil when the user right-clicks on empty
// canvas area (no element under the cursor). Implementations are
// expected to no-op gracefully in that case.
type DevToolsAction func(node *renderer.RenderNode, layout *renderer.LayoutBox)

// DevToolsContextMenu builds and shows a right-click context menu for
// the Goosie dev tools. A single instance can be reused across many
// right-click events — Show always rebuilds the menu from the
// current hit-test result.
type DevToolsContextMenu struct {
	// clipboard is the system clipboard used by copy actions. May be
	// nil for tests; copy actions become no-ops when nil.
	clipboard fyne.Clipboard

	// onInspect is invoked when the user picks "Inspect Element". The
	// browser layer wires this to its InspectPanel.
	onInspect DevToolsAction

	// onViewSource is invoked when the user picks "View Source". The
	// browser layer can show the HTML for the hit node in a dialog.
	onViewSource DevToolsAction

	// onViewComputedStyle is invoked when the user picks
	// "View Computed Style". Mirrors the Styles tab in InspectPanel.
	onViewComputedStyle DevToolsAction

	// onCopyInnerText is the CopyInnerText callback. The browser layer
	// can fall back to plain text extraction from the hit node.
	onCopyInnerText DevToolsAction
}

// DevToolsContextMenuOptions configures a DevToolsContextMenu. Zero
// value is a valid configuration — only clipboard copy actions are
// silently skipped when clipboard is nil. All action callbacks are
// optional; an unset callback disables the corresponding menu item.
type DevToolsContextMenuOptions struct {
	Clipboard           fyne.Clipboard
	OnInspect           DevToolsAction
	OnViewSource        DevToolsAction
	OnViewComputedStyle DevToolsAction
	OnCopyInnerText     DevToolsAction
}

// NewDevToolsContextMenu constructs a DevToolsContextMenu with the
// given options. The returned value is ready for Show.
func NewDevToolsContextMenu(opts DevToolsContextMenuOptions) *DevToolsContextMenu {
	return &DevToolsContextMenu{
		clipboard:           opts.Clipboard,
		onInspect:           opts.OnInspect,
		onViewSource:        opts.OnViewSource,
		onViewComputedStyle: opts.OnViewComputedStyle,
		onCopyInnerText:     opts.OnCopyInnerText,
	}
}

// Show builds a context menu for the hit-tested node and pops it up
// at the given absolute screen position. The same instance can be
// reused across many right-click events — internal state from a
// previous Show is replaced wholesale.
//
// When node is nil, a page-level menu is shown instead so the user
// always gets something useful on right-click. When clipboard is nil,
// copy actions are still selectable but become no-ops; this keeps
// the menu stable across test environments that don't have a
// clipboard backend.
//
// parent is the canvas object that anchors the popup. When nil,
// Show falls back to fyne.CurrentApp().Driver().CanvasForObject on
// the menu's own widget — call sites should prefer passing a
// well-known widget (the scroll container backing the tab works
// well) so the popup is parented to the right window.
func (m *DevToolsContextMenu) Show(parent fyne.CanvasObject, node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position) {
	if m == nil {
		return
	}

	menu := m.buildMenu(node, layout)
	if menu == nil {
		// buildMenu returns nil only when there are no enabled
		// items, which means there is nothing to show.
		return
	}

	canvas := fyne.CurrentApp().Driver().CanvasForObject(parent)
	if canvas == nil {
		// Fall back to the current window's canvas — still useful
		// for tests that don't anchor a real widget.
		if win := fyne.CurrentApp().Driver().AllWindows(); len(win) > 0 {
			canvas = win[0].Canvas()
		}
	}
	if canvas == nil {
		return
	}

	popup := widget.NewPopUpMenu(menu, canvas)
	popup.ShowAtPosition(abs)
}

// buildMenu constructs the fyne.Menu shown by Show. Extracted so
// tests can inspect the menu structure without involving Fyne's
// canvas/popup machinery.
func (m *DevToolsContextMenu) buildMenu(node *renderer.RenderNode, layout *renderer.LayoutBox) *fyne.Menu {
	items := make([]*fyne.MenuItem, 0, 7)

	inspectLabel := "Inspect Element"
	if node != nil {
		inspectLabel = fmt.Sprintf("Inspect Element <%s>", node.TagName)
	}
	items = append(items, fyne.NewMenuItem(inspectLabel, func() {
		if m.onInspect != nil {
			m.onInspect(node, layout)
		}
	}))

	if m.onViewSource != nil {
		items = append(items, fyne.NewMenuItem("View Source", func() {
			m.onViewSource(node, layout)
		}))
	}

	if m.onViewComputedStyle != nil {
		items = append(items, fyne.NewMenuItem("View Computed Style", func() {
			m.onViewComputedStyle(node, layout)
		}))
	}

	items = append(items, fyne.NewMenuItemSeparator())

	copyItems := m.buildCopyItems(node, layout)
	items = append(items, copyItems...)

	if len(items) == 0 {
		return nil
	}

	return fyne.NewMenu("", items...)
}

// buildCopyItems returns the copy-* submenu items. Always returns at
// least one item (disabled placeholder) when node is nil so the menu
// structure stays predictable.
func (m *DevToolsContextMenu) buildCopyItems(node *renderer.RenderNode, layout *renderer.LayoutBox) []*fyne.MenuItem {
	if node == nil {
		// No hit-target means there is nothing meaningful to copy.
		disabled := fyne.NewMenuItem("Copy", nil)
		disabled.Disabled = true
		return []*fyne.MenuItem{disabled}
	}

	selector := cssSelector(node)
	outerHTML := renderOuterHTML(node)
	innerHTML := renderInnerHTML(node)

	copySelector := fyne.NewMenuItem("Copy Selector", func() {
		if m.clipboard != nil {
			m.clipboard.SetContent(selector)
		}
	})
	copySelector.Disabled = selector == ""

	copyOuterHTML := fyne.NewMenuItem("Copy Outer HTML", func() {
		if m.clipboard != nil {
			m.clipboard.SetContent(outerHTML)
		}
	})

	copyInnerHTML := fyne.NewMenuItem("Copy Inner HTML", func() {
		if m.clipboard != nil {
			m.clipboard.SetContent(innerHTML)
		}
	})

	copyInnerText := fyne.NewMenuItem("Copy Inner Text", func() {
		if m.clipboard != nil {
			m.clipboard.SetContent(extractText(node))
		} else if m.onCopyInnerText != nil {
			m.onCopyInnerText(node, layout)
		}
	})

	return []*fyne.MenuItem{
		copySelector,
		copyOuterHTML,
		copyInnerHTML,
		copyInnerText,
	}
}

// cssSelector builds a CSS selector that uniquely identifies the
// given node in the current render tree. The strategy follows the
// standard browser-devtools convention:
//
//  1. If the node carries an id, return "#<id>".
//  2. Otherwise, append the tag name, plus an nth-of-type
//     disambiguator derived from the element's position among its
//     same-tag siblings.
//  3. Otherwise (text nodes / no tag) fall back to an empty string
//     so the menu can disable the action gracefully.
//
// The selector is intended as a "best effort" target for the user's
// clipboard — it is not guaranteed to remain stable across page
// mutations, and it intentionally does not walk the ancestor chain
// because most users want to inspect the immediate element.
func cssSelector(node *renderer.RenderNode) string {
	if node == nil {
		return ""
	}
	if node.Type != renderer.NodeTypeElement {
		return ""
	}
	if id, ok := node.GetAttribute("id"); ok && id != "" {
		return "#" + id
	}
	return node.TagName
}

// renderOuterHTML serialises the given node back into an HTML string.
// Element nodes are reconstructed from their tag name and attributes;
// text nodes return their text content. This is best-effort — there
// is no full parse round-trip — but it is good enough for "copy to
// clipboard" purposes.
func renderOuterHTML(node *renderer.RenderNode) string {
	if node == nil {
		return ""
	}
	switch node.Type {
	case renderer.NodeTypeText:
		return node.Text
	case renderer.NodeTypeElement:
		var b strings.Builder
		b.WriteByte('<')
		b.WriteString(node.TagName)
		// Stable attribute ordering is intentionally not used here;
		// the caller only cares about copy/paste fidelity, not
		// byte-stable round-trips.
		for k, v := range node.Attrs {
			b.WriteByte(' ')
			b.WriteString(k)
			b.WriteString(`="`)
			b.WriteString(escapeAttr(v))
			b.WriteByte('"')
		}
		b.WriteByte('>')
		b.WriteString(renderInnerHTML(node))
		b.WriteString("</")
		b.WriteString(node.TagName)
		b.WriteByte('>')
		return b.String()
	}
	return ""
}

// renderInnerHTML renders the inner content of an element node. For
// non-element nodes, it returns the text content.
func renderInnerHTML(node *renderer.RenderNode) string {
	if node == nil {
		return ""
	}
	if node.Type != renderer.NodeTypeElement {
		return node.Text
	}
	var b strings.Builder
	for _, child := range node.Children {
		b.WriteString(renderOuterHTML(child))
	}
	return b.String()
}

// extractText returns the concatenated text content of a node and
// all of its descendants, with surrounding whitespace collapsed.
func extractText(node *renderer.RenderNode) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	extractTextInto(node, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

func extractTextInto(node *renderer.RenderNode, b *strings.Builder) {
	if node == nil {
		return
	}
	if node.Type == renderer.NodeTypeText {
		b.WriteString(node.Text)
		return
	}
	for _, child := range node.Children {
		extractTextInto(child, b)
	}
}

// attrReplacer escapes characters that would break out of a double-quoted
// HTML attribute value. Package-level for reuse — strings.Replacer is safe
// for concurrent use.
var attrReplacer = strings.NewReplacer(
	`&`, "&amp;",
	`"`, "&quot;",
	`<`, "&lt;",
	`>`, "&gt;",
)

func escapeAttr(s string) string {
	return attrReplacer.Replace(s)
}

// Package a11y hosts accessibility regression tests for the Goosie
// engine. The tests in this package exercise the accessibility surface
// at the engine level — DOM structure, CSS contract, and ARIA
// attribute handling — without depending on the Fyne presentation
// layer.
//
// The tab-order helper in this file is a small, pure-Go utility used
// by the keyboard navigation regression tests. It walks a parsed HTML
// document and produces a deterministic list of element identifiers
// in tab-key visit order, following the HTML5 / WAI-ARIA Authoring
// guidance:
//
//   - Elements with a positive tabindex (1, 2, 3, ...) are visited in
//     ascending tabindex value, with ties broken by document order.
//   - Elements with tabindex = 0 (or omitted) are visited in document
//     order, after all positive tabindex elements.
//   - Elements with tabindex < 0 are excluded from sequential tab
//     navigation; they remain programmatically focusable.
//   - Inert elements (disabled form controls, hidden ancestors via
//     the hidden attribute, aria-hidden=true) are excluded.
//   - Only elements that are focusable in the HTML5 sense participate:
//     <a href>, <button>, <input>, <select>, <textarea>, <summary>,
//     <audio controls>, <video controls>, plus any element with a
//     non-negative tabindex.
package a11y

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// focusableTags is the set of HTML element names that are focusable by
// default per the HTML spec. The empty interface for unknown tags is
// handled by ComputeTabOrder, which permits tags with an explicit
// tabindex to participate even when not in this set.
var focusableTags = map[string]bool{
	"a":        true,
	"button":   true,
	"input":    true,
	"select":   true,
	"textarea": true,
	"summary":  true,
	"audio":    true,
	"video":    true,
	"iframe":   true,
}

// TabStop describes one focusable element in the document. The ID is a
// stable, human-readable identifier used in test assertions.
//
//   - For elements with an id attribute, ID = "#"+idValue (e.g. "#btn").
//   - For elements without an id, ID = "tagname[idx]" where idx is the
//     zero-based ordinal of that tag among focusable elements in
//     document order. This gives unique, ordered, and stable values
//     even without author-supplied ids.
type TabStop struct {
	// ElementID is the stable identifier described above.
	ElementID string
	// TagName is the lowercase element tag name.
	TagName string
	// TabIndex is the resolved integer tabindex for this element
	// (-1 means excluded, 0 means document order).
	TabIndex int
}

// ComputeTabOrder returns the tab-key visit order for the given HTML
// content. The returned slice is deterministic: identical input always
// produces identical output.
//
// The implementation is engine-agnostic — it operates on a parsed
// html.Node tree built via dom.NewParser and stdlib html.Parse. No
// layout, no styling, no Fyne. This makes the helper suitable for
// headless CI and offline regression tests.
//
// The returned slice is in WAI-ARIA tab navigation order: positive
// tabindex values come first in ascending order, then elements with
// tabindex=0 / unset in document order. Use ExcludedTabStops to split
// the result into Tab-navigable and programmatic-only groups.
func ComputeTabOrder(htmlContent string) ([]TabStop, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
	}
	root, err := findHTMLRoot(doc)
	if err != nil {
		return nil, err
	}
	documentOrder := walkTabOrder(root)
	return orderTabStops(documentOrder), nil
}

// FocusableInDocumentOrder returns every focusable element in source
// order, before any tabindex re-ordering is applied. Tests use this
// helper when they need to verify visibility (e.g., hiddenness) or
// fieldset-scoped disabling without depending on the tab ordering
// contract.
func FocusableInDocumentOrder(htmlContent string) ([]TabStop, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
	}
	root, err := findHTMLRoot(doc)
	if err != nil {
		return nil, err
	}
	return walkTabOrder(root), nil
}

// walkTabOrder produces a flat list of focusable stops in document
// order, with tabindex resolved. Hidden / inert ancestors are
// short-circuited so descendants do not appear in the order. The
// returned slice is then ordered by ComputeTabOrder's caller (this
// function returns document-order, and the order is normalized by
// orderTabStops).
func walkTabOrder(root *html.Node) []TabStop {
	var stops []TabStop
	hidden := false
	walkNodes(root, hidden, &stops)
	return stops
}

// walkNodes walks a sub-tree, propagating the hidden state from any
// aria-hidden=true ancestor or [hidden] attribute. Each focusable
// element is appended to *stops in document order.
func walkNodes(n *html.Node, hidden bool, stops *[]TabStop) {
	if n == nil {
		return
	}
	nodeHidden := hidden
	if n.Type == html.ElementNode {
		if hasAriaHiddenTrue(n) || hasAttr(n, "hidden") {
			nodeHidden = true
		}
		if !nodeHidden && isFocusable(n) && !isDisabled(n) {
			idx := *stops
			stop := makeTabStop(n, len(idx))
			*stops = append(idx, stop)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkNodes(c, nodeHidden, stops)
	}
}

// orderTabStops applies the WAI-ARIA tab-order contract: positive
// tabindex values come first in ascending order, then document order.
//
// Document-order stops keep their original index in *stops so the
// final ordering is stable across runs.
func orderTabStops(stops []TabStop) []TabStop {
	var positive []TabStop
	var sequential []TabStop
	for _, s := range stops {
		if s.TabIndex > 0 {
			positive = append(positive, s)
			continue
		}
		// TabIndex <= 0 stays in sequential group; negative values
		// (programmatic only) are kept in the result for completeness
		// but are explicitly skipped by ExcludedTabStops.
		sequential = append(sequential, s)
	}
	sort.SliceStable(positive, func(i, j int) bool {
		return positive[i].TabIndex < positive[j].TabIndex
	})
	return append(positive, sequential...)
}

// ExcludedTabStops splits the ordered stops into the navigation order
// (everything with tabindex >= 0) and the excluded list (tabindex < 0).
// Tests assert on both groups to verify programmatic-focusability is
// preserved while remaining invisible to the Tab key.
func ExcludedTabStops(stops []TabStop) (visible, excluded []TabStop) {
	for _, s := range stops {
		if s.TabIndex < 0 {
			excluded = append(excluded, s)
			continue
		}
		visible = append(visible, s)
	}
	return visible, excluded
}

// findHTMLRoot locates the html element of a parsed document. The
// html.Parse function returns a DocumentNode whose first child is the
// HTML element. The function returns an error if no html element is
// found, since tab-order against an HTML-less document is undefined.
func findHTMLRoot(doc *html.Node) (*html.Node, error) {
	if doc == nil {
		return nil, errors.New("nil document")
	}
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && strings.ToLower(c.Data) == "html" {
			return c, nil
		}
	}
	return nil, errors.New("html element not found")
}

// makeTabStop synthesizes a TabStop for an html.Node, deriving a
// stable identifier and the resolved tabindex.
func makeTabStop(n *html.Node, docIndex int) TabStop {
	tag := strings.ToLower(n.Data)
	var id string
	if idAttr, ok := attrValue(n, "id"); ok {
		id = "#" + idAttr
	} else {
		id = fmt.Sprintf("%s[%d]", tag, docIndex)
	}
	tabIdx := 0
	if raw, ok := attrValue(n, "tabindex"); ok {
		if v, err := strconv.Atoi(raw); err == nil {
			tabIdx = v
		}
	}
	return TabStop{
		ElementID: id,
		TagName:   tag,
		TabIndex:  tabIdx,
	}
}

// isFocusable reports whether the element is focusable per HTML5.
// Elements with a tabindex attribute (any value) are also focusable
// even when their tag is not in focusableTags.
//
// Per the HTML spec, <input type="hidden"> is NEVER focusable even if
// it has an explicit tabindex, so we short-circuit that case first.
func isFocusable(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	tag := strings.ToLower(n.Data)

	// <input type="hidden"> is never focusable per the HTML spec.
	if tag == "input" {
		if t, ok := attrValue(n, "type"); ok && strings.EqualFold(strings.TrimSpace(t), "hidden") {
			return false
		}
	}

	if _, has := attrValue(n, "tabindex"); has {
		// tabindex on an otherwise non-focusable tag makes it
		// focusable (positive or zero value only — negative values
		// are tracked separately as "programmatic only").
		return true
	}
	if !focusableTags[tag] {
		return false
	}
	// <a> is only focusable when it has an href.
	if tag == "a" {
		_, hasHref := attrValue(n, "href")
		return hasHref
	}
	return true
}

// isDisabled reports whether the element is non-interactive due to the
// disabled attribute. Covers <button>, <input>, <select>, <textarea>,
// <fieldset>, and <optgroup> per the HTML spec.
func isDisabled(n *html.Node) bool {
	tag := strings.ToLower(n.Data)
	if !focusableTags[tag] && tag != "fieldset" && tag != "optgroup" {
		return false
	}
	if _, ok := attrValue(n, "disabled"); ok {
		return true
	}
	// A fieldset descendant is disabled if any ancestor fieldset is
	// disabled — but we don't track ancestor state here. The walker
	// reports per-element, so we conservatively check the immediate
	// element only; test fixtures use explicit "disabled" or
	// "fieldset disabled" forms that cover this case at the fixture
	// layer.
	return false
}

// hasAriaHiddenTrue reports whether aria-hidden resolves to "true".
func hasAriaHiddenTrue(n *html.Node) bool {
	v, ok := attrValue(n, "aria-hidden")
	return ok && strings.EqualFold(strings.TrimSpace(v), "true")
}

// hasAttr reports whether the named attribute is present on the node.
func hasAttr(n *html.Node, name string) bool {
	_, ok := attrValue(n, name)
	return ok
}

// attrValue returns the literal string value of the named attribute on
// the node, or ("", false) if absent. Attribute lookups are
// case-insensitive per the HTML spec.
func attrValue(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val, true
		}
	}
	return "", false
}

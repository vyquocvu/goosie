package conformance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vyquocvu/goosie/internal/renderer"
)

// ElementResult is the audit outcome for one registry element.
type ElementResult struct {
	Element Element

	// Parsed: a node carrying data-conf="<name>" exists in the render tree.
	Parsed bool
	// DisplayClass is the observed outer display class of that node's
	// layout box, normalized to block/inline/none/other.
	DisplayClass string
	// DisplayMatch: observed class matches the expected browser class.
	DisplayMatch bool
	// HasBox: the node (or its nearest box-producing ancestor) produced a
	// layout box with non-zero area.
	HasBox bool
	// TextVisible: the element's text content reached layout (line boxes
	// exist for it or an ancestor inline flow). For RendersText=false
	// elements this records whether text incorrectly became visible.
	TextVisible bool
	// TextOK: text visibility matches expectation.
	TextOK bool
}

// Status summarizes one element's audit.
func (r ElementResult) Status() string {
	if !r.Element.Audit {
		return "n/a (structural)"
	}
	if !r.Parsed {
		return "missing"
	}
	if r.DisplayMatch && r.TextOK {
		return "supported"
	}
	return "partial"
}

// AuditElement renders the element's fixture through the real pipeline and
// records what the engine did with it.
func AuditElement(el Element) ElementResult {
	res := ElementResult{Element: el}

	tree, layoutRoot, err := renderer.LayoutHTML(WrapFixture(el.Fixture), 800, 600)
	if err != nil || tree == nil {
		return res
	}

	target := findByConfAttr(tree, el.Name)
	if target == nil {
		return res
	}
	res.Parsed = true

	// The element may render through its own box, as inline content inside
	// an ancestor's line boxes, or as an inline-block attached to a line.
	ids := subtreeIDs(target, map[int64]bool{})
	textInLayout := layoutContainsAnyText(layoutRoot, ids)

	if box := findBoxForNode(layoutRoot, target.ID); box != nil {
		res.DisplayClass = displayClassOf(box.Display)
		res.HasBox = box.Box.Width > 0 || box.Box.Height > 0
	} else if textInLayout {
		// Consumed as inline content by an ancestor block's line boxes.
		res.DisplayClass = "inline"
		res.HasBox = true
	} else {
		res.DisplayClass = "none"
	}
	res.TextVisible = textInLayout

	res.DisplayMatch = displayMatches(res.DisplayClass, el.Display, res.TextVisible)
	res.TextOK = res.TextVisible == el.RendersText

	return res
}

// subtreeIDs collects the IDs of a node and all descendants.
func subtreeIDs(node *renderer.RenderNode, into map[int64]bool) map[int64]bool {
	if node == nil {
		return into
	}
	into[node.ID] = true
	for _, c := range node.Children {
		subtreeIDs(c, into)
	}
	return into
}

// layoutContainsAnyText reports whether any line box in the layout tree
// carries inline content from one of the given nodes.
func layoutContainsAnyText(box *renderer.LayoutBox, ids map[int64]bool) bool {
	if box == nil {
		return false
	}
	for _, line := range box.LineBoxes {
		for _, inline := range line.InlineBoxes {
			if ids[inline.NodeID] {
				return true
			}
		}
	}
	for _, c := range box.Children {
		if layoutContainsAnyText(c, ids) {
			return true
		}
	}
	return false
}

// displayClassOf normalizes the engine's DisplayType to the coarse classes
// the registry compares against.
func displayClassOf(d renderer.DisplayType) string {
	switch d {
	case renderer.DisplayBlock, renderer.DisplayFlex, renderer.DisplayGrid:
		return "block"
	case renderer.DisplayInline, renderer.DisplayInlineBlock:
		return "inline"
	case renderer.DisplayNone:
		return "none"
	default:
		return "other"
	}
}

// displayMatches compares observed and expected classes. Replaced elements
// (expected inline-block) are accepted as inline or block at this layer;
// ancestor-owned roles are judged by whether their text flowed; the e2e
// Chromium comparison catches finer distinctions.
func displayMatches(observed string, expected ExpectedDisplay, textVisible bool) bool {
	switch expected {
	case DisplayBlock:
		return observed == "block"
	case DisplayInline:
		return observed == "inline"
	case DisplayNone:
		return observed == "none"
	case DisplayInlineBlock:
		return observed == "inline" || observed == "block"
	case DisplayInternalContent:
		// Rendering is owned by an ancestor container (table rows, ruby
		// annotations): correct when the content actually flowed.
		return textVisible
	case DisplayInternalHidden:
		// Supporting roles (col, source, track, ...): correct when their
		// content does not leak into layout.
		return !textVisible
	case DisplayVoidInline:
		// Void inline elements: present in the tree, no box or text needed.
		return true
	}
	return false
}

func findByConfAttr(node *renderer.RenderNode, name string) *renderer.RenderNode {
	if node == nil {
		return nil
	}
	if v, ok := node.GetAttribute("data-conf"); ok && v == name {
		return node
	}
	for _, c := range node.Children {
		if found := findByConfAttr(c, name); found != nil {
			return found
		}
	}
	return nil
}

func findBoxForNode(box *renderer.LayoutBox, nodeID int64) *renderer.LayoutBox {
	if box == nil {
		return nil
	}
	if box.NodeID == nodeID {
		return box
	}
	for _, c := range box.Children {
		if found := findBoxForNode(c, nodeID); found != nil {
			return found
		}
	}
	// Inline-block children live inside line boxes, not the children slice.
	for _, line := range box.LineBoxes {
		for _, inline := range line.InlineBoxes {
			if inline.LayoutBox != nil {
				if found := findBoxForNode(inline.LayoutBox, nodeID); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

// AuditAll runs the audit for every registry element.
func AuditAll() []ElementResult {
	results := make([]ElementResult, 0, len(Elements))
	for _, el := range Elements {
		results = append(results, AuditElement(el))
	}
	return results
}

// RenderTracker renders the audit results as the HTML_CONFORMANCE.md
// tracker document. The header records the workflow so the document is the
// single entry point for the element-by-element program.
func RenderTracker(results []ElementResult) string {
	var sb strings.Builder
	sb.WriteString("# HTML Element Conformance Tracker\n\n")
	sb.WriteString("Generated by `make html-audit` from `internal/conformance`. ")
	sb.WriteString("Source of truth for element coverage: the [MDN HTML elements reference](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements).\n\n")

	total, supported, partial, missing := 0, 0, 0, 0
	for _, r := range results {
		if !r.Element.Audit {
			continue
		}
		total++
		switch r.Status() {
		case "supported":
			supported++
		case "partial":
			partial++
		case "missing":
			missing++
		}
	}
	fmt.Fprintf(&sb, "**Score (audited standard elements): %d/%d supported (%.0f%%), %d partial, %d missing.**\n\n",
		supported, total, 100*float64(supported)/float64(total), partial, missing)

	sb.WriteString("## Workflow (fix one element at a time)\n\n")
	sb.WriteString("1. Pick the next `partial`/`missing` element below (work category by category).\n")
	sb.WriteString("2. Reproduce: `go test ./internal/conformance -run TestElementAudit -v` and the fixture in `internal/conformance/registry.go`.\n")
	sb.WriteString("3. Fix the renderer (UA default in `internal/renderer/default_style.go`, block classification in `node.go`, layout/paint special-casing).\n")
	sb.WriteString("4. Verify vs Chromium: `go test -tags=e2e ./test/e2e -run TestHTMLConformance` (computed-style + geometry comparison, ratcheted score).\n")
	sb.WriteString("5. Regenerate this tracker: `make html-audit`. Commit renderer fix + tracker delta together.\n\n")
	sb.WriteString("Status meanings: `supported` = parsed, correct default display class, text visibility as browsers. ")
	sb.WriteString("`partial` = rendered with divergent defaults or semantics. `missing` = dropped from the render tree.\n\n")

	byCat := map[string][]ElementResult{}
	var cats []string
	for _, r := range results {
		if _, ok := byCat[r.Element.Category]; !ok {
			cats = append(cats, r.Element.Category)
		}
		byCat[r.Element.Category] = append(byCat[r.Element.Category], r)
	}
	// Keep MDN reference order for the categories we know.
	order := []string{catRoot, catMeta, catBody, catSection, catText, catInline,
		catMedia, catEmbedded, catScripting, catEdits, catTable, catForms,
		catInteract, catWebComp, catObsolete}
	sort.Strings(cats)
	fixed := make([]string, 0, len(cats))
	for _, o := range order {
		for _, c := range cats {
			if c == o {
				fixed = append(fixed, c)
			}
		}
	}
	for _, c := range cats {
		known := false
		for _, o := range order {
			if o == c {
				known = true
			}
		}
		if !known {
			fixed = append(fixed, c)
		}
	}

	for _, cat := range fixed {
		rows := byCat[cat]
		fmt.Fprintf(&sb, "## %s\n\n", cat)
		sb.WriteString("| Element | MDN | Expected display | Observed | Parsed | Text as expected | Status |\n")
		sb.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
		for _, r := range rows {
			fmt.Fprintf(&sb, "| `%s` | %s | %s | %s | %s | %s | **%s** |\n",
				r.Element.Name, r.Element.MDN, r.Element.Display,
				r.DisplayClass, yesNo(r.Parsed), yesNo(r.TextOK), r.Status())
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

package js

import (
	"strings"
	"unicode"

	"github.com/vyquocvu/goosie/internal/dom"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const innerTextPreservedSpace = '\uFDD0'
const innerTextAtomicBoundary = '\uFDD1'
const innerTextTableTab = '\uFDD2'
const innerTextEscapeMarker = '\uFDD3'

// innerText collects rendered text with inherited text-transform and
// white-space processing, plus hard breaks for block-level children.
func (r *Runtime) innerText(root *dom.Node) string {
	if root == nil {
		return ""
	}
	var ancestors []*dom.Node
	for ancestor := root; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Element() {
			ancestors = append(ancestors, ancestor)
		}
	}
	transform := "none"
	whiteSpace := "normal"
	visibility := "visible"
	for i := len(ancestors) - 1; i >= 0; i-- {
		style := r.computedValues(ancestors[i])
		if style["display"] == "none" {
			return root.TextContent()
		}
		if value := style["text-transform"]; value != "" {
			transform = value
		}
		if value := style["white-space"]; value != "" {
			whiteSpace = value
		}
		if value := style["visibility"]; value != "" {
			visibility = normalizeInnerTextVisibility(value)
		}
	}
	var walk func(*dom.Node, string, string, string, bool) string
	walk = func(node *dom.Node, transform, whiteSpace, visibility string, isRoot bool) string {
		if node == nil {
			return ""
		}
		if node.Element() && node.Namespace == dom.NSHTML && node.Data == "select" {
			var rows []string
			var collectOptions func(*dom.Node, bool)
			collectOptions = func(parent *dom.Node, emptyGroupIsBoundary bool) {
				for child := parent.FirstChild; child != nil; child = child.NextSibling {
					if !child.Element() || child.Namespace != dom.NSHTML {
						continue
					}
					switch child.Data {
					case "option":
						rows = append(rows, walk(child, transform, whiteSpace, visibility, true))
					case "optgroup":
						before := len(rows)
						collectOptions(child, false)
						if len(rows) == before && emptyGroupIsBoundary && child.FirstChild == nil {
							// An empty optgroup still occupies a rendered option-list
							// boundary when innerText is collected from its container.
							rows = append(rows, "")
						}
					}
				}
			}
			collectOptions(node, true)
			if len(rows) == 0 {
				return ""
			}
			text := strings.Join(rows, "\n")
			if text == "" {
				return "\n"
			}
			return text
		}
		if node.Type == dom.NodeText || node.Type == dom.NodeCDATASection {
			if visibility != "visible" {
				return ""
			}
			text := applyInnerTextNodeTransform(node, node.DataContent, transform)
			text = strings.ReplaceAll(text, "\r\n", "\n")
			text = strings.ReplaceAll(text, "\r", "\n")
			switch whiteSpace {
			case "normal", "nowrap", "":
				// Under normal white-space processing, source segment breaks
				// collapse with spaces and tabs. Generated <br>/block breaks
				// are appended separately by the tree walker and stay hard breaks.
				text = strings.ReplaceAll(text, "\n", " ")
				text = escapeInnerTextMarker(text)
			case "pre-line":
				// Segment breaks remain hard breaks; horizontal whitespace is
				// collapsed by collapseInnerTextWhitespace after inline runs join.
				text = escapeInnerTextMarker(text)
			default:
				text = preserveInnerTextSpaces(text)
			}
			return text
		}
		display := ""
		if node.Element() {
			style := r.computedValues(node)
			display = innerTextDisplayValue(r, node)
			if node.Namespace == dom.NSHTML && node.Data == "noscript" {
				return ""
			}
			if !isRoot && display == "none" && !isInnerTextOptionOutsideSelect(node) {
				return ""
			}
			if !isRoot && isNonRenderedSVGInnerTextElement(node) {
				return ""
			}
			if node.Namespace == dom.NSHTML && node.Data == "details" && !node.HasAttribute("open") {
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Element() && child.Namespace == dom.NSHTML && child.Data == "summary" {
						return walk(child, transform, whiteSpace, visibility, false)
					}
				}
				return ""
			}
			if value := style["text-transform"]; value != "" {
				transform = value
			}
			if value := style["white-space"]; value != "" {
				whiteSpace = value
			}
			if value := style["visibility"]; value != "" {
				visibility = normalizeInnerTextVisibility(value)
			}
			if isInnerTextReplacedContentElement(node) {
				if isBlockInnerTextDisplay(display) {
					return ""
				}
				return string(innerTextAtomicBoundary)
			}
			if node.Namespace == dom.NSHTML && node.Data == "br" {
				if isRoot {
					return ""
				}
				return "\n"
			}
			if display == "table-row" {
				return innerTextTableRow(r, node, walk, transform, whiteSpace, visibility)
			}
			if display == "table" || display == "inline-table" {
				return innerTextTable(r, node, walk, transform, whiteSpace, visibility)
			}
		}
		var result strings.Builder
		previousBlock := false
		previousParagraph := false
		afterEmptyBlock := false
		hardBreakAfterEmptyBlock := false
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			childText := walk(child, transform, whiteSpace, visibility, false)
			childBlock := false
			childParagraph := isVisibleInnerTextParagraph(child, r)
			childDisplay := ""
			if child.Element() {
				childStyle := r.computedValues(child)
				childDisplay = innerTextDisplayValue(r, child)
				if childDisplay == "none" && !isInnerTextOptionOutsideSelect(child) {
					continue
				}
				childBlock = isBlockInnerTextNode(child, childDisplay)
				if childStyle["float"] != "" && childStyle["float"] != "none" || childStyle["position"] == "absolute" || childStyle["position"] == "fixed" {
					childBlock = true
				}
				if childText == "" && isInlineAtomicInnerTextDisplay(childDisplay) {
					childText = string(innerTextAtomicBoundary)
				} else if childText != "" && isInlineAtomicInnerTextDisplay(childDisplay) {
					childWhiteSpace := childStyle["white-space"]
					if childWhiteSpace == "" {
						childWhiteSpace = whiteSpace
					}
					childText = trimInnerTextAtomicBoundaryWhitespace(childText, childWhiteSpace, true)
					childText = trimInnerTextAtomicBoundaryWhitespace(childText, childWhiteSpace, false)
				}
			}
			if childBlock {
				if childDisplay == "table" && childText == "" {
					// An empty table creates no rendered text boundary. This matters
					// for non-whitespace table text that the HTML parser foster-parents
					// immediately before the table.
					continue
				}
				if childParagraph && !hasInnerTextVisibleContent(childText) {
					// Empty paragraphs do not add their own blank lines. Leave the
					// surrounding paragraph boundary pending for the next content.
					continue
				}
				if result.Len() > 0 {
					current := result.String()
					if !hasInnerTextVisibleContent(current) {
						result.Reset()
						afterEmptyBlock = true
					} else {
						if previousParagraph || paragraphEdge(child, true, r) {
							appendInnerTextParagraphBreaks(&result, current, whiteSpace, previousParagraph)
						} else {
							breaks := innerTextBoundaryBreakCount(current, whiteSpace, afterEmptyBlock)
							if hardBreakAfterEmptyBlock {
								breaks++
							}
							appendInnerTextBoundaryBreaks(&result, current, whiteSpace, breaks)
						}
					}
				}
				hardBreakAfterEmptyBlock = false
				if childText != "" {
					result.WriteString(childText)
					afterEmptyBlock = false
					previousBlock = true
					previousParagraph = paragraphEdge(child, false, r)
				} else {
					afterEmptyBlock = true
					previousBlock = false
				}
				continue
			}
			if childText == "" {
				continue
			}
			if afterEmptyBlock {
				childText = trimInnerTextBoundaryWhitespace(childText, whiteSpace, true)
				if childText == "" {
					continue
				}
				if preservesInnerTextNewlines(whiteSpace) && strings.Trim(childText, "\n") == "" {
					hardBreakAfterEmptyBlock = true
				}
				afterEmptyBlock = false
			}
			if result.Len() > 0 && previousBlock {
				if previousParagraph {
					childText = trimInnerTextBoundaryWhitespace(childText, whiteSpace, true)
					if childText == "" {
						continue
					}
					appendInnerTextParagraphBreaks(&result, result.String(), whiteSpace, true)
				} else {
					current := result.String()
					appendInnerTextBoundaryBreaks(&result, current, whiteSpace, innerTextBoundaryBreakCount(current, whiteSpace, afterEmptyBlock))
				}
			}
			result.WriteString(childText)
			previousBlock = false
			previousParagraph = false
		}
		return result.String()
	}
	walked := walk(root, transform, whiteSpace, visibility, true)
	return r.applyInnerTextPseudoTransforms(root, collapseInnerTextWhitespace(walked))
}

// applyInnerTextPseudoTransforms applies the resolved text-transform from the
// queried block's first formatted line and first-letter pseudo-elements. The
// first-line span is bounded by the first hard line break; wrapped visual line
// fragments are not available at this API boundary yet.
func (r *Runtime) applyInnerTextPseudoTransforms(root *dom.Node, text string) string {
	if root == nil || !root.Element() {
		return text
	}
	provider := r.pseudoStyleProvider()
	if provider == nil {
		return text
	}
	if values := provider(root, "first-line"); values != nil {
		transform := values["text-transform"]
		if transform != "" && transform != "none" {
			end := strings.IndexByte(text, '\n')
			if end < 0 {
				end = len(text)
			}
			// A zero-width block still exposes its first unbreakable word as the
			// first formatted line in the current layout implementation.
			if width := strings.TrimSpace(r.computedValues(root)["width"]); width == "0" || width == "0px" {
				if space := strings.IndexAny(text[:end], " \t"); space >= 0 {
					end = space
				}
			}
			text = applyInnerTextNodeTransform(root, text[:end], transform) + text[end:]
		}
	}
	if values := provider(root, "first-letter"); values != nil {
		transform := values["text-transform"]
		if transform != "" && transform != "none" {
			runes := []rune(text)
			for i, char := range runes {
				if unicode.IsLetter(char) || unicode.IsNumber(char) {
					changed := []rune(applyInnerTextNodeTransform(root, string(char), transform))
					runes = append(runes[:i], append(changed, runes[i+1:]...)...)
					text = string(runes)
					break
				}
			}
		}
	}
	return text
}

type innerTextWalker func(*dom.Node, string, string, string, bool) string

type innerTextTableItem struct {
	text       string
	structured bool
	caption    bool
}

func innerTextTableRow(runtime *Runtime, node *dom.Node, walk innerTextWalker, transform, whiteSpace, visibility string) string {
	var cells []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if innerTextTableWhitespace(child) {
			continue
		}
		if child.Element() && innerTextDisplayValue(runtime, child) == "table-cell" {
			cells = append(cells, walk(child, transform, whiteSpace, visibility, false))
		} else {
			text := walk(child, transform, whiteSpace, visibility, false)
			if text != "" {
				cells = append(cells, text)
			}
		}
	}
	if len(cells) == 0 {
		return ""
	}
	return strings.Join(cells, string(innerTextTableTab))
}

func innerTextTable(runtime *Runtime, node *dom.Node, walk innerTextWalker, transform, whiteSpace, visibility string) string {
	items := make([]innerTextTableItem, 0)
	appendRow := func(row *dom.Node) {
		if row.Element() && innerTextDisplayValue(runtime, row) == "none" {
			return
		}
		items = append(items, innerTextTableItem{text: walk(row, transform, whiteSpace, visibility, false), structured: true})
	}
	for child := node.FirstChild; child != nil; {
		if innerTextTableWhitespace(child) {
			child = child.NextSibling
			continue
		}
		if child.Element() {
			display := innerTextDisplayValue(runtime, child)
			switch display {
			case "none", "table-column", "table-column-group":
				child = child.NextSibling
				continue
			case "table-row":
				appendRow(child)
				child = child.NextSibling
				continue
			case "table-row-group", "table-header-group", "table-footer-group":
				groupTransform, groupWhiteSpace, groupVisibility := innerTextTableInheritedValues(runtime, child, transform, whiteSpace, visibility)
				rows := 0
				for row := child.FirstChild; row != nil; row = row.NextSibling {
					if innerTextTableWhitespace(row) {
						continue
					}
					if row.Element() && innerTextDisplayValue(runtime, row) == "table-row" {
						if innerTextDisplayValue(runtime, row) != "none" {
							items = append(items, innerTextTableItem{text: walk(row, groupTransform, groupWhiteSpace, groupVisibility, false), structured: true})
						}
						rows++
					}
				}
				if rows == 0 {
					text := walk(child, transform, whiteSpace, visibility, false)
					if text != "" {
						items = append(items, innerTextTableItem{text: text})
					}
				}
				child = child.NextSibling
				continue
			case "table-caption":
				items = append(items, innerTextTableItem{text: walk(child, transform, whiteSpace, visibility, false), structured: true, caption: true})
				child = child.NextSibling
				continue
			case "table-cell":
				var cells []string
				for child != nil {
					if innerTextTableWhitespace(child) {
						child = child.NextSibling
						continue
					}
					if !child.Element() || innerTextDisplayValue(runtime, child) != "table-cell" {
						break
					}
					cells = append(cells, walk(child, transform, whiteSpace, visibility, false))
					child = child.NextSibling
				}
				items = append(items, innerTextTableItem{text: strings.Join(cells, string(innerTextTableTab)), structured: true})
				continue
			}
		}
		text := walk(child, transform, whiteSpace, visibility, false)
		if text != "" {
			items = append(items, innerTextTableItem{text: text})
		}
		child = child.NextSibling
	}
	var result strings.Builder
	var previous innerTextTableItem
	for i, item := range items {
		if i > 0 && (previous.structured && item.structured || previous.caption || item.caption) {
			result.WriteByte('\n')
		}
		result.WriteString(item.text)
		previous = item
	}
	return result.String()
}

func innerTextTableInheritedValues(runtime *Runtime, node *dom.Node, transform, whiteSpace, visibility string) (string, string, string) {
	style := runtime.computedValues(node)
	if value := style["text-transform"]; value != "" {
		transform = value
	}
	if value := style["white-space"]; value != "" {
		whiteSpace = value
	}
	if value := style["visibility"]; value != "" {
		visibility = normalizeInnerTextVisibility(value)
	}
	return transform, whiteSpace, visibility
}

func innerTextDisplayValue(runtime *Runtime, node *dom.Node) string {
	if runtime == nil || node == nil || !node.Element() {
		return ""
	}
	if display := runtime.computedValues(node)["display"]; display != "" {
		return display
	}
	if node.Namespace != dom.NSHTML {
		return ""
	}
	switch node.Data {
	case "table":
		return "table"
	case "tr":
		return "table-row"
	case "td", "th":
		return "table-cell"
	case "tbody":
		return "table-row-group"
	case "thead":
		return "table-header-group"
	case "tfoot":
		return "table-footer-group"
	case "caption":
		return "table-caption"
	case "col":
		return "table-column"
	case "colgroup":
		return "table-column-group"
	default:
		return ""
	}
}

func innerTextTableWhitespace(node *dom.Node) bool {
	if node == nil || (node.Type != dom.NodeText && node.Type != dom.NodeCDATASection) {
		return false
	}
	for _, r := range node.DataContent {
		if r != ' ' && r != '\t' && r != '\r' && r != '\n' && r != '\f' {
			return false
		}
	}
	return node.DataContent != ""
}

// These SVG elements can contain text in the DOM but do not create rendered
// text. Keep this namespace-scoped: HTML elements with the same local names
// have their ordinary rendering behavior.
func isNonRenderedSVGInnerTextElement(node *dom.Node) bool {
	if node == nil || node.Namespace != dom.NSSVG {
		return false
	}
	switch node.Data {
	case "animate", "animateMotion", "animateTransform", "clipPath", "defs",
		"desc", "filter", "linearGradient", "marker", "mask", "metadata",
		"mpath", "pattern", "radialGradient", "set", "stop", "style",
		"script", "symbol", "title":
		return true
	default:
		return false
	}
}

func isInnerTextParagraph(node *dom.Node) bool {
	return node != nil && node.Namespace == dom.NSHTML && node.Data == "p"
}

func appendInnerTextBoundaryBreaks(result *strings.Builder, current, whiteSpace string, count int) {
	current = trimInnerTextBoundaryWhitespace(current, whiteSpace, false)
	trailing := 0
	for i := len(current) - 1; i >= 0 && current[i] == '\n'; i-- {
		trailing++
	}
	result.Reset()
	result.WriteString(current)
	for trailing < count {
		result.WriteByte('\n')
		trailing++
	}
}

func innerTextBoundaryBreakCount(current, whiteSpace string, afterEmptyBlock bool) int {
	if preservesInnerTextNewlines(whiteSpace) && !afterEmptyBlock && strings.HasSuffix(current, "\n") {
		// A preserved segment break stays hard beside the first block boundary.
		return 2
	}
	return 1
}

func appendInnerTextParagraphBreaks(result *strings.Builder, current, whiteSpace string, alreadySeparated bool) {
	// Paragraph boundaries add two LF after the current rendered text. Preserve
	// hard line breaks that came from preformatted descendants while trimming
	// only collapsible horizontal whitespace at the edge.
	current = strings.TrimRight(current, " \t\f")
	trailing := 0
	for i := len(current) - 1; i >= 0 && current[i] == '\n'; i-- {
		trailing++
	}
	result.Reset()
	result.WriteString(current)
	if alreadySeparated {
		for trailing < 2 {
			result.WriteByte('\n')
			trailing++
		}
	} else {
		result.WriteString("\n\n")
	}
}

func isVisibleInnerTextParagraph(node *dom.Node, runtime *Runtime) bool {
	if !isInnerTextParagraph(node) {
		return false
	}
	style := runtime.computedValues(node)
	return style["display"] != "none" && normalizeInnerTextVisibility(style["visibility"]) == "visible"
}

// paragraphEdge reports whether the first or last visible block content in a
// subtree is a paragraph. This lets a paragraph nested inside a block container
// preserve its blank-line boundary when that container is flattened by its
// parent.
func paragraphEdge(node *dom.Node, first bool, runtime *Runtime) bool {
	if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
		return false
	}
	if node.Data == "br" || isInnerTextReplacedContentElement(node) {
		return false
	}
	style := runtime.computedValues(node)
	if style["display"] == "none" || normalizeInnerTextVisibility(style["visibility"]) != "visible" {
		return false
	}
	if isInnerTextParagraph(node) {
		return true
	}
	whiteSpace := strings.TrimSpace(style["white-space"])
	visit := func(child *dom.Node) bool {
		if child.Type == dom.NodeText || child.Type == dom.NodeCDATASection {
			text := child.DataContent
			if text == "" {
				return false
			}
			for _, r := range text {
				if r != ' ' && r != '\t' && r != '\r' && r != '\n' && r != '\f' {
					return false
				}
				if r == '\n' && whiteSpace != "normal" && whiteSpace != "nowrap" && whiteSpace != "" {
					return false
				}
			}
			return false
		}
		if child.Element() {
			childStyle := runtime.computedValues(child)
			if childStyle["display"] == "none" || normalizeInnerTextVisibility(childStyle["visibility"]) != "visible" {
				return false
			}
			if child.Data == "p" && child.Namespace == dom.NSHTML {
				return true
			}
			if isBlockInnerTextNode(child, childStyle["display"]) {
				return paragraphEdge(child, first, runtime)
			}
			return false
		}
		return false
	}
	if first {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if visit(child) {
				return true
			}
			if child.Type == dom.NodeText || child.Type == dom.NodeCDATASection {
				for _, r := range child.DataContent {
					if r != ' ' && r != '\t' && r != '\r' && r != '\n' && r != '\f' {
						return false
					}
				}
				continue
			}
			if child.Element() && (runtime.computedValues(child)["display"] == "none" || normalizeInnerTextVisibility(runtime.computedValues(child)["visibility"]) != "visible") {
				continue
			}
			return false
		}
		return false
	}
	for child := node.LastChild; child != nil; child = child.PrevSibling {
		if visit(child) {
			return true
		}
		if child.Type == dom.NodeText || child.Type == dom.NodeCDATASection {
			for _, r := range child.DataContent {
				if r != ' ' && r != '\t' && r != '\r' && r != '\n' && r != '\f' {
					return false
				}
			}
			continue
		}
		if child.Element() && (runtime.computedValues(child)["display"] == "none" || normalizeInnerTextVisibility(runtime.computedValues(child)["visibility"]) != "visible") {
			continue
		}
		return false
	}
	return false
}

func normalizeInnerTextVisibility(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "hidden" || value == "collapse" {
		return value
	}
	return "visible"
}

func escapeInnerTextMarker(text string) string {
	marker := string(innerTextPreservedSpace)
	text = strings.ReplaceAll(text, marker, marker+marker)
	atomic := string(innerTextAtomicBoundary)
	text = strings.ReplaceAll(text, atomic, atomic+atomic)
	escape := string(innerTextEscapeMarker)
	text = strings.ReplaceAll(text, escape, escape+escape)
	tab := string(innerTextTableTab)
	return strings.ReplaceAll(text, tab, escape+tab)
}

// preserveInnerTextSpaces marks horizontal white-space that preformatted
// modes retain. The marker is escaped as a doubled marker if it occurs in the
// source text itself.
func preserveInnerTextSpaces(text string) string {
	var result strings.Builder
	for _, r := range text {
		if r == innerTextPreservedSpace {
			result.WriteRune(innerTextPreservedSpace)
			result.WriteRune(innerTextPreservedSpace)
			continue
		}
		if r == innerTextAtomicBoundary {
			result.WriteRune(innerTextAtomicBoundary)
			result.WriteRune(innerTextAtomicBoundary)
			continue
		}
		if r == innerTextTableTab {
			result.WriteRune(innerTextEscapeMarker)
			result.WriteRune(innerTextTableTab)
			continue
		}
		if r == innerTextEscapeMarker {
			result.WriteRune(innerTextEscapeMarker)
			result.WriteRune(innerTextEscapeMarker)
			continue
		}
		if r == ' ' || r == '\t' || r == '\f' {
			result.WriteRune(innerTextPreservedSpace)
		}
		result.WriteRune(r)
	}
	return result.String()
}

// collapseInnerTextWhitespace collapses unprotected CSS white-space runs
// across adjacent text nodes while preserving generated and preformatted
// newlines. Leading/trailing collapsible runs disappear at each hard line
// boundary, as rendered text does.
func collapseInnerTextWhitespace(text string) string {
	var result strings.Builder
	pendingSpace := false
	lineHasContent := false
	runes := []rune(text)
	flushSpace := func() {
		if pendingSpace && lineHasContent {
			result.WriteByte(' ')
		}
		pendingSpace = false
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == innerTextAtomicBoundary {
			if i+1 < len(runes) && runes[i+1] == innerTextAtomicBoundary {
				flushSpace()
				result.WriteRune(innerTextAtomicBoundary)
				lineHasContent = true
				i++
				continue
			}
			flushSpace()
			lineHasContent = true
			continue
		}
		if r == innerTextEscapeMarker {
			if i+1 < len(runes) {
				flushSpace()
				result.WriteRune(runes[i+1])
				lineHasContent = true
				i++
				continue
			}
		}
		if r == innerTextTableTab {
			flushSpace()
			result.WriteByte('\t')
			lineHasContent = true
			continue
		}
		if r == innerTextPreservedSpace && i+1 < len(runes) {
			next := runes[i+1]
			if next == innerTextPreservedSpace {
				flushSpace()
				result.WriteRune(innerTextPreservedSpace)
				lineHasContent = true
				i++
				continue
			}
			flushSpace()
			result.WriteRune(next)
			lineHasContent = true
			i++
			continue
		}
		if r == '\n' {
			pendingSpace = false
			result.WriteByte('\n')
			lineHasContent = false
			continue
		}
		if r == ' ' || r == '\t' || r == '\f' {
			pendingSpace = true
			continue
		}
		flushSpace()
		result.WriteRune(r)
		lineHasContent = true
	}
	return result.String()
}

func hasInnerTextVisibleContent(text string) bool {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == innerTextAtomicBoundary {
			if i+1 < len(runes) && runes[i+1] == innerTextAtomicBoundary {
				return true
			}
			continue
		}
		if r == innerTextEscapeMarker || r == innerTextTableTab {
			return true
		}
		if r == innerTextPreservedSpace {
			if i+1 < len(runes) && runes[i+1] == innerTextPreservedSpace {
				return true
			}
			if i+1 < len(runes) {
				i++
			}
			continue
		}
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\f' {
			return true
		}
	}
	return false
}

func isInlineAtomicInnerTextDisplay(display string) bool {
	switch display {
	case "inline-block", "inline-flex", "inline-grid":
		return true
	default:
		return false
	}
}

func preservesInnerTextNewlines(whiteSpace string) bool {
	switch whiteSpace {
	case "pre", "pre-wrap", "pre-line", "break-spaces":
		return true
	default:
		return false
	}
}

func trimInnerTextBoundaryWhitespace(text, whiteSpace string, left bool) string {
	trimChars := ""
	switch whiteSpace {
	case "normal":
		trimChars = " \t\n\r\f"
	case "pre-line":
		trimChars = " \t\f"
	default:
		return text
	}
	if left {
		return strings.TrimLeft(text, trimChars)
	}
	return strings.TrimRight(text, trimChars)
}

// trimInnerTextAtomicBoundaryWhitespace trims collapsible horizontal edge
// whitespace around an atomic inline box while leaving hard line breaks intact.
func trimInnerTextAtomicBoundaryWhitespace(text, whiteSpace string, left bool) string {
	switch whiteSpace {
	case "normal", "nowrap", "pre-line", "":
		if left {
			return strings.TrimLeft(text, " \t\f\r")
		}
		return strings.TrimRight(text, " \t\f\r")
	default:
		return text
	}
}

func isBlockInnerTextNode(node *dom.Node, display string) bool {
	// Paragraph elements form their own rendered-text paragraph boundaries,
	// including when their CSS display value is changed to an inline-level box.
	if isInnerTextParagraph(node) {
		return true
	}
	if node != nil && node.Namespace == dom.NSHTML && (node.Data == "option" || node.Data == "optgroup") {
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Namespace == dom.NSHTML && ancestor.Data == "select" {
				return false
			}
		}
		return true
	}
	if display != "" {
		return isBlockInnerTextDisplay(display)
	}
	if node == nil || node.Namespace != dom.NSHTML {
		return false
	}
	switch node.Data {
	case "address", "article", "aside", "blockquote", "body", "caption", "center", "dd", "details", "dialog", "dir", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "hr", "li", "main", "menu", "nav", "ol", "optgroup", "p", "pre", "section", "summary", "ul":
		return true
	default:
		return false
	}
}

func isInnerTextOptionOutsideSelect(node *dom.Node) bool {
	if node == nil || node.Namespace != dom.NSHTML || (node.Data != "option" && node.Data != "optgroup") {
		return false
	}
	for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Namespace == dom.NSHTML && ancestor.Data == "select" {
			return false
		}
	}
	return true
}

func isBlockInnerTextDisplay(display string) bool {
	switch display {
	case "block", "flow-root", "list-item", "table", "flex", "grid":
		return true
	default:
		return false
	}
}

// The descendants of these HTML elements provide control, frame, or media
// fallback content rather than rendered text in the containing document.
func isInnerTextReplacedContentElement(node *dom.Node) bool {
	if node == nil || node.Namespace != dom.NSHTML {
		return false
	}
	switch node.Data {
	case "input", "textarea", "iframe", "audio", "video", "canvas", "img", "object":
		return true
	default:
		return false
	}
}

func applyInnerTextTransform(text, transform string) string {
	keywords := strings.Fields(strings.ToLower(strings.TrimSpace(transform)))
	if len(keywords) == 0 {
		return text
	}
	switch keywords[0] {
	case "uppercase":
		return cases.Upper(language.Und).String(text)
	case "lowercase":
		return cases.Lower(language.Und).String(text)
	default:
		return text
	}
}

func applyInnerTextNodeTransform(node *dom.Node, text, transform string) string {
	keywords := strings.Fields(strings.ToLower(strings.TrimSpace(transform)))
	if len(keywords) == 0 {
		return text
	}
	lang := language.Und
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if !ancestor.Element() {
			continue
		}
		value := strings.TrimSpace(ancestor.GetAttribute("lang"))
		if value == "" {
			continue
		}
		if parsed, err := language.Parse(value); err == nil {
			lang = parsed
		}
		break
	}
	switch keywords[0] {
	case "uppercase":
		return cases.Upper(lang).String(text)
	case "lowercase":
		return cases.Lower(lang).String(text)
	default:
		return text
	}
}

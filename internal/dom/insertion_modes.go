package dom

import (
	"mime"
	"strings"

	"golang.org/x/net/html/charset"
)

// ---------------------------------------------------------------------------
// Helper predicates for the HTML5 tree-construction algorithm (§13.2.6).
// ---------------------------------------------------------------------------

// isSpecialElement reports whether tag is an HTML5 "special" element (§13.2.4.2).
func isSpecialElement(tag string) bool {
	switch tag {
	case "address", "applet", "area", "article", "aside", "base", "basefont",
		"bgsound", "blockquote", "body", "br", "button", "caption", "center",
		"col", "colgroup", "dd", "details", "dir", "div", "dl", "dt", "embed",
		"fieldset", "figcaption", "figure", "footer", "form", "frame",
		"frameset", "h1", "h2", "h3", "h4", "h5", "h6", "head", "header",
		"hgroup", "hr", "html", "iframe", "img", "input", "li", "link",
		"listing", "main", "marquee", "menu", "meta", "nav", "noembed",
		"noframes", "noscript", "object", "ol", "p", "param", "plaintext",
		"pre", "script", "search", "section", "select", "source", "style",
		"summary", "table", "tbody", "td", "template", "textarea", "tfoot",
		"th", "thead", "title", "tr", "track", "ul", "wbr", "xmp":
		return true
	}
	return false
}

// isFormattingElement reports whether tag is an HTML5 formatting element.
func isFormattingElement(tag string) bool {
	switch tag {
	case "a", "b", "big", "code", "em", "font", "i", "nobr", "s", "small",
		"strike", "strong", "tt", "u":
		return true
	}
	return false
}

// setDocumentEncodingFromMeta records the encoding label declared by a meta
// element. The parser currently tokenizes input as UTF-8, but exposing the
// recognized declaration is still required by Document.characterSet.
func (tb *treeBuilder) setDocumentEncodingFromMeta(n *Node) {
	if n == nil || tb.doc == nil {
		return
	}
	label := n.GetAttribute("charset")
	if label == "" {
		if !strings.EqualFold(strings.TrimSpace(n.GetAttribute("http-equiv")), "content-type") {
			return
		}
		_, params, err := mime.ParseMediaType(n.GetAttribute("content"))
		if err != nil {
			return
		}
		label = params["charset"]
	}
	if _, canonical := charset.Lookup(label); canonical != "" {
		if strings.HasPrefix(canonical, "iso-8859-") {
			canonical = strings.ToUpper(canonical)
		}
		tb.doc.CharacterSet = canonical
	}
}

// isScopeElementTag reports whether tag is a scope boundary for the regular
// "has element in scope" check.
func isScopeElementTag(tag string) bool {
	switch tag {
	case "applet", "caption", "html", "table", "td", "th",
		"marquee", "object", "template":
		return true
	}
	return false
}

func isButtonScopeElement(tag string) bool {
	return isScopeElementTag(tag) || tag == "button"
}

func isTableScopeElement(tag string) bool {
	switch tag {
	case "table", "html":
		return true
	}
	return false
}

func isListItemScopeElement(tag string) bool {
	return isScopeElementTag(tag) || tag == "ol" || tag == "ul"
}

// ---------------------------------------------------------------------------
// Scope-checking helpers.
// ---------------------------------------------------------------------------

func (tb *treeBuilder) hasInButtonScope(tag string) bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		n := tb.openStack[i]
		if n.Data == tag {
			return true
		}
		if isButtonScopeElement(n.Data) {
			return false
		}
	}
	return false
}

func (tb *treeBuilder) hasInListItemScope(tag string) bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		n := tb.openStack[i]
		if n.Data == tag {
			return true
		}
		if isListItemScopeElement(n.Data) {
			return false
		}
	}
	return false
}

func (tb *treeBuilder) hasInTableScope(tag string) bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		n := tb.openStack[i]
		if n.Data == tag {
			return true
		}
		if isTableScopeElement(n.Data) {
			return false
		}
	}
	return false
}

func (tb *treeBuilder) hasInSelectScope(tag string) bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		n := tb.openStack[i]
		if n.Data == tag {
			return true
		}
		switch n.Data {
		case "optgroup", "option":
			// not scope boundaries
		default:
			return false
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Active formatting elements list management (§13.2.4.2).
// ---------------------------------------------------------------------------

func (tb *treeBuilder) pushToActiveFormatting(n *Node, tag string) {
	// Noah's Ark clause: if there are already 3 entries with the same tag,
	// same attributes, remove the earliest.
	count := 0
	lastIdx := -1
	for i := len(tb.activeFormattingElems) - 1; i >= 0; i-- {
		e := &tb.activeFormattingElems[i]
		if e.marker {
			break
		}
		if e.tag == tag && sameAttrs(n.Attr, e.node.Attr) {
			count++
			lastIdx = i
		}
	}
	if count >= 3 && lastIdx >= 0 {
		tb.activeFormattingElems = append(tb.activeFormattingElems[:lastIdx], tb.activeFormattingElems[lastIdx+1:]...)
	}
	tb.activeFormattingElems = append(tb.activeFormattingElems, formatEntry{node: n, tag: tag})
}

func sameAttrs(a, b []Attribute) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}

func (tb *treeBuilder) pushMarker() {
	tb.activeFormattingElems = append(tb.activeFormattingElems, formatEntry{marker: true})
}

func (tb *treeBuilder) removeFromActiveFormatting(n *Node) {
	for i, e := range tb.activeFormattingElems {
		if e.node == n {
			tb.activeFormattingElems = append(tb.activeFormattingElems[:i], tb.activeFormattingElems[i+1:]...)
			return
		}
	}
}

func (tb *treeBuilder) inActiveFormatting(tag string) (*Node, int) {
	for i := len(tb.activeFormattingElems) - 1; i >= 0; i-- {
		e := &tb.activeFormattingElems[i]
		if e.marker {
			return nil, -1
		}
		if e.tag == tag {
			return e.node, i
		}
	}
	return nil, -1
}

func (tb *treeBuilder) activeFormattingIndex(n *Node) int {
	for i, e := range tb.activeFormattingElems {
		if e.node == n {
			return i
		}
	}
	return -1
}

func (tb *treeBuilder) clearActiveFormattingToMarker() {
	for len(tb.activeFormattingElems) > 0 {
		last := tb.activeFormattingElems[len(tb.activeFormattingElems)-1]
		tb.activeFormattingElems = tb.activeFormattingElems[:len(tb.activeFormattingElems)-1]
		if last.marker {
			return
		}
	}
}

// reconstructActiveFormattingElements re-opens any formatting elements that
// should be active before inserting content (§13.2.4.2).
func (tb *treeBuilder) reconstructActiveFormattingElements() {
	if len(tb.activeFormattingElems) == 0 {
		return
	}
	last := &tb.activeFormattingElems[len(tb.activeFormattingElems)-1]
	if last.marker {
		return
	}
	// Check if the last entry is already on the stack.
	for _, o := range tb.openStack {
		if o == last.node {
			return
		}
	}
	// Walk backwards to find the first entry that is on the stack or a marker.
	i := len(tb.activeFormattingElems) - 1
	for i > 0 {
		i--
		entry := &tb.activeFormattingElems[i]
		if entry.marker {
			i++
			break
		}
		onStack := false
		for _, o := range tb.openStack {
			if o == entry.node {
				onStack = true
				break
			}
		}
		if onStack {
			i++
			break
		}
	}
	// Now re-create elements from i to the end.
	for j := i; j < len(tb.activeFormattingElems); j++ {
		entry := &tb.activeFormattingElems[j]
		n := tb.insertElement(entry.tag, entry.node.Attr, false)
		entry.node = n
	}
}

// ---------------------------------------------------------------------------
// Implied end tags (§13.2.4.1).
// ---------------------------------------------------------------------------

func (tb *treeBuilder) generateImpliedEndTags() {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		switch top.Data {
		case "dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc":
			tb.pop()
		default:
			return
		}
	}
}

func (tb *treeBuilder) generateImpliedEndTagsExcept(except string) {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		if top.Data == except {
			return
		}
		switch top.Data {
		case "dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc":
			tb.pop()
		default:
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Table context clearing helpers.
// ---------------------------------------------------------------------------

func (tb *treeBuilder) clearStackToTableContext() {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		switch top.Data {
		case "table", "template", "html":
			return
		}
		tb.pop()
	}
}

func (tb *treeBuilder) clearStackToTableBodyContext() {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		switch top.Data {
		case "tbody", "thead", "tfoot", "template", "html":
			return
		}
		tb.pop()
	}
}

func (tb *treeBuilder) clearStackToTableRowContext() {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		switch top.Data {
		case "tr", "template", "html":
			return
		}
		tb.pop()
	}
}

// ---------------------------------------------------------------------------
// Miscellaneous helpers.
// ---------------------------------------------------------------------------

func (tb *treeBuilder) insertTemplateElement(attrs []Attribute, selfClose bool) *Node {
	elem := tb.insertElement("template", attrs, selfClose)
	if elem != nil {
		var mode string
		var clonable, delegatesFocus, nullCustomElementRegistry bool
		for _, a := range attrs {
			switch {
			case strings.EqualFold(a.Name, "shadowrootmode") || strings.EqualFold(a.Name, "shadowroot"):
				mode = strings.ToLower(a.Value)
				if mode != "closed" {
					mode = "open"
				}
			case strings.EqualFold(a.Name, "shadowrootclonable"):
				clonable = strings.EqualFold(a.Value, "true") || a.Value == ""
			case strings.EqualFold(a.Name, "shadowrootdelegatesfocus"):
				delegatesFocus = true
			case strings.EqualFold(a.Name, "shadowrootcustomelementregistry"):
				nullCustomElementRegistry = true
			}
		}
		if mode != "" && elem.Parent != nil {
			host := elem.Parent
			if host.ShadowRoot == nil {
				host.ShadowRoot = &ShadowRoot{
					Host:                                 host,
					Mode:                                 mode,
					Declarative:                          true,
					DelegatesFocus:                       delegatesFocus,
					SlotAssignment:                       "named",
					Clonable:                             clonable,
					CustomElementRegistryNull:            nullCustomElementRegistry,
					CustomElementRegistryNullDeclarative: nullCustomElementRegistry,
				}
			}
			if selfClose {
				host.RemoveChild(elem)
			}
		}
	}
	return elem
}

func (tb *treeBuilder) closeParagraph() {
	if tb.hasInButtonScope("p") {
		tb.generateImpliedEndTagsExcept("p")
		tb.popUntil("p")
	}
}

func (tb *treeBuilder) closeCell() {
	tb.generateImpliedEndTags()
	if tb.hasOpenTag("td") {
		tb.popUntil("td")
	} else if tb.hasOpenTag("th") {
		tb.popUntil("th")
	}
}

func (tb *treeBuilder) closeCellFor(tag string) {
	tb.generateImpliedEndTags()
	tb.popUntil(tag)
}

func (tb *treeBuilder) hasTableSectionInScope() bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		n := tb.openStack[i]
		switch n.Data {
		case "tbody", "thead", "tfoot":
			return true
		case "table", "html":
			return false
		}
	}
	return false
}

// resetInsertionMode determines the appropriate insertion mode based on the
// open-element stack (§13.2.6.1 "the appropriate end tag token" / reset).
func (tb *treeBuilder) resetInsertionMode() {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		node := tb.openStack[i]
		last := i == 0
		if last && tb.fragmentContext != nil {
			node = tb.fragmentContext
		}
		// In the real spec, if the element was created by the parser for
		// html, the context is the fragment case. We always have a full doc.
		switch node.Data {
		case "select":
			tb.mode = modeInSelect
			return
		case "td", "th":
			if !last {
				tb.mode = modeInCell
				return
			}
		case "tr":
			tb.mode = modeInRow
			return
		case "tbody", "thead", "tfoot":
			tb.mode = modeInTableBody
			return
		case "caption":
			tb.mode = modeInCaption
			return
		case "colgroup":
			tb.mode = modeInColumnGroup
			return
		case "table":
			tb.mode = modeInTable
			return
		case "template":
			if len(tb.templateStack) > 0 {
				tb.mode = tb.templateStack[len(tb.templateStack)-1]
			} else {
				tb.mode = modeInBody
			}
			return
		case "head":
			if !last {
				tb.mode = modeInHead
				return
			}
		case "body":
			tb.mode = modeInBody
			return
		case "frameset":
			tb.mode = modeInFrameset
			return
		case "html":
			if tb.headElem == nil {
				tb.mode = modeBeforeHead
			} else {
				tb.mode = modeAfterHead
			}
			return
		}
		if last {
			tb.mode = modeInBody
			return
		}
	}
}

// popUntilInBody pops elements until we reach a state consistent with "in body"
// processing after a </body> or </html>.
func (tb *treeBuilder) popUntilInBody() {
	// Generate implied end tags, then check stack.
	tb.generateImpliedEndTags()
}

// ---------------------------------------------------------------------------
// Adoption agency algorithm (§13.2.4.3).
// ---------------------------------------------------------------------------

func (tb *treeBuilder) adoptionAgency(tag string) {
	// Step 1: If the current node is an HTML element with the same tag name,
	// and it's in the active formatting list, pop it and remove from the list.
	current := tb.current()
	if current.Data == tag && tb.activeFormattingIndex(current) < 0 {
		tb.pop()
		return
	}

	// Run the algorithm for up to 8 iterations.
	for outer := 0; outer < 8; outer++ {
		// Step 3: Find the formatting element.
		fmtNode, fmtIdx := tb.inActiveFormatting(tag)
		if fmtNode == nil {
			// No formatting element; process as "any other end tag".
			tb.handleInBodyEndTagAnyOther(tag)
			return
		}

		// Step 4: If the formatting element is not on the stack, remove it
		// from the active formatting list and return.
		fmtStackIdx := -1
		for i, o := range tb.openStack {
			if o == fmtNode {
				fmtStackIdx = i
				break
			}
		}
		if fmtStackIdx < 0 {
			tb.activeFormattingElems = append(tb.activeFormattingElems[:fmtIdx], tb.activeFormattingElems[fmtIdx+1:]...)
			return
		}

		// Step 5: If the formatting element is on the stack but not in scope, ignore.
		if !tb.hasInScope(tag) {
			return
		}

		// Step 6: If the formatting element is not the current node, parse error (we continue).

		// Step 7-8: Find furthest block.
		furthestIdx := -1
		for i := fmtStackIdx + 1; i < len(tb.openStack); i++ {
			if isSpecialElement(tb.openStack[i].Data) {
				furthestIdx = i
				break
			}
		}
		if furthestIdx < 0 {
			// No furthest block: pop until we pop the formatting element.
			for len(tb.openStack) > 0 {
				popped := tb.pop()
				if popped == fmtNode {
					break
				}
			}
			tb.activeFormattingElems = append(tb.activeFormattingElems[:fmtIdx], tb.activeFormattingElems[fmtIdx+1:]...)
			return
		}

		furthestBlock := tb.openStack[furthestIdx]

		// Step 9-10: Common parent is the element above the formatting element on the stack.
		commonParent := tb.openStack[fmtStackIdx-1]

		// Step 11: Build a bookmark for the formatting element position.
		bookmark := fmtIdx

		// Step 12: Initialize node and inner loop counter.
		nodeIdx := furthestIdx
		lastNode := furthestBlock

		// Inner loop: up to 3 iterations.
		for inner := 0; inner < 3; inner++ {
			// Find the element above node on the stack.
			nodeIdx--
			if nodeIdx < fmtStackIdx {
				break
			}
			nodeAbove := tb.openStack[nodeIdx]
			if nodeAbove == fmtNode {
				break
			}

			// Check if nodeAbove is in active formatting elements.
			afIdx := tb.activeFormattingIndex(nodeAbove)
			if afIdx < 0 {
				// Not in active formatting: remove from stack, continue.
				tb.openStack = append(tb.openStack[:nodeIdx], tb.openStack[nodeIdx+1:]...)
				continue
			}

			// Create a new element with the same tag and attributes.
			newElem := tb.doc.NewElement(nodeAbove.Data)
			newElem.Attr = make([]Attribute, len(nodeAbove.Attr))
			copy(newElem.Attr, nodeAbove.Attr)

			// Update the active formatting elements list.
			tb.activeFormattingElems[afIdx] = formatEntry{node: newElem, tag: nodeAbove.Data}

			// Update the open element stack.
			tb.openStack[nodeIdx] = newElem

			if lastNode == furthestBlock {
				bookmark = afIdx + 1
			}
			newElem.AppendChild(lastNode)
			lastNode = newElem
		}

		// Step 13: Insert lastNode into the common parent, using the foster
		// parenting location for table ancestors. Remove it from its live parent
		// first because script may have moved it while the parser was paused. The node is
		// dropped if this insertion would create a host-including cycle, as the
		// DOM pre-insertion algorithm requires (including shadow/template roots).
		if lastNode.Parent != nil {
			lastNode.Parent.RemoveChild(lastNode)
		}
		insertParent, insertBefore := commonParent, (*Node)(nil)
		if isTableElement(commonParent) {
			insertParent, insertBefore = tb.fosterInsertionLocation()
		} else if commonParent.IsHTMLTemplate() {
			insertParent = commonParent.TemplateContent()
		}
		if !hostIncludingInclusiveAncestor(lastNode, insertParent) {
			insertAtFosterLocation(insertParent, insertBefore, lastNode)
		}

		// Step 14: Create a new formatting element.
		newFmt := tb.doc.NewElement(tag)
		// Copy attributes from the original formatting element.
		if fmtNode != nil {
			newFmt.Attr = make([]Attribute, len(fmtNode.Attr))
			copy(newFmt.Attr, fmtNode.Attr)
		}

		// Step 15: Move furthest block's children into the new formatting element.
		for child := furthestBlock.FirstChild; child != nil; {
			next := child.NextSibling
			furthestBlock.RemoveChild(child)
			newFmt.AppendChild(child)
			child = next
		}

		// Step 16: Append the new formatting element to the furthest block.
		furthestBlock.AppendChild(newFmt)

		// Step 17: Replace the old formatting element at the bookmark. The
		// inner loop may have moved the bookmark past cloned formatting entries.
		if oldIdx := tb.activeFormattingIndex(fmtNode); oldIdx >= 0 {
			tb.activeFormattingElems = append(tb.activeFormattingElems[:oldIdx], tb.activeFormattingElems[oldIdx+1:]...)
			if oldIdx < bookmark {
				bookmark--
			}
		}
		if bookmark < 0 {
			bookmark = 0
		}
		if bookmark > len(tb.activeFormattingElems) {
			bookmark = len(tb.activeFormattingElems)
		}
		tb.activeFormattingElems = append(tb.activeFormattingElems, formatEntry{})
		copy(tb.activeFormattingElems[bookmark+1:], tb.activeFormattingElems[bookmark:])
		tb.activeFormattingElems[bookmark] = formatEntry{node: newFmt, tag: tag}

		// Step 18: Remove the old formatting element from the stack and insert
		// the replacement immediately below the furthest block.
		for i, open := range tb.openStack {
			if open == fmtNode {
				tb.openStack = append(tb.openStack[:i], tb.openStack[i+1:]...)
				break
			}
		}
		for i, o := range tb.openStack {
			if o == furthestBlock {
				newStack := make([]*Node, 0, len(tb.openStack)+1)
				newStack = append(newStack, tb.openStack[:i+1]...)
				newStack = append(newStack, newFmt)
				newStack = append(newStack, tb.openStack[i+1:]...)
				tb.openStack = newStack
				break
			}
		}

	}
}

// hostIncludingInclusiveAncestor reports whether ancestor is node itself or
// reachable through the parent chain, crossing a template content fragment to
// its host. Shadow-root children already carry their host as Parent in this DOM
// representation. Cycles terminate conservatively.
func hostIncludingInclusiveAncestor(ancestor, node *Node) bool {
	seen := make(map[*Node]struct{})
	for current := node; current != nil; {
		if current == ancestor {
			return true
		}
		if _, exists := seen[current]; exists {
			return false
		}
		seen[current] = struct{}{}
		if current.Parent != nil {
			current = current.Parent
		} else if current.Type == NodeDocumentFragment {
			current = current.TemplateHost
		} else {
			current = nil
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Block-level start tag set (used by "in body" mode).
// ---------------------------------------------------------------------------

var blockLevelTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"center": true, "details": true, "dialog": true, "dir": true,
	"div": true, "dl": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "header": true, "hgroup": true,
	"main": true, "menu": true, "nav": true, "ol": true, "search": true,
	"section": true, "summary": true, "ul": true,
}

var headingTags = map[string]bool{
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// formattingStartTags are the formatting elements that get pushed to the
// active formatting elements list.
var formattingStartTags = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true,
	"font": true, "i": true, "nobr": true, "s": true, "small": true,
	"strike": true, "strong": true, "tt": true, "u": true,
}

// ---------------------------------------------------------------------------
// Insertion mode handlers.
// Each returns true if the token must be reprocessed in the (possibly changed)
// insertion mode, or false if the token was fully handled.
// ---------------------------------------------------------------------------

// handleInitial (§13.2.6.1).
func (tb *treeBuilder) handleInitial(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			return false // ignore whitespace
		}
		tb.quirksMode = true
		tb.doc.QuirksMode = true
	case tokenComment:
		// Insert comment as child of document.
		n := tb.doc.NewComment(data)
		tb.doc.Node.AppendChild(n)
		return false
	case tokenDoctype:
		// Process doctype.
		if !tb.allowNode(&tb.doc.Node) {
			return false
		}
		dt := tb.doc.NewDoctype(data)
		ids := parseDoctypeIdentifiers(data)
		dt.DoctypeName, dt.PublicID, dt.SystemID = ids.name, ids.publicID, ids.systemID
		// The tree builder pre-creates the html element so insertion modes can
		// always target it. A doctype token still belongs before that element.
		if tb.htmlElem != nil && tb.htmlElem.Parent == &tb.doc.Node {
			tb.doc.Node.InsertBefore(dt, tb.htmlElem)
		} else {
			tb.doc.Node.AppendChild(dt)
		}
		tb.doc.DocType = dt
		tb.quirksMode, tb.doc.LimitedQuirksMode = doctypeModes(data)
		tb.doc.QuirksMode = tb.quirksMode
		tb.mode = modeBeforeHTML
		return false
	case tokenStartTag:
		tb.quirksMode = true
		tb.doc.QuirksMode = true
		tb.mode = modeBeforeHTML
		return true // reprocess
	case tokenEndTag:
		tb.quirksMode = true
		tb.doc.QuirksMode = true
		tb.mode = modeBeforeHTML
		return true // reprocess
	}
	tb.mode = modeBeforeHTML
	return true
}

// handleBeforeHTML (§13.2.6.2).
func (tb *treeBuilder) handleBeforeHTML(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenDoctype:
		return false // ignore
	case tokenComment:
		n := tb.doc.NewComment(data)
		tb.doc.Node.AppendChild(n)
		return false
	case tokenChars:
		if !hasNonSpace(data) {
			return false // ignore whitespace
		}
		// Non-whitespace: fall through to "anything else".
	case tokenStartTag:
		if tag == "html" {
			if tb.htmlElem == nil {
				tb.insertElement("html", attrs, false)
			} else {
				tb.mergeAttrs(tb.htmlElem, attrs)
			}
			tb.mode = modeBeforeHead
			return false
		}
	case tokenEndTag:
		switch tag {
		case "head", "body", "html", "br", "p":
			// Act as if <html> was seen, then reprocess.
			if tb.htmlElem == nil {
				tb.insertElement("html", nil, false)
			}
			tb.mode = modeBeforeHead
			return true
		default:
			return false // ignore
		}
	}
	// "Anything else": act as if <html> start tag, then reprocess.
	if tb.htmlElem == nil {
		tb.insertElement("html", nil, false)
	}
	tb.mode = modeBeforeHead
	return true
}

// handleBeforeHead (§13.2.6.3).
func (tb *treeBuilder) handleBeforeHead(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			return false
		}
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		if tag == "head" {
			tb.insertElement("head", attrs, false)
			tb.mode = modeInHead
			return false
		}
	case tokenEndTag:
		switch tag {
		case "head", "body", "html", "br", "p":
			// Fall through to "anything else".
		default:
			return false // ignore
		}
	}
	// "Anything else": create head, switch to inHead, reprocess.
	tb.insertElement("head", nil, false)
	tb.mode = modeInHead
	return true
}

// handleInHead (§13.2.6.4).
func (tb *treeBuilder) handleInHead(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			tb.insertText(data)
			return false
		}
		// Non-whitespace: fall through to "anything else".
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		switch tag {
		case "html":
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		case "meta":
			n := tb.insertElement("meta", attrs, true)
			tb.setDocumentEncodingFromMeta(n)
			return false
		case "title":
			tb.insertElement("title", attrs, false)
			tb.prevMode = tb.mode
			tb.mode = modeText
			return false
		case "style":
			tb.insertElement("style", attrs, false)
			tb.prevMode = tb.mode
			tb.mode = modeText
			return false
		case "script":
			tb.insertElement("script", attrs, false)
			tb.prevMode = tb.mode
			tb.mode = modeText
			return false
		case "noframes":
			tb.insertElement("noframes", attrs, false)
			tb.prevMode = tb.mode
			tb.mode = modeText
			return false
		case "base", "link":
			tb.insertElement(tag, attrs, true)
			return false
		case "noscript":
			tb.insertElement("noscript", attrs, false)
			if tb.scriptingEnabled {
				// With scripting enabled, <noscript> in head uses the RAWTEXT
				// tokenizer state. With scripting disabled it uses the dedicated
				// in-head-noscript tree-construction mode instead.
				tb.prevMode = tb.mode
				tb.mode = modeText
			} else {
				tb.mode = modeInHeadNoscript
			}
			return false
		case "template":
			tb.insertTemplateElement(attrs, false)
			tb.pushMarker()
			tb.templateStack = append(tb.templateStack, modeInTemplate)
			tb.mode = modeInTemplate
			return false
		case "head":
			return false // ignore
		}
	case tokenEndTag:
		if tag == "head" {
			tb.pop()
			tb.mode = modeAfterHead
			return false
		}
		switch tag {
		case "body", "html", "br":
			// Fall through to "anything else".
		case "template":
			if tb.hasOpenTag("template") {
				// Find the template element.
				var templateElem *Node
				for i := len(tb.openStack) - 1; i >= 0; i-- {
					if tb.openStack[i].Data == "template" {
						templateElem = tb.openStack[i]
						break
					}
				}
				tb.generateImpliedEndTags()
				tb.popUntil("template")
				tb.clearActiveFormattingToMarker()
				if len(tb.templateStack) > 0 {
					tb.templateStack = tb.templateStack[:len(tb.templateStack)-1]
				}
				// If this was a shadow DOM template, move children to shadow root.
				if templateElem != nil {
					host := templateElem.Parent
					isShadowTemplate := false
					if host != nil && host.ShadowRoot != nil {
						for _, a := range templateElem.Attr {
							if strings.EqualFold(a.Name, "shadowrootmode") || strings.EqualFold(a.Name, "shadowroot") {
								isShadowTemplate = true
								break
							}
						}
						if isShadowTemplate {
							content := templateElem.TemplateContent()
							for child := content.FirstChild; child != nil; {
								next := child.NextSibling
								content.RemoveChild(child)
								host.ShadowRoot.AppendChild(child)
								child = next
							}
							host.RemoveChild(templateElem)
						}
					}
					if !isShadowTemplate {
						templateElem.TemplateContent()
					}
				}
				tb.resetInsertionMode()
			}
			return false
		default:
			return false // ignore
		}
	}
	// "Anything else": pop head, switch to afterHead, reprocess.
	tb.pop()
	tb.mode = modeAfterHead
	return true
}

// handleInHeadNoscript (§13.2.6.5).
func (tb *treeBuilder) handleInHeadNoscript(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenDoctype:
		return false
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		if tag == "noscript" {
			return false // ignore
		}
		if tag == "head" {
			return false // ignore in this insertion mode
		}
		if tag == "basefont" || tag == "bgsound" || tag == "link" || tag == "meta" || tag == "noframes" || tag == "style" {
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		}
	case tokenEndTag:
		if tag == "noscript" {
			tb.pop()
			tb.mode = modeInHead
			return false
		}
		if tag == "br" {
			// Fall through to "anything else".
		} else {
			return false // ignore
		}
	case tokenChars:
		if !hasNonSpace(data) {
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		}
	}
	// "Anything else": pop noscript, switch to inHead, reprocess.
	tb.pop()
	tb.mode = modeInHead
	return true
}

// handleAfterHead (§13.2.6.6).
func (tb *treeBuilder) handleAfterHead(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			tb.insertText(data)
			return false
		}
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		switch tag {
		case "html":
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		case "body":
			tb.insertElement("body", attrs, false)
			tb.mode = modeInBody
			return false
		case "frameset":
			tb.insertElement("frameset", attrs, false)
			tb.mode = modeInFrameset
			return false
		case "head":
			return false // ignore
		}
		// Other tags: create body, reprocess.
	case tokenEndTag:
		switch tag {
		case "body", "html", "br", "p":
			// Fall through to "anything else".
		default:
			return false // ignore
		}
	}
	// "Anything else": create body, switch to inBody, reprocess.
	tb.insertElement("body", nil, false)
	tb.mode = modeInBody
	return true
}

// handleInBody (§13.2.6.7) - the main content processing mode.
func (tb *treeBuilder) handleInBody(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		tb.reconstructActiveFormattingElements()
		tb.insertText(data)
		return false

	case tokenComment:
		tb.insertComment(data)
		return false

	case tokenDoctype:
		return false // ignore

	case tokenStartTag:
		return tb.handleInBodyStartTag(tag, attrs, selfClose)

	case tokenEndTag:
		return tb.handleInBodyEndTag(tag)
	}
	return false
}

func (tb *treeBuilder) handleInBodyStartTag(tag string, attrs []Attribute, selfClose bool) bool {
	switch tag {
	case "html":
		if tb.hasOpenTag("template") {
			return false
		}
		tb.mergeAttrs(tb.htmlElem, attrs)
		return false

	case "head", "frame", "frameset":
		// Both tokens are ignored in the in-body insertion mode. This matters
		// inside template contents too: the in-template mode delegates ordinary
		// tokens here, where head/body/frameset wrappers must not become nodes.
		return false

	case "base", "link", "meta":
		return tb.handleInHead(tokenStartTag, tag, attrs, selfClose, "")

	case "title":
		return tb.handleInHead(tokenStartTag, tag, attrs, selfClose, "")

	case "style", "script":
		return tb.handleInHead(tokenStartTag, tag, attrs, selfClose, "")

	case "template":
		return tb.handleInHead(tokenStartTag, tag, attrs, selfClose, "")

	case "body":
		// Fragment parsing has no document body pointer. The in-body rule only
		// merges attributes into the existing body element when one exists.
		if tb.bodyElem != nil {
			tb.mergeAttrs(tb.bodyElem, attrs)
		}
		return false

	case "address", "article", "aside", "blockquote", "center",
		"dir", "div", "dl", "figcaption", "figure",
		"footer", "header", "hgroup", "main", "nav", "ol", "search",
		"section", "summary", "ul":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		return false

	case "h1", "h2", "h3", "h4", "h5", "h6":
		tb.closeParagraph()
		// If current node is a heading, pop it.
		top := tb.current()
		if headingTags[top.Data] {
			tb.pop()
		}
		tb.insertElement(tag, attrs, false)
		return false

	case "p":
		tb.closeParagraph()
		tb.insertElement("p", attrs, false)
		return false

	case "pre", "listing":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		return false

	case "form":
		tb.closeParagraph()
		if len(tb.templateStack) == 0 && tb.formElem != nil {
			return false
		}
		elem := tb.insertElement("form", attrs, false)
		if len(tb.templateStack) == 0 {
			tb.formElem = elem
		}
		return false

	case "li":
		// Close previous li if in scope.
		for i := len(tb.openStack) - 1; i >= 0; i-- {
			n := tb.openStack[i]
			if n.Data == "li" {
				tb.generateImpliedEndTagsExcept("li")
				tb.popUntil("li")
				break
			}
			if isSpecialElement(n.Data) && n.Data != "address" && n.Data != "div" && n.Data != "p" {
				break
			}
		}
		tb.closeParagraph()
		tb.insertElement("li", attrs, false)
		return false

	case "dd", "dt":
		for i := len(tb.openStack) - 1; i >= 0; i-- {
			n := tb.openStack[i]
			if n.Data == "dd" {
				tb.generateImpliedEndTagsExcept("dd")
				tb.popUntil("dd")
				break
			}
			if n.Data == "dt" {
				tb.generateImpliedEndTagsExcept("dt")
				tb.popUntil("dt")
				break
			}
			if isSpecialElement(n.Data) && n.Data != "address" && n.Data != "div" && n.Data != "p" {
				break
			}
		}
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		return false

	case "table":
		tb.closeParagraph()
		tb.insertElement("table", attrs, false)
		tb.mode = modeInTable
		return false

	case "input":
		tb.reconstructActiveFormattingElements()
		tb.insertElement("input", attrs, true)
		return false

	case "br":
		tb.reconstructActiveFormattingElements()
		tb.insertElement("br", attrs, true)
		return false

	case "img", "hr", "embed", "wbr", "area", "source", "track":
		tb.reconstructActiveFormattingElements()
		tb.insertElement(tag, attrs, true)
		return false

	case "textarea":
		tb.insertElement("textarea", attrs, false)
		tb.prevMode = tb.mode
		tb.mode = modeText
		return false

	case "select":
		tb.reconstructActiveFormattingElements()
		tb.insertElement("select", attrs, false)
		tb.mode = modeInSelect
		return false

	case "option", "optgroup":
		if tb.current().Data == "option" {
			tb.pop()
			if tb.customizableSelectFor(tb.current()) != nil {
				tb.mode = modeInSelect
				return true
			}
		}
		tb.reconstructActiveFormattingElements()
		tb.insertElement(tag, attrs, false)
		if tag == "option" && tb.customizableSelectFor(tb.current()) != nil {
			tb.mode = modeInBody
		}
		return false

	case "button":
		if tb.hasInScope("button") {
			tb.generateImpliedEndTags()
			tb.popUntil("button")
		}
		tb.reconstructActiveFormattingElements()
		tb.insertElement("button", attrs, false)
		return false

	case "a", "b", "big", "code", "em", "font", "i", "s", "small",
		"strike", "strong", "tt", "u":
		tb.reconstructActiveFormattingElements()
		n := tb.insertElement(tag, attrs, false)
		if n != nil {
			tb.pushToActiveFormatting(n, tag)
		}
		return false

	case "nobr":
		tb.reconstructActiveFormattingElements()
		if tb.hasInScope("nobr") {
			tb.adoptionAgency("nobr")
			tb.reconstructActiveFormattingElements()
		}
		n := tb.insertElement("nobr", attrs, false)
		if n != nil {
			tb.pushToActiveFormatting(n, "nobr")
		}
		return false

	case "marquee", "object":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		tb.pushMarker()
		return false

	case "caption":
		tb.closeParagraph()
		tb.insertElement("caption", attrs, false)
		tb.mode = modeInCaption
		return false

	case "colgroup":
		tb.closeParagraph()
		tb.insertElement("colgroup", attrs, false)
		tb.mode = modeInColumnGroup
		return false

	case "col":
		// No colgroup open: create one.
		tb.closeParagraph()
		tb.insertElement("colgroup", nil, false)
		tb.mode = modeInColumnGroup
		return true // reprocess

	case "tbody", "thead", "tfoot":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		tb.mode = modeInTableBody
		return false

	case "tr":
		tb.closeParagraph()
		tb.insertElement("tbody", nil, false)
		tb.mode = modeInTableBody
		return true // reprocess

	case "td", "th":
		tb.closeParagraph()
		tb.insertElement("tbody", nil, false)
		tb.mode = modeInTableBody
		return true // reprocess

	case "fieldset":
		tb.closeParagraph()
		tb.insertElement("fieldset", attrs, false)
		return false

	case "legend":
		tb.reconstructActiveFormattingElements()
		tb.insertElement("legend", attrs, false)
		return false

	case "datalist", "label", "output", "progress", "meter":
		tb.reconstructActiveFormattingElements()
		tb.insertElement(tag, attrs, false)
		return false

	case "details", "dialog", "menu":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		return false

	case "iframe", "noembed", "noframes":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		tb.prevMode = tb.mode
		tb.mode = modeText
		return false

	case "xmp":
		tb.closeParagraph()
		tb.reconstructActiveFormattingElements()
		tb.insertElement("xmp", attrs, false)
		tb.prevMode = tb.mode
		tb.mode = modeText
		return false

	case "plaintext":
		tb.closeParagraph()
		tb.insertElement("plaintext", attrs, false)
		tb.prevMode = tb.mode
		tb.mode = modeText
		return false

	case "noscript":
		tb.insertElement("noscript", attrs, false)
		if tb.scriptingEnabled {
			tb.prevMode = tb.mode
			tb.mode = modeText
		}
		return false

	case "image":
		// Treat as <img>.
		tag = "img"
		tb.reconstructActiveFormattingElements()
		tb.insertElement("img", attrs, true)
		return false

	case "svg", "math":
		// Foreign roots enter their own namespace, after reconstructing any
		// active HTML formatting elements as required by the in-body mode.
		tb.reconstructActiveFormattingElements()
		tb.processStartTag(tag, attrs, selfClose)
		return false
	}

	// Default: any other start tag.
	tb.reconstructActiveFormattingElements()
	tb.insertElement(tag, attrs, selfClose)
	return false
}

func (tb *treeBuilder) handleInBodyEndTag(tag string) bool {
	switch tag {
	case "option":
		if tb.hasInScope("option") {
			tb.popUntil("option")
			if tb.customizableSelectFor(tb.current()) != nil {
				tb.resetInsertionMode()
			}
		}
		return false

	case "body":
		if tb.hasInScope("body") {
			tb.mode = modeAfterBody
		}
		return false

	case "html":
		if tb.hasInScope("body") {
			tb.mode = modeAfterBody
			return true // reprocess in afterBody
		}
		return false

	case "button":
		if tb.hasInScope("button") {
			tb.generateImpliedEndTags()
			tb.popUntil("button")
			if tb.current().Data == "select" {
				tb.resetInsertionMode()
			}
		}
		return false

	case "address", "article", "aside", "blockquote", "center",
		"details", "dialog", "dir", "div", "dl", "fieldset", "figcaption",
		"figure", "footer", "header", "hgroup", "listing", "main", "menu",
		"nav", "ol", "pre", "search", "section", "summary", "ul":
		if tb.hasInScope(tag) {
			tb.generateImpliedEndTags()
			tb.popUntil(tag)
		}
		return false

	case "form":
		if len(tb.templateStack) != 0 {
			if !tb.hasInScope("form") {
				return false
			}
			tb.generateImpliedEndTags()
			tb.popUntil("form")
			return false
		}
		if tb.formElem == nil {
			return false
		}
		form := tb.formElem
		tb.formElem = nil
		if !tb.hasInScope("form") {
			return false
		}
		tb.generateImpliedEndTags()
		tb.removeOpenElement(form)
		return false

	case "p":
		if !tb.hasInButtonScope("p") {
			tb.insertElement("p", nil, false)
		}
		tb.closeParagraph()
		return false

	case "li":
		if tb.hasInListItemScope("li") {
			tb.generateImpliedEndTagsExcept("li")
			tb.popUntil("li")
		}
		return false

	case "dd", "dt":
		if tb.hasInScope(tag) {
			tb.generateImpliedEndTagsExcept(tag)
			tb.popUntil(tag)
		}
		return false

	case "h1", "h2", "h3", "h4", "h5", "h6":
		// If any heading is in scope, generate implied end tags and pop until
		// we pop a heading element.
		for i := len(tb.openStack) - 1; i >= 0; i-- {
			n := tb.openStack[i]
			if headingTags[n.Data] {
				tb.generateImpliedEndTags()
				tb.popUntil(n.Data)
				return false
			}
			if isSpecialElement(n.Data) && !headingTags[n.Data] {
				return false // not in scope
			}
		}
		return false

	case "a", "b", "big", "code", "em", "font", "i", "s", "small",
		"strike", "strong", "tt", "u":
		tb.adoptionAgency(tag)
		return false

	case "applet", "marquee", "object":
		if tb.hasInScope(tag) {
			tb.generateImpliedEndTags()
			tb.popUntil(tag)
			tb.clearActiveFormattingToMarker()
		}
		return false

	case "br":
		// Act as <br> start tag.
		return tb.handleInBodyStartTag("br", nil, false)

	case "template":
		return tb.handleInHead(tokenEndTag, tag, nil, false, "")
	}

	// Any other end tag.
	tb.handleInBodyEndTagAnyOther(tag)
	return false
}

// customizableSelectFor returns the nearest select whose first element child
// is a button. In that content model, option descendants may contain markup.
func (tb *treeBuilder) customizableSelectFor(node *Node) *Node {
	for current := node; current != nil; current = current.Parent {
		if current.Data != "select" {
			continue
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != NodeElement {
				continue
			}
			if child.Data == "button" {
				return current
			}
			return nil
		}
		return nil
	}
	return nil
}

func (tb *treeBuilder) handleInBodyEndTagAnyOther(tag string) {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		node := tb.openStack[i]
		if node.Data == tag {
			tb.generateImpliedEndTagsExcept(tag)
			// Pop everything up to and including this element.
			for len(tb.openStack) > i {
				tb.pop()
			}
			return
		}
		if isSpecialElement(node.Data) {
			return // parse error, ignore
		}
	}
}

// handleTextMode (§13.2.6.4.7) - handles raw text element content.
func (tb *treeBuilder) handleTextMode(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		tb.insertText(data)
		return false
	case tokenEndTag:
		// Pop until we match the end tag, then restore mode.
		tb.pop()
		tb.mode = tb.prevMode
		return false
	}
	return false
}

// handleInTable (§13.2.6.8).
func (tb *treeBuilder) handleInTable(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		// Switch to "in table text" and reprocess.
		tb.prevMode = tb.mode
		tb.mode = modeInTableText
		return true

	case tokenComment:
		tb.insertComment(data)
		return false

	case tokenDoctype:
		return false

	case tokenStartTag:
		switch tag {
		case "template", "style", "script":
			// The in-table insertion mode delegates these tokens to the in-head
			// rules without enabling foster parenting. A template is a child of
			// the table here, while ordinary stray content is foster parented.
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		case "caption":
			tb.clearStackToTableContext()
			tb.pushMarker()
			tb.insertElement("caption", attrs, false)
			tb.mode = modeInCaption
			return false
		case "colgroup":
			tb.clearStackToTableContext()
			tb.insertElement("colgroup", attrs, false)
			tb.mode = modeInColumnGroup
			return false
		case "col":
			tb.clearStackToTableContext()
			tb.insertElement("colgroup", nil, false)
			tb.mode = modeInColumnGroup
			return true // reprocess
		case "tbody", "thead", "tfoot":
			tb.clearStackToTableContext()
			tb.insertElement(tag, attrs, false)
			tb.mode = modeInTableBody
			return false
		case "tr":
			tb.clearStackToTableContext()
			tb.insertElement("tbody", nil, false)
			tb.mode = modeInTableBody
			return true // reprocess
		case "td", "th":
			tb.clearStackToTableContext()
			tb.insertElement("tbody", nil, false)
			tb.mode = modeInTableBody
			return true // reprocess
		case "table":
			if tb.hasInTableScope("table") {
				tb.popUntil("table")
				tb.resetInsertionMode()
				return true // reprocess
			}
			return false // ignore
		case "input":
			// Only if type is "hidden".
			for _, a := range attrs {
				if strings.EqualFold(a.Name, "type") && strings.EqualFold(a.Value, "hidden") {
					tb.reconstructActiveFormattingElements()
					tb.insertElement("input", attrs, true)
					return false
				}
			}
			// Fall through to foster parenting.
		case "form":
			if tb.formElem != nil && len(tb.templateStack) == 0 {
				return false
			}
			tb.closeParagraph()
			form := tb.insertElement("form", attrs, false)
			if len(tb.templateStack) == 0 {
				tb.formElem = form
			}
			tb.removeOpenElement(form)
			return false
		}
		// "Anything else": foster parent.
		tb.fosterParent = true
		tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		tb.fosterParent = false
		return false

	case tokenEndTag:
		switch tag {
		case "table":
			if tb.hasInTableScope("table") {
				tb.popUntil("table")
				tb.resetInsertionMode()
			}
			return false
		case "body", "caption", "col", "colgroup", "html", "tbody", "td",
			"tfoot", "th", "thead", "tr":
			return false // ignore
		}
		// "Anything else": foster parent.
		tb.fosterParent = true
		tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		tb.fosterParent = false
		return false
	}
	return false
}

// handleInTableText (§13.2.6.9).
func (tb *treeBuilder) handleInTableText(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			tb.insertText(data)
		} else {
			// Foster-parent the text.
			tb.fosterParent = true
			tb.insertText(data)
			tb.fosterParent = false
		}
		return false
	}
	// Any other token: switch back to previous mode and reprocess.
	tb.mode = tb.prevMode
	return true
}

// handleInCaption (§13.2.6.10).
func (tb *treeBuilder) handleInCaption(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenEndTag:
		if tag == "caption" {
			if tb.hasInTableScope("caption") {
				tb.generateImpliedEndTags()
				tb.popUntil("caption")
				tb.clearActiveFormattingToMarker()
				tb.mode = modeInTable
			}
			return false
		}
		// Table-related end tags: close caption, reprocess.
		switch tag {
		case "table", "col", "colgroup", "tbody", "td", "tfoot", "th",
			"thead", "tr":
			if tb.hasInTableScope("caption") {
				tb.generateImpliedEndTags()
				tb.popUntil("caption")
				tb.clearActiveFormattingToMarker()
				tb.mode = modeInTable
				return true // reprocess
			}
			return false
		}
	case tokenStartTag:
		// Table-related start tags: close caption, reprocess.
		switch tag {
		case "caption", "col", "colgroup", "tbody", "td", "tfoot", "th",
			"thead", "tr":
			if tb.hasInTableScope("caption") {
				tb.generateImpliedEndTags()
				tb.popUntil("caption")
				tb.clearActiveFormattingToMarker()
				tb.mode = modeInTable
				return true // reprocess
			}
			return false
		}
	}
	// "Anything else": process as in body.
	return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
}

// handleInColumnGroup (§13.2.6.11).
func (tb *treeBuilder) handleInColumnGroup(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			tb.insertText(data)
			return false
		}
		// Fall through to "anything else".
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		switch tag {
		case "html":
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		case "col":
			tb.insertElement("col", attrs, true)
			return false
		}
	case tokenEndTag:
		if tag == "colgroup" {
			if tb.current().Data != "html" {
				tb.pop()
				tb.mode = modeInTable
			}
			return false
		}
		if tag == "col" {
			return false // ignore
		}
	}
	// "Anything else": if not html, pop colgroup, switch to inTable, reprocess.
	if tb.current().Data != "html" {
		tb.pop()
		tb.mode = modeInTable
		return true
	}
	return false
}

// handleInTableBody (§13.2.6.12).
func (tb *treeBuilder) handleInTableBody(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenStartTag:
		switch tag {
		case "tr":
			tb.clearStackToTableBodyContext()
			tb.insertElement("tr", attrs, false)
			tb.mode = modeInRow
			return false
		case "td", "th":
			tb.clearStackToTableBodyContext()
			tb.insertElement("tr", nil, false)
			tb.mode = modeInRow
			return true // reprocess
		case "caption", "col", "colgroup", "tbody", "tfoot", "thead":
			if tb.hasTableSectionInScope() {
				tb.clearStackToTableBodyContext()
				tb.pop() // pop tbody/thead/tfoot
				tb.mode = modeInTable
				return true // reprocess
			}
			return false
		}
	case tokenEndTag:
		switch tag {
		case "tbody", "thead", "tfoot":
			if tb.hasInTableScope(tag) {
				tb.clearStackToTableBodyContext()
				tb.pop()
				tb.mode = modeInTable
			}
			return false
		case "table":
			if tb.hasTableSectionInScope() {
				tb.clearStackToTableBodyContext()
				tb.pop()
				tb.mode = modeInTable
				return true // reprocess
			}
			return false
		case "body", "caption", "col", "colgroup", "html", "td", "th", "tr":
			return false // ignore
		}
	}
	// "Anything else": process as in table.
	return tb.handleInTable(tokenType, tag, attrs, selfClose, data)
}

// handleInRow (§13.2.6.13).
func (tb *treeBuilder) handleInRow(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenStartTag:
		switch tag {
		case "td", "th":
			tb.clearStackToTableRowContext()
			tb.insertElement(tag, attrs, false)
			tb.mode = modeInCell
			tb.pushMarker()
			return false
		case "caption", "col", "colgroup", "tbody", "tfoot", "thead", "tr":
			if tb.hasInTableScope("tr") {
				tb.clearStackToTableRowContext()
				tb.pop() // pop tr
				tb.mode = modeInTableBody
				return true // reprocess
			}
			return false
		}
	case tokenEndTag:
		switch tag {
		case "tr":
			if tb.hasInTableScope("tr") {
				tb.clearStackToTableRowContext()
				tb.pop()
				tb.mode = modeInTableBody
			}
			return false
		case "table":
			if tb.hasInTableScope("tr") {
				tb.clearStackToTableRowContext()
				tb.pop()
				tb.mode = modeInTableBody
				return true // reprocess
			}
			return false
		case "tbody", "thead", "tfoot":
			if tb.hasInTableScope(tag) {
				if tb.hasInTableScope("tr") {
					tb.clearStackToTableRowContext()
					tb.pop()
					tb.mode = modeInTableBody
					return true // reprocess
				}
			}
			return false
		case "body", "caption", "col", "colgroup", "html", "td", "th":
			return false // ignore
		}
	}
	// "Anything else": process as in table.
	return tb.handleInTable(tokenType, tag, attrs, selfClose, data)
}

// handleInCell (§13.2.6.14).
func (tb *treeBuilder) handleInCell(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenEndTag:
		switch tag {
		case "td", "th":
			if tb.hasInTableScope(tag) {
				tb.generateImpliedEndTags()
				tb.clearActiveFormattingToMarker()
				tb.popUntil(tag)
				tb.mode = modeInRow
			}
			return false
		case "body", "caption", "col", "colgroup", "html":
			return false // ignore
		case "table", "tbody", "tfoot", "thead", "tr":
			if tb.hasInTableScope(tag) {
				tb.closeCell()
				tb.mode = modeInRow
				return true // reprocess
			}
			return false
		}
	case tokenStartTag:
		switch tag {
		case "caption", "col", "colgroup", "tbody", "td", "tfoot", "th",
			"thead", "tr":
			if tb.hasInTableScope("td") || tb.hasInTableScope("th") {
				tb.closeCell()
				tb.mode = modeInRow
				return true // reprocess
			}
			return false
		}
	}
	// "Anything else": process as in body.
	return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
}

// handleInSelect (§13.2.6.15).
func (tb *treeBuilder) handleInSelect(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		tb.insertText(data)
		return false
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		switch tag {
		case "html":
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		case "option":
			if tb.current().Data == "option" {
				tb.pop()
			}
			tb.insertElement("option", attrs, false)
			if tb.customizableSelectFor(tb.current()) != nil {
				tb.mode = modeInBody
			}
			return false
		case "optgroup":
			if tb.current().Data == "option" {
				tb.pop()
			}
			if tb.current().Data == "optgroup" {
				tb.pop()
			}
			tb.insertElement("optgroup", attrs, false)
			return false
		case "select":
			// Close select and reprocess.
			tb.popUntil("select")
			tb.resetInsertionMode()
			return false
		case "button":
			// Customizable selects may begin with a select button containing a
			// selectedcontent element. Parse its descendants with the ordinary
			// in-body rules, then the closing button tag resets back to in-select.
			if tb.current().Data == "select" {
				tb.insertElement("button", attrs, false)
				tb.mode = modeInBody
			}
			return false
		case "input", "textarea":
			tb.popUntil("select")
			tb.resetInsertionMode()
			return true
		case "script", "template":
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		}
		return false // ignore
	case tokenEndTag:
		switch tag {
		case "optgroup":
			if tb.current().Data == "option" {
				tb.pop()
			}
			if tb.current().Data == "optgroup" {
				tb.pop()
			}
			return false
		case "option":
			if tb.current().Data == "option" {
				tb.pop()
			}
			return false
		case "select":
			if tb.hasInSelectScope("select") {
				tb.popUntil("select")
				tb.resetInsertionMode()
			}
			return false
		case "template":
			return tb.handleInHead(tokenEndTag, tag, nil, false, "")
		}
		return false // ignore
	}
	return false
}

// handleInSelectInTable (§13.2.6.16).
func (tb *treeBuilder) handleInSelectInTable(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenStartTag:
		switch tag {
		case "caption", "table", "tbody", "tfoot", "thead", "tr", "td", "th":
			tb.popUntil("select")
			tb.resetInsertionMode()
			return true
		}
	case tokenEndTag:
		switch tag {
		case "caption", "table", "tbody", "tfoot", "thead", "tr", "td", "th":
			if tb.hasInTableScope(tag) {
				tb.popUntil("select")
				tb.resetInsertionMode()
				return true
			}
			return false
		}
	}
	return tb.handleInSelect(tokenType, tag, attrs, selfClose, data)
}

// handleInTemplate (§13.2.6.17).
func (tb *treeBuilder) handleInTemplate(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
	case tokenComment:
		return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
	case tokenDoctype:
		return false
	case tokenStartTag:
		switch tag {
		case "style", "script":
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		case "caption", "colgroup", "tbody", "tfoot", "thead":
			tb.setTemplateInsertionMode(modeInTable)
			tb.mode = modeInTable
			return true
		case "col":
			tb.setTemplateInsertionMode(modeInColumnGroup)
			tb.mode = modeInColumnGroup
			return true
		case "tr":
			tb.setTemplateInsertionMode(modeInTableBody)
			tb.mode = modeInTableBody
			return true
		case "td", "th":
			tb.setTemplateInsertionMode(modeInRow)
			tb.mode = modeInRow
			return true
		default:
			tb.setTemplateInsertionMode(modeInBody)
			tb.mode = modeInBody
			return true
		}
	case tokenEndTag:
		if tag == "template" {
			return tb.handleInHead(tokenEndTag, tag, nil, false, "")
		}
		return false // ignore
	}
	// "Anything else": pop template, reset mode, reprocess.
	if tb.hasOpenTag("template") {
		tb.popUntil("template")
		tb.clearActiveFormattingToMarker()
		if len(tb.templateStack) > 0 {
			tb.templateStack = tb.templateStack[:len(tb.templateStack)-1]
		}
		tb.resetInsertionMode()
		return true
	}
	return false
}

// setTemplateInsertionMode updates the current template's insertion mode.
// The stack gets one entry when each <template> starts; tokens processed while
// that template is open replace its entry rather than growing the stack.
func (tb *treeBuilder) setTemplateInsertionMode(mode insertionMode) {
	if len(tb.templateStack) == 0 {
		tb.templateStack = append(tb.templateStack, mode)
		return
	}
	tb.templateStack[len(tb.templateStack)-1] = mode
}

// handleAfterBody (§13.2.6.18).
func (tb *treeBuilder) handleAfterBody(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		// Non-whitespace: reprocess in inBody.
		tb.mode = modeInBody
		return true
	case tokenComment:
		// Insert comment as child of html element.
		if tb.htmlElem != nil {
			n := tb.doc.NewComment(data)
			tb.htmlElem.AppendChild(n)
		}
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		tb.mode = modeInBody
		return true
	case tokenEndTag:
		if tag == "html" {
			tb.mode = modeAfterAfterBody
			return false
		}
		tb.mode = modeInBody
		return true
	}
	return false
}

// handleInFrameset (§13.2.6.19).
func (tb *treeBuilder) handleInFrameset(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		tb.insertText(data)
		return false
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		switch tag {
		case "html":
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		case "frameset":
			tb.insertElement("frameset", attrs, false)
			return false
		case "frame":
			tb.insertElement("frame", attrs, true)
			return false
		case "noframes":
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		}
		return false
	case tokenEndTag:
		if tag == "frameset" {
			if tb.current().Data != "html" {
				tb.pop()
				if tb.current().Data != "frameset" {
					tb.mode = modeAfterFrameset
				}
			}
			return false
		}
		return false
	}
	return false
}

// handleAfterFrameset (§13.2.6.20).
func (tb *treeBuilder) handleAfterFrameset(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			tb.insertText(data)
		}
		return false
	case tokenComment:
		tb.insertComment(data)
		return false
	case tokenDoctype:
		return false
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		if tag == "noframes" {
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		}
		return false
	case tokenEndTag:
		if tag == "html" {
			tb.mode = modeAfterAfterFrameset
			return false
		}
		return false
	}
	return false
}

// handleAfterAfterBody (§13.2.6.21).
func (tb *treeBuilder) handleAfterAfterBody(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenComment:
		n := tb.doc.NewComment(data)
		tb.doc.Node.AppendChild(n)
		return false
	case tokenDoctype:
		return false
	case tokenChars:
		if !hasNonSpace(data) {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		tb.mode = modeInBody
		return true
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		tb.mode = modeInBody
		return true
	case tokenEndTag:
		if tag == "html" {
			return false // ignore
		}
		tb.mode = modeInBody
		return true
	}
	return false
}

// handleAfterAfterFrameset (§13.2.6.22).
func (tb *treeBuilder) handleAfterAfterFrameset(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenComment:
		n := tb.doc.NewComment(data)
		tb.doc.Node.AppendChild(n)
		return false
	case tokenDoctype:
		return false
	case tokenChars:
		if !hasNonSpace(data) {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		tb.mode = modeInBody
		return true
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		tb.mode = modeInBody
		return true
	case tokenEndTag:
		return false
	}
	return false
}

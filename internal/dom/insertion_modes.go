package dom

import (
	"mime"
	"strings"

	"golang.org/x/net/html/charset"
)

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

func isFormattingElement(tag string) bool {
	switch tag {
	case "a", "b", "big", "code", "em", "font", "i", "nobr", "s", "small",
		"strike", "strong", "tt", "u":
		return true
	}
	return false
}

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
		default:
			return false
		}
	}
	return false
}

func (tb *treeBuilder) pushToActiveFormatting(n *Node, tag string) {
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

func (tb *treeBuilder) reconstructActiveFormattingElements() {
	if len(tb.activeFormattingElems) == 0 {
		return
	}
	last := &tb.activeFormattingElems[len(tb.activeFormattingElems)-1]
	if last.marker {
		return
	}
	for _, o := range tb.openStack {
		if o == last.node {
			return
		}
	}
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
	for j := i; j < len(tb.activeFormattingElems); j++ {
		entry := &tb.activeFormattingElems[j]
		n := tb.insertElement(entry.tag, entry.node.Attr, false)
		entry.node = n
	}
}

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

func (tb *treeBuilder) resetInsertionMode() {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		node := tb.openStack[i]
		last := i == 0
		if last && tb.fragmentContext != nil {
			node = tb.fragmentContext
		}
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

func (tb *treeBuilder) popUntilInBody() {
	tb.generateImpliedEndTags()
}

func (tb *treeBuilder) adoptionAgency(tag string) {
	current := tb.current()
	if current.Data == tag && tb.activeFormattingIndex(current) < 0 {
		tb.pop()
		return
	}

	for outer := 0; outer < 8; outer++ {
		fmtNode, fmtIdx := tb.inActiveFormatting(tag)
		if fmtNode == nil {
			tb.handleInBodyEndTagAnyOther(tag)
			return
		}

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

		if !tb.hasInScope(tag) {
			return
		}

		furthestIdx := -1
		for i := fmtStackIdx + 1; i < len(tb.openStack); i++ {
			if isSpecialElement(tb.openStack[i].Data) {
				furthestIdx = i
				break
			}
		}
		if furthestIdx < 0 {
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

		commonParent := tb.openStack[fmtStackIdx-1]

		bookmark := fmtIdx

		nodeIdx := furthestIdx
		lastNode := furthestBlock

		for inner := 0; inner < 3; inner++ {
			nodeIdx--
			if nodeIdx < fmtStackIdx {
				break
			}
			nodeAbove := tb.openStack[nodeIdx]
			if nodeAbove == fmtNode {
				break
			}

			afIdx := tb.activeFormattingIndex(nodeAbove)
			if afIdx < 0 {
				tb.openStack = append(tb.openStack[:nodeIdx], tb.openStack[nodeIdx+1:]...)
				continue
			}

			newElem := tb.doc.NewElement(nodeAbove.Data)
			newElem.Attr = make([]Attribute, len(nodeAbove.Attr))
			copy(newElem.Attr, nodeAbove.Attr)

			tb.activeFormattingElems[afIdx] = formatEntry{node: newElem, tag: nodeAbove.Data}

			tb.openStack[nodeIdx] = newElem

			if lastNode == furthestBlock {
				bookmark = afIdx + 1
			}
			newElem.AppendChild(lastNode)
			lastNode = newElem
		}

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

		newFmt := tb.doc.NewElement(tag)
		if fmtNode != nil {
			newFmt.Attr = make([]Attribute, len(fmtNode.Attr))
			copy(newFmt.Attr, fmtNode.Attr)
		}

		for child := furthestBlock.FirstChild; child != nil; {
			next := child.NextSibling
			furthestBlock.RemoveChild(child)
			newFmt.AppendChild(child)
			child = next
		}

		furthestBlock.AppendChild(newFmt)

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

var formattingStartTags = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true,
	"font": true, "i": true, "nobr": true, "s": true, "small": true,
	"strike": true, "strong": true, "tt": true, "u": true,
}

func (tb *treeBuilder) handleInitial(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			return false
		}
		tb.quirksMode = true
		tb.doc.QuirksMode = true
	case tokenComment:
		n := tb.doc.NewComment(data)
		tb.doc.Node.AppendChild(n)
		return false
	case tokenDoctype:
		if !tb.allowNode(&tb.doc.Node) {
			return false
		}
		dt := tb.doc.NewDoctype(data)
		ids := parseDoctypeIdentifiers(data)
		dt.DoctypeName, dt.PublicID, dt.SystemID = ids.name, ids.publicID, ids.systemID
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
		return true
	case tokenEndTag:
		tb.quirksMode = true
		tb.doc.QuirksMode = true
		tb.mode = modeBeforeHTML
		return true
	}
	tb.mode = modeBeforeHTML
	return true
}

func (tb *treeBuilder) handleBeforeHTML(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenDoctype:
		return false
	case tokenComment:
		n := tb.doc.NewComment(data)
		tb.doc.Node.AppendChild(n)
		return false
	case tokenChars:
		if !hasNonSpace(data) {
			return false
		}
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
			if tb.htmlElem == nil {
				tb.insertElement("html", nil, false)
			}
			tb.mode = modeBeforeHead
			return true
		default:
			return false
		}
	}
	if tb.htmlElem == nil {
		tb.insertElement("html", nil, false)
	}
	tb.mode = modeBeforeHead
	return true
}

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
		default:
			return false
		}
	}
	tb.insertElement("head", nil, false)
	tb.mode = modeInHead
	return true
}

func (tb *treeBuilder) handleInHead(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
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
			return false
		}
	case tokenEndTag:
		if tag == "head" {
			tb.pop()
			tb.mode = modeAfterHead
			return false
		}
		switch tag {
		case "body", "html", "br":
		case "template":
			if tb.hasOpenTag("template") {
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
			return false
		}
	}
	tb.pop()
	tb.mode = modeAfterHead
	return true
}

func (tb *treeBuilder) handleInHeadNoscript(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenDoctype:
		return false
	case tokenStartTag:
		if tag == "html" {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		if tag == "noscript" {
			return false
		}
		if tag == "head" {
			return false
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
		} else {
			return false
		}
	case tokenChars:
		if !hasNonSpace(data) {
			return tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		}
	}
	tb.pop()
	tb.mode = modeInHead
	return true
}

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
			return false
		}
	case tokenEndTag:
		switch tag {
		case "body", "html", "br", "p":
		default:
			return false
		}
	}
	tb.insertElement("body", nil, false)
	tb.mode = modeInBody
	return true
}

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
		return false

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
		tb.closeParagraph()
		tb.insertElement("colgroup", nil, false)
		tb.mode = modeInColumnGroup
		return true

	case "tbody", "thead", "tfoot":
		tb.closeParagraph()
		tb.insertElement(tag, attrs, false)
		tb.mode = modeInTableBody
		return false

	case "tr":
		tb.closeParagraph()
		tb.insertElement("tbody", nil, false)
		tb.mode = modeInTableBody
		return true

	case "td", "th":
		tb.closeParagraph()
		tb.insertElement("tbody", nil, false)
		tb.mode = modeInTableBody
		return true

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
		tag = "img"
		tb.reconstructActiveFormattingElements()
		tb.insertElement("img", attrs, true)
		return false

	case "svg", "math":
		tb.reconstructActiveFormattingElements()
		tb.processStartTag(tag, attrs, selfClose)
		return false
	}

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
			return true
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
		for i := len(tb.openStack) - 1; i >= 0; i-- {
			n := tb.openStack[i]
			if headingTags[n.Data] {
				tb.generateImpliedEndTags()
				tb.popUntil(n.Data)
				return false
			}
			if isSpecialElement(n.Data) && !headingTags[n.Data] {
				return false
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
		return tb.handleInBodyStartTag("br", nil, false)

	case "template":
		return tb.handleInHead(tokenEndTag, tag, nil, false, "")
	}

	tb.handleInBodyEndTagAnyOther(tag)
	return false
}

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
			for len(tb.openStack) > i {
				tb.pop()
			}
			return
		}
		if isSpecialElement(node.Data) {
			return
		}
	}
}

func (tb *treeBuilder) handleTextMode(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		tb.insertText(data)
		return false
	case tokenEndTag:
		tb.pop()
		tb.mode = tb.prevMode
		return false
	}
	return false
}

func (tb *treeBuilder) handleInTable(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
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
			return true
		case "tbody", "thead", "tfoot":
			tb.clearStackToTableContext()
			tb.insertElement(tag, attrs, false)
			tb.mode = modeInTableBody
			return false
		case "tr":
			tb.clearStackToTableContext()
			tb.insertElement("tbody", nil, false)
			tb.mode = modeInTableBody
			return true
		case "td", "th":
			tb.clearStackToTableContext()
			tb.insertElement("tbody", nil, false)
			tb.mode = modeInTableBody
			return true
		case "table":
			if tb.hasInTableScope("table") {
				tb.popUntil("table")
				tb.resetInsertionMode()
				return true
			}
			return false
		case "input":
			for _, a := range attrs {
				if strings.EqualFold(a.Name, "type") && strings.EqualFold(a.Value, "hidden") {
					tb.reconstructActiveFormattingElements()
					tb.insertElement("input", attrs, true)
					return false
				}
			}
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
			return false
		}
		tb.fosterParent = true
		tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		tb.fosterParent = false
		return false
	}
	return false
}

func (tb *treeBuilder) handleInTableText(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			tb.insertText(data)
		} else {
			tb.fosterParent = true
			tb.insertText(data)
			tb.fosterParent = false
		}
		return false
	}
	tb.mode = tb.prevMode
	return true
}

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
		switch tag {
		case "table", "col", "colgroup", "tbody", "td", "tfoot", "th",
			"thead", "tr":
			if tb.hasInTableScope("caption") {
				tb.generateImpliedEndTags()
				tb.popUntil("caption")
				tb.clearActiveFormattingToMarker()
				tb.mode = modeInTable
				return true
			}
			return false
		}
	case tokenStartTag:
		switch tag {
		case "caption", "col", "colgroup", "tbody", "td", "tfoot", "th",
			"thead", "tr":
			if tb.hasInTableScope("caption") {
				tb.generateImpliedEndTags()
				tb.popUntil("caption")
				tb.clearActiveFormattingToMarker()
				tb.mode = modeInTable
				return true
			}
			return false
		}
	}
	return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
}

func (tb *treeBuilder) handleInColumnGroup(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
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
			return false
		}
	}
	if tb.current().Data != "html" {
		tb.pop()
		tb.mode = modeInTable
		return true
	}
	return false
}

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
			return true
		case "caption", "col", "colgroup", "tbody", "tfoot", "thead":
			if tb.hasTableSectionInScope() {
				tb.clearStackToTableBodyContext()
				tb.pop()
				tb.mode = modeInTable
				return true
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
				return true
			}
			return false
		case "body", "caption", "col", "colgroup", "html", "td", "th", "tr":
			return false
		}
	}
	return tb.handleInTable(tokenType, tag, attrs, selfClose, data)
}

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
				tb.pop()
				tb.mode = modeInTableBody
				return true
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
				return true
			}
			return false
		case "tbody", "thead", "tfoot":
			if tb.hasInTableScope(tag) {
				if tb.hasInTableScope("tr") {
					tb.clearStackToTableRowContext()
					tb.pop()
					tb.mode = modeInTableBody
					return true
				}
			}
			return false
		case "body", "caption", "col", "colgroup", "html", "td", "th":
			return false
		}
	}
	return tb.handleInTable(tokenType, tag, attrs, selfClose, data)
}

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
			return false
		case "table", "tbody", "tfoot", "thead", "tr":
			if tb.hasInTableScope(tag) {
				tb.closeCell()
				tb.mode = modeInRow
				return true
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
				return true
			}
			return false
		}
	}
	return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
}

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
			tb.popUntil("select")
			tb.resetInsertionMode()
			return false
		case "button":
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
		return false
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
		return false
	}
	return false
}

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
		return false
	}
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

func (tb *treeBuilder) setTemplateInsertionMode(mode insertionMode) {
	if len(tb.templateStack) == 0 {
		tb.templateStack = append(tb.templateStack, mode)
		return
	}
	tb.templateStack[len(tb.templateStack)-1] = mode
}

func (tb *treeBuilder) handleAfterBody(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) bool {
	switch tokenType {
	case tokenChars:
		if !hasNonSpace(data) {
			return tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		}
		tb.mode = modeInBody
		return true
	case tokenComment:
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
			return false
		}
		tb.mode = modeInBody
		return true
	}
	return false
}

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

package dom

import (
	"fmt"
	"strings"
	"unicode"
)

// Parse parses an HTML document and returns the Document.
func Parse(html string) *Document {
	doc := NewDocument()
	tb := &treeBuilder{doc: doc}
	tb.parse(html)
	return doc
}

// ParseLimits bounds allocations while the actual HTML tokenizer builds the tree.
// Nodes includes the document and implicit elements; depth counts edges from it.
// Zero limits are only used by the legacy, unchecked Parse entry point.
type ParseLimits struct {
	Nodes, Depth, Attributes, AttributeBytes int
}

// ParseBounded is Parse with resource limits, checked before node allocation.
// The caller must bound the input byte length before calling it.
func ParseBounded(html string, limits ParseLimits) (*Document, error) {
	if limits.Nodes <= 0 || limits.Depth <= 0 || limits.Attributes <= 0 || limits.AttributeBytes <= 0 {
		return nil, fmt.Errorf("HTML parse limits must be positive")
	}
	tb := &treeBuilder{doc: NewDocument(), limits: limits, text: make(map[*Node]*strings.Builder)}
	tb.parse(html)
	if tb.err != nil {
		return nil, tb.err
	}
	for n, text := range tb.text {
		n.DataContent = text.String()
	}
	return tb.doc, nil
}

type treeBuilder struct {
	doc          *Document
	openStack    []*Node
	headElem     *Node
	bodyElem     *Node
	htmlElem     *Node
	fosterParent bool
	limits       ParseLimits
	err          error
	text         map[*Node]*strings.Builder
}

func (tb *treeBuilder) allowNode(parent *Node) bool {
	if tb.err != nil {
		return false
	}
	if tb.limits.Nodes == 0 {
		return true
	}
	if int(tb.doc.nextID) >= tb.limits.Nodes {
		tb.err = fmt.Errorf("HTML node limit exceeded (%d)", tb.limits.Nodes)
		return false
	}
	depth := 0
	for p := parent; p != nil; p = p.Parent {
		depth++
		if depth > tb.limits.Depth {
			tb.err = fmt.Errorf("HTML tree depth limit exceeded (%d)", tb.limits.Depth)
			return false
		}
	}
	return true
}

func (tb *treeBuilder) current() *Node {
	if len(tb.openStack) > 0 {
		return tb.openStack[len(tb.openStack)-1]
	}
	return &tb.doc.Node
}

func (tb *treeBuilder) push(n *Node) {
	tb.openStack = append(tb.openStack, n)
}

func (tb *treeBuilder) pop() *Node {
	if len(tb.openStack) == 0 {
		return nil
	}
	n := tb.openStack[len(tb.openStack)-1]
	tb.openStack = tb.openStack[:len(tb.openStack)-1]
	return n
}

func (tb *treeBuilder) popUntil(tag string) {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		tb.pop()
		if top.Data == tag {
			return
		}
	}
}

func (tb *treeBuilder) hasInScope(tag string) bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tb.openStack[i].Data == tag {
			return true
		}
		if isScopeElement(tb.openStack[i]) {
			return false
		}
	}
	return false
}

func isScopeElement(n *Node) bool {
	switch n.Data {
	case "applet", "caption", "html", "table", "td", "th",
		"marquee", "object", "template":
		return true
	}
	return false
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true,
	"embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

var rawTextElements = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true,
}

var rawTextEndTags = map[string]string{
	"script":   "</script",
	"style":    "</style",
	"textarea": "</textarea",
	"title":    "</title",
}

func (tb *treeBuilder) insertElement(tag string, attrs []Attribute, selfClose bool) *Node {
	parent := tb.current()
	if tb.fosterParent && isTableElement(parent) {
		parent = tb.findFosterParent()
	}
	if !tb.allowNode(parent) {
		return nil
	}
	n := tb.doc.NewElement(tag)
	n.Attr = attrs
	parent.AppendChild(n)

	if tag == "html" {
		tb.htmlElem = n
		tb.doc.HTML = n
	} else if tag == "head" {
		tb.headElem = n
		tb.doc.Head = n
	} else if tag == "body" {
		tb.bodyElem = n
		tb.doc.Body = n
	}

	if !selfClose && !voidElements[tag] {
		tb.push(n)
	}
	return n
}

func isTableElement(n *Node) bool {
	switch n.Data {
	case "table", "tbody", "thead", "tfoot", "tr":
		return true
	}
	return false
}

func (tb *treeBuilder) findFosterParent() *Node {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tb.openStack[i].Data == "table" {
			if i > 0 {
				return tb.openStack[i-1]
			}
			return tb.openStack[i]
		}
	}
	return &tb.doc.Node
}

func (tb *treeBuilder) insertText(data string) {
	if data == "" {
		return
	}
	parent := tb.current()
	if tb.fosterParent && isTableElement(parent) {
		parent = tb.findFosterParent()
	}
	if last := parent.LastChild; last != nil && last.Text() {
		if tb.text == nil {
			last.DataContent += data
		} else {
			b := tb.text[last]
			if b == nil {
				b = &strings.Builder{}
				b.WriteString(last.DataContent)
				tb.text[last] = b
			}
			b.WriteString(data)
		}
		return
	}
	if !tb.allowNode(parent) {
		return
	}
	n := tb.doc.NewText(data)
	parent.AppendChild(n)
}

func (tb *treeBuilder) insertComment(data string) {
	if !tb.allowNode(tb.current()) {
		return
	}
	n := tb.doc.NewComment(data)
	tb.current().AppendChild(n)
}

func (tb *treeBuilder) parse(html string) {
	p := &tokenizer{input: html, limits: tb.limits}

	for p.pos < len(p.input) && tb.err == nil {
		if p.pos < len(p.input) && p.input[p.pos] == '<' {
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '/' {
				tb.handleEndTag(p)
			} else if p.pos+1 < len(p.input) && p.input[p.pos+1] == '!' {
				tb.handleCommentOrDoctype(p)
			} else {
				tb.handleStartTag(p)
			}
		} else {
			tb.handleText(p)
		}
	}
}

func (tb *treeBuilder) handleText(p *tokenizer) {
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '<' {
		p.pos++
	}
	text := p.input[start:p.pos]
	text = normalizeWhitespace(text)
	if text != "" {
		tb.insertText(text)
	}
}

func normalizeWhitespace(s string) string {
	var sb strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				sb.WriteByte(' ')
				prevSpace = true
			}
		} else {
			sb.WriteRune(r)
			prevSpace = false
		}
	}
	return sb.String()
}

func (tb *treeBuilder) handleStartTag(p *tokenizer) {
	p.pos++
	if p.pos >= len(p.input) {
		tb.insertText("<")
		return
	}

	tag, attrs, selfClose := p.parseTag()
	if p.err != nil {
		tb.err = p.err
		return
	}
	if tag == "" {
		tb.insertText("<")
		return
	}

	tag = strings.ToLower(tag)

	tb.processStartTag(tag, attrs, selfClose)

	if rawTextElements[tag] {
		tb.consumeRawText(p, tag)
	}
}

func (tb *treeBuilder) consumeRawText(p *tokenizer, tag string) {
	endTag := rawTextEndTags[tag]
	start := p.pos
	for p.pos < len(p.input) {
		idx := strings.Index(p.input[p.pos:], endTag)
		if idx < 0 {
			p.pos = len(p.input)
			break
		}
		p.pos += idx
		endPos := p.pos + len(endTag)
		if endPos >= len(p.input) || p.input[endPos] == '>' || p.input[endPos] == ' ' {
			break
		}
		p.pos++
	}
	if p.pos > start {
		tb.insertText(p.input[start:p.pos])
	}
	if p.pos < len(p.input) {
		idx := strings.Index(p.input[p.pos:], ">")
		if idx >= 0 {
			p.pos += idx + 1
		} else {
			p.pos = len(p.input)
		}
	}
	tb.popUntil(tag)
}

func (tb *treeBuilder) processStartTag(tag string, attrs []Attribute, selfClose bool) {
	switch tag {
	case "html":
		if tb.htmlElem == nil {
			tb.insertElement(tag, attrs, false)
		}
		return
	case "head":
		if tb.headElem == nil && tb.htmlElem != nil {
			tb.insertElement(tag, attrs, false)
		}
		return
	case "body":
		if tb.bodyElem == nil && tb.headElem != nil {
			tb.insertElement(tag, attrs, false)
		} else if tb.bodyElem == nil && tb.htmlElem != nil {
			tb.insertElement(tag, attrs, false)
		}
		return
	case "meta", "link", "base":
		tb.ensureHead()
		tb.insertElement(tag, attrs, true)
		return
	case "title":
		tb.ensureHead()
		tb.insertElement(tag, attrs, false)
		return
	case "style":
		tb.ensureHead()
		tb.insertElement(tag, attrs, false)
		return
	case "script":
		tb.insertElement(tag, attrs, false)
		return
	case "br", "hr", "img", "input", "embed", "wbr", "col", "area", "source", "track":
		tb.ensureBody()
		tb.insertElement(tag, attrs, true)
		return
	case "p":
		tb.ensureBody()
		if tb.hasInScope("p") {
			tb.closeP()
		}
		tb.insertElement(tag, attrs, false)
		return
	case "li":
		tb.ensureBody()
		tb.closeListItem()
		tb.insertElement(tag, attrs, false)
		return
	case "dd", "dt":
		tb.ensureBody()
		tb.closeDefinition()
		tb.insertElement(tag, attrs, false)
		return
	case "tr":
		tb.ensureBody()
		tb.closeUntil("tbody", "thead", "tfoot")
		tb.insertElement(tag, attrs, false)
		return
	case "td", "th":
		tb.ensureBody()
		tb.closeUntil("td", "th")
		tb.insertElement(tag, attrs, false)
		return
	case "tbody", "tfoot", "thead":
		tb.ensureBody()
		tb.closeUntil("tbody", "thead", "tfoot")
		tb.insertElement(tag, attrs, false)
		return
	case "table":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "caption":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "colgroup":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "form", "fieldset", "details", "dialog", "menu":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "select":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "option", "optgroup":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "textarea":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "label", "button", "output", "progress", "meter":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	case "datalist":
		tb.ensureBody()
		tb.insertElement(tag, attrs, false)
		return
	default:
		tb.ensureBody()
		tb.insertElement(tag, attrs, selfClose)
		return
	}
}

func (tb *treeBuilder) ensureHead() {
	if tb.headElem == nil && tb.htmlElem != nil {
		tb.insertElement("head", nil, false)
	}
}

func (tb *treeBuilder) ensureBody() {
	if tb.bodyElem == nil {
		tb.ensureHead()
		if tb.headElem != nil {
			tb.pop()
		}
		tb.insertElement("body", nil, false)
	}
}

func (tb *treeBuilder) closeP() {
	tb.popUntil("p")
}

func (tb *treeBuilder) closeListItem() {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tb.openStack[i].Data == "li" {
			tb.popUntil("li")
			return
		}
		if isScopeElement(tb.openStack[i]) && tb.openStack[i].Data != "ul" && tb.openStack[i].Data != "ol" {
			return
		}
	}
}

func (tb *treeBuilder) closeDefinition() {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tb.openStack[i].Data == "dt" || tb.openStack[i].Data == "dd" {
			if tb.openStack[i].Data == "dd" {
				tb.popUntil("dd")
			} else {
				tb.popUntil("dt")
			}
			return
		}
		if isScopeElement(tb.openStack[i]) {
			return
		}
	}
}

func (tb *treeBuilder) closeUntil(tags ...string) {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tagSet[tb.openStack[i].Data] {
			return
		}
	}
}

func (tb *treeBuilder) handleEndTag(p *tokenizer) {
	p.pos += 2
	tag := p.readTagName()
	if tag == "" {
		return
	}
	p.skipTo('>')
	if p.pos < len(p.input) {
		p.pos++
	}

	tag = strings.ToLower(tag)
	tb.processEndTag(tag)
}

func (tb *treeBuilder) processEndTag(tag string) {
	switch tag {
	case "html":
		return
	case "head":
		if tb.headElem != nil {
			tb.popUntil("head")
		}
		return
	case "body":
		if tb.bodyElem != nil {
			tb.popUntil("body")
		}
		return
	case "p":
		if !tb.hasInScope("p") {
			tb.insertElement("p", nil, false)
		}
		tb.closeP()
		return
	case "li":
		tb.popUntil("li")
		return
	case "dd", "dt":
		tb.popUntil(tag)
		return
	case "td", "th":
		tb.popUntil(tag)
		return
	case "tr":
		tb.popUntil("tr")
		return
	case "tbody", "tfoot", "thead":
		tb.popUntil(tag)
		return
	case "table":
		tb.popUntil("table")
		return
	case "caption":
		tb.popUntil("caption")
		return
	case "colgroup":
		tb.popUntil("colgroup")
		return
	case "form", "fieldset", "details", "dialog", "menu":
		tb.popUntil(tag)
		return
	case "select":
		tb.popUntil("select")
		return
	case "option", "optgroup":
		tb.popUntil(tag)
		return
	case "textarea":
		tb.popUntil("textarea")
		return
	case "script", "style":
		tb.popUntil(tag)
		return
	default:
		tb.popUntil(tag)
		return
	}
}

func (tb *treeBuilder) handleCommentOrDoctype(p *tokenizer) {
	p.pos += 2
	if p.pos+1 < len(p.input) && p.input[p.pos] == '-' && p.input[p.pos+1] == '-' {
		p.pos += 2
		end := strings.Index(p.input[p.pos:], "-->")
		if end >= 0 {
			tb.insertComment(p.input[p.pos : p.pos+end])
			p.pos += end + 3
		} else {
			p.pos = len(p.input)
		}
		return
	}
	if len(p.input)-p.pos >= 7 && strings.EqualFold(p.input[p.pos:p.pos+7], "DOCTYPE") {
		p.pos += 7
		end := strings.Index(p.input[p.pos:], ">")
		if end >= 0 {
			if !tb.allowNode(&tb.doc.Node) {
				return
			}
			data := strings.TrimSpace(p.input[p.pos : p.pos+end])
			dt := tb.doc.NewDoctype(data)
			tb.doc.Node.AppendChild(dt)
			tb.doc.DocType = dt
			p.pos += end + 1
		} else {
			p.pos = len(p.input)
		}
		return
	}
	p.skipTo('>')
	if p.pos < len(p.input) {
		p.pos++
	}
}

type tokenizer struct {
	input  string
	pos    int
	limits ParseLimits
	err    error
}

func (t *tokenizer) readTagName() string {
	start := t.pos
	for t.pos < len(t.input) {
		c := t.input[t.pos]
		if c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		t.pos++
	}
	return t.input[start:t.pos]
}

func (t *tokenizer) skipTo(c byte) {
	for t.pos < len(t.input) && t.input[t.pos] != c {
		t.pos++
	}
}

func (t *tokenizer) parseTag() (tag string, attrs []Attribute, selfClose bool) {
	t.skipWhitespace()
	tag = t.readTagName()
	if tag == "" {
		t.skipTo('>')
		if t.pos < len(t.input) {
			t.pos++
		}
		return "", nil, false
	}

	for t.pos < len(t.input) {
		t.skipWhitespace()
		if t.pos >= len(t.input) {
			break
		}
		c := t.input[t.pos]
		if c == '>' {
			t.pos++
			return
		}
		if c == '/' {
			t.pos++
			if t.pos < len(t.input) && t.input[t.pos] == '>' {
				t.pos++
			}
			selfClose = true
			return
		}
		name, value := t.parseAttr()
		if name != "" {
			if t.limits.Attributes > 0 && len(attrs) >= t.limits.Attributes {
				t.err = fmt.Errorf("HTML attribute count limit exceeded (%d)", t.limits.Attributes)
				return
			}
			if t.limits.AttributeBytes > 0 && len(name)+len(value) > t.limits.AttributeBytes {
				t.err = fmt.Errorf("HTML attribute byte limit exceeded (%d)", t.limits.AttributeBytes)
				return
			}
			attrs = append(attrs, Attribute{
				Name:     name,
				NameAtom: LookupAttr(name),
				Value:    value,
			})
		}
	}
	return
}

func (t *tokenizer) skipWhitespace() {
	for t.pos < len(t.input) {
		c := t.input[t.pos]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '\f' {
			break
		}
		t.pos++
	}
}

func (t *tokenizer) parseAttr() (name, value string) {
	start := t.pos
	for t.pos < len(t.input) {
		c := t.input[t.pos]
		if c == '=' || c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		t.pos++
	}
	name = strings.ToLower(t.input[start:t.pos])
	if name == "" {
		if t.pos < len(t.input) {
			t.pos++
		}
		return "", ""
	}

	t.skipWhitespace()
	if t.pos >= len(t.input) || t.input[t.pos] != '=' {
		return name, ""
	}
	t.pos++
	t.skipWhitespace()

	if t.pos >= len(t.input) {
		return name, ""
	}

	quote := t.input[t.pos]
	if quote == '"' || quote == '\'' {
		t.pos++
		start = t.pos
		for t.pos < len(t.input) && t.input[t.pos] != quote {
			t.pos++
		}
		value = t.input[start:t.pos]
		if t.pos < len(t.input) {
			t.pos++
		}
	} else {
		start = t.pos
		for t.pos < len(t.input) {
			c := t.input[t.pos]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' {
				break
			}
			t.pos++
		}
		value = t.input[start:t.pos]
	}
	return name, value
}

package dom

import (
	"fmt"
	"strings"

	xhtml "golang.org/x/net/html"
)

type ParseLimits struct {
	Nodes, Depth, Attributes, AttributeBytes int
}

type doctypeIdentifiers struct {
	name, publicID, systemID string
	hasPublic, hasSystem     bool
	malformed                bool
}

func parseDoctypeIdentifiers(data string) doctypeIdentifiers {
	var parts doctypeIdentifiers
	i := 0
	next := func() (string, bool) {
		for i < len(data) && isHTMLSpace(data[i]) {
			i++
		}
		if i >= len(data) {
			return "", false
		}
		if data[i] == '\'' || data[i] == '"' {
			quote := data[i]
			i++
			start := i
			for i < len(data) && data[i] != quote {
				i++
			}
			value := data[start:i]
			if i >= len(data) {
				return value, false
			}
			i++
			return value, true
		}
		start := i
		for i < len(data) && !isHTMLSpace(data[i]) {
			i++
		}
		return data[start:i], true
	}
	var ok bool
	parts.name, ok = next()
	if !ok {
		parts.malformed = true
		return parts
	}
	keyword, ok := next()
	if !ok {
		return parts
	}
	switch strings.ToLower(keyword) {
	case "public":
		parts.hasPublic = true
		parts.publicID, ok = next()
		if !ok {
			parts.malformed = true
			return parts
		}
		rest, exists := next()
		if exists {
			parts.hasSystem = true
			parts.systemID = rest
		}
	case "system":
		parts.hasSystem = true
		parts.systemID, ok = next()
		if !ok {
			parts.malformed = true
		}
	default:
		parts.malformed = true
	}
	return parts
}

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func doctypeModes(data string) (quirks, limited bool) {
	d := parseDoctypeIdentifiers(data)
	name := strings.ToLower(d.name)
	if d.malformed || name != "html" {
		return true, false
	}
	if !d.hasPublic {
		return false, false
	}
	public := strings.ToLower(d.publicID)
	quirksPrefixes := []string{
		"+//silmaril//dtd html pro v0r11 19970101//",
		"-//advasoft ltd//dtd html 3.0 aswedit + extensions//",
		"-//as//dtd html 3.0 aswedit + extensions//",
		"-//ietf//dtd html 2.0",
		"-//ietf//dtd html 3.0",
		"-//ietf//dtd html 3.2",
		"-//microsoft//dtd internet explorer 2.0 html",
		"-//netscape comm. corp.//dtd html",
		"-//o'reilly and associates//dtd html",
		"-//softquad software//dtd hotmetal pro 6.0::19990601::extensions to html 4.0//",
		"-//w3c//dtd html 3 ", "-//w3c//dtd html 4.0 frameset//", "-//w3c//dtd html 4.0 transitional//",
		"-//w3c//dtd html experimental 19960712//", "-//w3c//dtd html experimental 970421//",
		"-//w3c//dtd w3 html//", "-//w3o//dtd w3 html 3.0//", "-//webtechs//dtd mozilla html 2.0//",
	}
	if public == "-//w3o//dtd w3 html strict 3.0//en//" || public == "-/w3c/dtd html 4.0 transitional/en" || public == "html" {
		return true, false
	}
	for _, prefix := range quirksPrefixes {
		if strings.HasPrefix(public, prefix) {
			return true, false
		}
	}
	if strings.HasPrefix(public, "-//w3c//dtd html 4.01 frameset//") || strings.HasPrefix(public, "-//w3c//dtd html 4.01 transitional//") {
		if !d.hasSystem || d.systemID == "" {
			return true, false
		}
		return false, true
	}
	if strings.HasPrefix(public, "-//w3c//dtd xhtml 1.0 frameset//") || strings.HasPrefix(public, "-//w3c//dtd xhtml 1.0 transitional//") {
		return false, true
	}
	return false, false
}

func ParseBounded(html string, limits ParseLimits) (*Document, error) {
	return ParseBoundedWithScriptHandler(html, limits, nil)
}

func ParseBoundedWithScriptHandler(html string, limits ParseLimits, onScript func(*Document, *Node)) (*Document, error) {
	return ParseBoundedWithHandlers(html, limits, nil, nil, onScript)
}

func ParseBoundedWithHandlers(html string, limits ParseLimits, onElement, onInserted func(*Node), onScript func(*Document, *Node)) (*Document, error) {
	return ParseBoundedWithParserCallbacks(html, limits, func(node *Node, _ []Attribute) *Node {
		if onElement != nil {
			onElement(node)
		}
		return node
	}, nil, onInserted, onScript)
}

func ParseBoundedWithParserCallbacks(html string, limits ParseLimits, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node), onScript func(*Document, *Node)) (*Document, error) {
	return ParseBoundedWithParserCallbacksAndScripting(html, limits, true, onElement, onAttribute, onInserted, onScript)
}

func ParseBoundedWithParserCallbacksAndScripting(html string, limits ParseLimits, scriptingEnabled bool, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node), onScript func(*Document, *Node)) (*Document, error) {
	return parseDocumentBounded(html, limits, true, scriptingEnabled, onElement, onAttribute, onInserted, onScript)
}

func ParseBoundedWithHandlersAndScripting(html string, limits ParseLimits, scriptingEnabled bool, onElement, onInserted func(*Node), onScript func(*Document, *Node)) (*Document, error) {
	return ParseBoundedWithParserCallbacksAndScripting(html, limits, scriptingEnabled, func(node *Node, _ []Attribute) *Node {
		if onElement != nil {
			onElement(node)
		}
		return node
	}, nil, onInserted, onScript)
}

func ParseOpenBounded(html string, limits ParseLimits) (*Document, error) {
	return parseDocumentBounded(html, limits, false, true, nil, nil, nil, nil)
}

func ParseOpenBoundedWithParserCallbacks(html string, limits ParseLimits, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node)) (*Document, error) {
	return parseDocumentBounded(html, limits, false, true, onElement, onAttribute, onInserted, nil)
}

func ParseOpenBoundedWithParserCallbacksAndScripts(html string, limits ParseLimits, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node), onScript func(*Document, *Node)) (*Document, error) {
	return parseDocumentBounded(html, limits, false, true, onElement, onAttribute, onInserted, onScript)
}

func parseDocumentBounded(html string, limits ParseLimits, complete, scriptingEnabled bool, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node), onScript func(*Document, *Node)) (*Document, error) {
	if limits.Nodes <= 0 || limits.Depth <= 0 || limits.Attributes <= 0 || limits.AttributeBytes <= 0 {
		return nil, fmt.Errorf("HTML parse limits must be positive")
	}
	tb := &treeBuilder{
		doc:               NewDocument(),
		limits:            limits,
		text:              make(map[*Node]*strings.Builder),
		mode:              modeInitial,
		scriptingEnabled:  scriptingEnabled,
		onParserElement:   onElement,
		onParserAttribute: onAttribute,
		onParserInserted:  onInserted,
		onParserScript:    onScript,
	}
	tb.parse(html)
	if complete && tb.doc.DocType == nil {
		tb.quirksMode = true
		tb.doc.QuirksMode = true
	}
	if complete && tb.err == nil && tb.htmlElem == nil {
		tb.insertElement("html", nil, false)
	}
	if complete && tb.err == nil && tb.fragmentContext == nil && tb.bodyElem == nil && !tb.documentHasFrameset() {
		tb.ensureBody()
	}
	if tb.err != nil {
		return nil, tb.err
	}
	for n, text := range tb.text {
		n.DataContent = text.String()
	}
	return tb.doc, nil
}

func (tb *treeBuilder) documentHasFrameset() bool {
	if tb == nil || tb.htmlElem == nil {
		return false
	}
	for child := tb.htmlElem.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && child.Data == "frameset" {
			return true
		}
	}
	return false
}

func ParseFragmentBounded(context *Node, input string, limits ParseLimits) (*Node, error) {
	return ParseFragmentBoundedWithParserCallbacks(context, input, limits, nil, nil, nil)
}

func ParseFragmentBoundedWithParserCallbacks(context *Node, input string, limits ParseLimits, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node)) (*Node, error) {
	fragment, _, err := ParseFragmentBoundedWithParserCallbacksAndState(context, input, limits, onElement, onAttribute, onInserted)
	return fragment, err
}

func ParseFragmentBoundedWithParserCallbacksAndState(context *Node, input string, limits ParseLimits, onElement func(*Node, []Attribute) *Node, onAttribute func(*Node, Attribute), onInserted func(*Node)) (*Node, bool, error) {
	if context == nil || !context.Element() {
		return nil, false, fmt.Errorf("HTML fragment parsing requires an element context")
	}
	if limits.Nodes <= 0 || limits.Depth <= 0 || limits.Attributes <= 0 || limits.AttributeBytes <= 0 {
		return nil, false, fmt.Errorf("HTML parse limits must be positive")
	}
	doc := NewDocument()
	contextNode := doc.NewElement(context.Data)
	contextNode.Namespace = context.Namespace
	contextNode.NamespaceURI = context.NamespaceURI
	contextNode.Data = context.Data
	contextNode.DataAtom = context.DataAtom
	if len(context.Attr) != 0 {
		contextNode.Attr = append([]Attribute(nil), context.Attr...)
	}
	root := doc.NewElement("html")
	tb := &treeBuilder{
		doc:               doc,
		limits:            limits,
		text:              make(map[*Node]*strings.Builder),
		mode:              modeInBody,
		scriptingEnabled:  true,
		htmlElem:          root,
		openStack:         []*Node{root},
		fragmentContext:   contextNode,
		onParserElement:   onElement,
		onParserAttribute: onAttribute,
		onParserInserted:  onInserted,
	}
	if contextNode.Namespace == NSSVG {
		tb.svgDepth = 1
	}
	if contextNode.IsHTMLTemplate() {
		tb.templateStack = []insertionMode{modeInTemplate}
	}
	for ancestor := context.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Element() && ancestor.Namespace == NSHTML && ancestor.Data == "form" {
			tb.formElem = ancestor
			break
		}
	}
	tb.resetInsertionMode()
	tb.parse(input)
	if tb.err != nil {
		return nil, false, tb.err
	}
	complete := len(tb.openStack) == 1 && completeHTMLInputBoundary(input)
	for n, text := range tb.text {
		n.DataContent = text.String()
	}
	fragment := doc.NewDocumentFragment()
	for child := root.FirstChild; child != nil; {
		next := child.NextSibling
		root.RemoveChild(child)
		fragment.AppendChild(child)
		child = next
	}
	return fragment, complete, nil
}

func completeHTMLInputBoundary(input string) bool {
	lastOpen, lastClose := strings.LastIndexByte(input, '<'), strings.LastIndexByte(input, '>')
	if lastOpen > lastClose {
		tail := input[lastOpen:]
		if len(tail) == 1 || strings.HasPrefix(tail, "</") || strings.HasPrefix(tail, "<!") || strings.HasPrefix(tail, "<?") ||
			(len(tail) > 1 && isASCIILetter(tail[1])) {
			return false
		}
	}
	if strings.LastIndex(input, "<!--") > strings.LastIndex(input, "-->") ||
		strings.LastIndex(input, "<![CDATA[") > strings.LastIndex(input, "]]>") {
		return false
	}
	if amp := strings.LastIndexByte(input, '&'); amp >= 0 {
		if semi := strings.LastIndexByte(input, ';'); amp > semi {
			tail := input[amp+1:]
			if tail == "" || tail[0] == '#' || isASCIILetter(tail[0]) || (tail[0] >= '0' && tail[0] <= '9') {
				return false
			}
		}
	}
	return true
}

func HTMLFragmentInputComplete(context *Node, input string, limits ParseLimits) (bool, error) {
	_, complete, err := ParseFragmentBoundedWithParserCallbacksAndState(context, input, limits, nil, nil, nil)
	return complete, err
}

type insertionMode int

const (
	modeInitial insertionMode = iota
	modeBeforeHTML
	modeBeforeHead
	modeInHead
	modeInHeadNoscript
	modeAfterHead
	modeInBody
	modeText
	modeInTable
	modeInTableText
	modeInCaption
	modeInColumnGroup
	modeInTableBody
	modeInRow
	modeInCell
	modeInSelect
	modeInSelectInTable
	modeInTemplate
	modeAfterBody
	modeInFrameset
	modeAfterFrameset
	modeAfterAfterBody
	modeAfterAfterFrameset
)

const (
	tokenDoctype  = 0
	tokenStartTag = 1
	tokenEndTag   = 2
	tokenComment  = 3
	tokenChars    = 4
)

type formatEntry struct {
	node   *Node
	tag    string
	marker bool
}

type treeBuilder struct {
	doc                   *Document
	openStack             []*Node
	headElem              *Node
	bodyElem              *Node
	htmlElem              *Node
	fosterParent          bool
	limits                ParseLimits
	err                   error
	text                  map[*Node]*strings.Builder
	svgDepth              int
	mode                  insertionMode
	scriptingEnabled      bool
	prevMode              insertionMode
	activeFormattingElems []formatEntry
	formElem              *Node
	parserRadioGroups     map[parserRadioGroupKey]*Node
	quirksMode            bool
	templateStack         []insertionMode
	fragmentContext       *Node
	onParserElement       func(*Node, []Attribute) *Node
	onParserAttribute     func(*Node, Attribute)
	onParserInserted      func(*Node)
	onParserScript        func(*Document, *Node)
}

type parserRadioGroupKey struct {
	owner *Node
	name  string
}

func (tb *treeBuilder) allowNode(parent *Node) bool {
	if tb.err != nil {
		return false
	}
	if int(tb.doc.nextID) >= tb.limits.Nodes {
		tb.err = fmt.Errorf("HTML node limit exceeded (%d)", tb.limits.Nodes)
		return false
	}
	if parent == tb.current() || parent == &tb.doc.Node {
		depth := 1
		if parent != &tb.doc.Node {
			depth = len(tb.openStack) + 1
		}
		if depth > tb.limits.Depth {
			tb.err = fmt.Errorf("HTML tree depth limit exceeded (%d)", tb.limits.Depth)
			return false
		}
		return true
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

func (tb *treeBuilder) adjustedCurrent() *Node {
	if tb.fragmentContext != nil && len(tb.openStack) == 1 {
		return tb.fragmentContext
	}
	return tb.current()
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
	if !tb.hasOpenTag(tag) {
		return
	}
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		tb.pop()
		if top.Data == tag {
			return
		}
	}
}

func (tb *treeBuilder) removeOpenElement(node *Node) {
	if tb == nil || node == nil {
		return
	}
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tb.openStack[i] == node {
			tb.openStack = append(tb.openStack[:i], tb.openStack[i+1:]...)
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

var svgTagAdjustments = map[string]string{
	"altglyph": "altGlyph", "altglyphdef": "altGlyphDef", "altglyphitem": "altGlyphItem",
	"animatecolor": "animateColor", "animatemotion": "animateMotion", "animatetransform": "animateTransform",
	"clippath": "clipPath", "feblend": "feBlend", "fecolormatrix": "feColorMatrix",
	"fecomponenttransfer": "feComponentTransfer", "fecomposite": "feComposite", "feconvolvematrix": "feConvolveMatrix",
	"fediffuselighting": "feDiffuseLighting", "fedisplacementmap": "feDisplacementMap", "fedistantlight": "feDistantLight",
	"feflood": "feFlood", "fefunca": "feFuncA", "fefuncb": "feFuncB", "fefuncg": "feFuncG", "fefuncr": "feFuncR",
	"fegaussianblur": "feGaussianBlur", "feimage": "feImage", "femerge": "feMerge", "femergenode": "feMergeNode",
	"femorphology": "feMorphology", "feoffset": "feOffset", "fepointlight": "fePointLight",
	"fespecularlighting": "feSpecularLighting", "fespotlight": "feSpotLight", "fetile": "feTile",
	"feturbulence": "feTurbulence", "foreignobject": "foreignObject", "glyphref": "glyphRef",
	"lineargradient": "linearGradient", "radialgradient": "radialGradient", "textpath": "textPath",
}

var foreignBreakoutElements = map[string]bool{
	"b": true, "big": true, "blockquote": true, "body": true, "br": true, "center": true,
	"code": true, "dd": true, "div": true, "dl": true, "dt": true, "em": true, "embed": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"head": true, "hr": true, "i": true, "img": true, "li": true, "listing": true,
	"menu": true, "meta": true, "nobr": true, "ol": true, "p": true, "pre": true,
	"ruby": true, "s": true, "small": true, "span": true, "strong": true, "strike": true,
	"sub": true, "sup": true, "table": true, "tt": true, "u": true, "ul": true, "var": true,
}

var rawTextElements = map[string]bool{
	"iframe": true, "noembed": true, "noframes": true, "noscript": true,
	"script": true, "style": true, "textarea": true, "title": true,
	"plaintext": true, "xmp": true,
}

var rawTextEndTags = map[string]string{
	"script":   "</script",
	"style":    "</style",
	"textarea": "</textarea",
	"title":    "</title",
}

func (tb *treeBuilder) mergeAttrs(n *Node, attrs []Attribute) {
	for _, a := range attrs {
		if n.GetAttribute(a.Name) == "" && !n.HasAttribute(a.Name) {
			n.Attr = append(n.Attr, a)
		}
	}
}

func (tb *treeBuilder) insertElement(tag string, attrs []Attribute, selfClose bool) *Node {
	parent := tb.appropriateInsertionParent(tb.current())
	var before *Node
	fosterInsertion := false
	if tb.fosterParent && isTableElement(parent) {
		parent, before = tb.fosterInsertionLocation()
		fosterInsertion = true
	}
	if !tb.allowNode(parent) {
		return nil
	}
	n := tb.doc.NewElement(tag)
	if parent != nil && parent.Type == NodeDocumentFragment && parent.Doc != nil && parent.Doc != tb.doc {
		n.Doc = parent.Doc
	}
	if tb.formElem != nil && len(tb.templateStack) == 0 && isFormAssociatedElement(tag) {
		n.FormOwner = tb.formElem
	}
	current := tb.adjustedCurrent()
	inSVG := tag == "svg" || (tb.svgDepth > 0 && current.Namespace == NSSVG && !isSVGHTMLIntegrationPoint(current))
	if inSVG {
		n.Namespace = NSSVG
		n.NamespaceURI = "http://www.w3.org/2000/svg"
		n.Data = tag
		n.DataAtom = Lookup(tag)
	}
	if n.Namespace == NSHTML {
		if IsValidCustomElementName(tag) {
			n.CustomElementUndefined = true
		} else {
			for _, attr := range attrs {
				if attr.Namespace == NSHTML && attr.NamespaceURI == "" && attr.Name == "is" {
					n.CustomElementUndefined = true
					break
				}
			}
		}
	}
	if tb.onParserElement != nil {
		if replacement := tb.onParserElement(n, attrs); replacement != nil {
			n = replacement
		}
	}
	for _, attr := range attrs {
		n.Attr = append(n.Attr, attr)
		if tb.onParserAttribute != nil {
			tb.onParserAttribute(n, attr)
		}
	}
	if n.Data == "input" {
		n.syncInputCheckednessFromAttribute()
	}
	insertIntoParent := n.Parent == nil && !hostIncludingInclusiveAncestor(n, parent)
	if insertIntoParent && parent != nil && parent.Type == NodeDocument {
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() {
				insertIntoParent = false
				break
			}
		}
	}
	if insertIntoParent {
		if fosterInsertion {
			insertAtFosterLocation(parent, before, n)
		} else {
			parent.AppendChild(n)
		}
		if n.Data == "input" && strings.EqualFold(n.GetAttribute("type"), "radio") {
			tb.arbitrateConnectedRadio(n)
		} else if n.Data == "form" && n.HasAttribute("id") {
			tb.rebuildConnectedRadioGroups()
		}
		if tb.onParserInserted != nil {
			tb.onParserInserted(n)
		}
	}

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

func (tb *treeBuilder) arbitrateConnectedRadio(node *Node) {
	if tb == nil || tb.doc == nil || node == nil || !node.Element() || node.Namespace != NSHTML || node.Data != "input" || !strings.EqualFold(node.GetAttribute("type"), "radio") || !node.IsConnected() || !node.InputChecked() {
		return
	}
	name := node.GetAttribute("name")
	if name == "" {
		return
	}
	if tb.parserRadioGroups == nil {
		tb.parserRadioGroups = make(map[parserRadioGroupKey]*Node)
	}
	key := parserRadioGroupKey{owner: tb.parserRadioFormOwner(node), name: name}
	if earlier := tb.parserRadioGroups[key]; earlier != nil && earlier != node && earlier.IsConnected() && earlier.InputChecked() {
		earlier.SetInputChecked(false, true)
	}
	tb.parserRadioGroups[key] = node
}

func (tb *treeBuilder) rebuildConnectedRadioGroups() {
	if tb == nil || tb.doc == nil {
		return
	}
	tb.parserRadioGroups = make(map[parserRadioGroupKey]*Node)
	tb.doc.Node.Walk(func(node *Node) bool {
		if !node.Element() || node.Namespace != NSHTML || node.Data != "input" || !strings.EqualFold(node.GetAttribute("type"), "radio") || !node.IsConnected() || !node.InputChecked() {
			return true
		}
		name := node.GetAttribute("name")
		if name == "" {
			return true
		}
		key := parserRadioGroupKey{owner: tb.parserRadioFormOwner(node), name: name}
		if earlier := tb.parserRadioGroups[key]; earlier != nil {
			earlier.SetInputChecked(false, true)
		}
		tb.parserRadioGroups[key] = node
		return true
	})
}

func (tb *treeBuilder) parserRadioFormOwner(control *Node) *Node {
	if control == nil {
		return nil
	}
	if control.HasAttribute("form") {
		id := control.GetAttribute("form")
		if id == "" {
			return nil
		}
		var owner *Node
		tb.doc.Node.Walk(func(node *Node) bool {
			if node.Element() && node.Namespace == NSHTML && node.Data == "form" && node.GetAttribute("id") == id {
				owner = node
				return false
			}
			return true
		})
		return owner
	}
	if control.FormOwner != nil && control.FormOwner.IsConnected() {
		return control.FormOwner
	}
	for parent := control.Parent; parent != nil; parent = parent.Parent {
		if parent.Element() && parent.Namespace == NSHTML && parent.Data == "form" {
			return parent
		}
	}
	return nil
}

func (tb *treeBuilder) appropriateInsertionParent(parent *Node) *Node {
	if parent.IsHTMLTemplate() {
		return parent.TemplateContent()
	}
	return parent
}

func isFormAssociatedElement(tag string) bool {
	switch tag {
	case "button", "fieldset", "input", "object", "output", "select", "textarea":
		return true
	default:
		return false
	}
}

func (tb *treeBuilder) insertMathMLElement(tag string, attrs []Attribute, selfClose bool) *Node {
	parent := tb.appropriateInsertionParent(tb.current())
	if !tb.allowNode(parent) {
		return nil
	}
	n := tb.doc.NewElement(tag)
	n.Namespace = NSMathML
	n.NamespaceURI = "http://www.w3.org/1998/Math/MathML"
	n.Attr = adjustForeignAttributes(attrs)
	parent.AppendChild(n)
	if tb.onParserInserted != nil {
		tb.onParserInserted(n)
	}
	if !selfClose {
		tb.push(n)
	}
	return n
}

func adjustForeignAttributes(attrs []Attribute) []Attribute {
	for i := range attrs {
		attr := &attrs[i]
		switch strings.ToLower(attr.Name) {
		case "definitionurl":
			attr.Name = "definitionURL"
			attr.NameAtom = LookupAttr(attr.Name)
		case "xmlns":
			attr.Namespace = NSOther
			attr.NamespaceURI = "http://www.w3.org/2000/xmlns/"
		case "xmlns:xlink":
			attr.Namespace = NSOther
			attr.NamespaceURI = "http://www.w3.org/2000/xmlns/"
		case "xml:base", "xml:lang", "xml:space":
			attr.Namespace = NSXML
			attr.NamespaceURI = "http://www.w3.org/XML/1998/namespace"
		default:
			if strings.HasPrefix(strings.ToLower(attr.Name), "xlink:") {
				attr.Namespace = NSOther
				attr.NamespaceURI = "http://www.w3.org/1999/xlink"
			}
		}
	}
	return attrs
}

func isTableElement(n *Node) bool {
	switch n.Data {
	case "table", "tbody", "thead", "tfoot", "tr":
		return true
	}
	return false
}

func (tb *treeBuilder) fosterInsertionLocation() (*Node, *Node) {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		table := tb.openStack[i]
		if table.Data != "table" {
			continue
		}
		if table.Parent != nil {
			parent := table.Parent
			if parent.IsHTMLTemplate() {
				return parent.TemplateContent(), nil
			}
			return parent, table
		}
		if i > 0 {
			parent := tb.openStack[i-1]
			if parent.IsHTMLTemplate() {
				return parent.TemplateContent(), nil
			}
			return parent, nil
		}
		return table, nil
	}
	return &tb.doc.Node, nil
}

func insertAtFosterLocation(parent, before, child *Node) bool {
	if parent == nil || child == nil {
		return false
	}
	if parent.Type == NodeDocument {
		for n := parent.FirstChild; n != nil; n = n.NextSibling {
			if n.Element() && n != child {
				return false
			}
		}
	}
	if before != nil && before.Parent == parent {
		parent.InsertBefore(child, before)
	} else {
		parent.AppendChild(child)
	}
	return true
}

func (tb *treeBuilder) insertText(data string) {
	if data == "" {
		return
	}
	parent := tb.appropriateInsertionParent(tb.current())
	var before *Node
	if tb.fosterParent && isTableElement(parent) {
		parent, before = tb.fosterInsertionLocation()
	}
	last := parent.LastChild
	if before != nil && before.Parent == parent {
		last = before.PrevSibling
	}
	if last != nil && last.Text() {
		b := tb.text[last]
		if b == nil {
			b = &strings.Builder{}
			b.WriteString(last.DataContent)
			tb.text[last] = b
		}
		b.WriteString(data)
		return
	}
	if !tb.allowNode(parent) {
		return
	}
	n := tb.doc.NewText(data)
	if before != nil && before.Parent == parent {
		parent.InsertBefore(n, before)
	} else {
		parent.AppendChild(n)
	}
	if tb.onParserInserted != nil {
		tb.onParserInserted(n)
	}
}

func (tb *treeBuilder) insertRawText(data string) {
	if data == "" {
		return
	}
	parent := tb.current()
	if last := parent.LastChild; last != nil && last.Text() {
		b := tb.text[last]
		if b == nil {
			b = &strings.Builder{}
			b.WriteString(last.DataContent)
			tb.text[last] = b
		}
		b.WriteString(data)
		return
	}
	if !tb.allowNode(parent) {
		return
	}
	n := tb.doc.NewText(data)
	parent.AppendChild(n)
	if tb.onParserInserted != nil {
		tb.onParserInserted(n)
	}
}

func (tb *treeBuilder) flushText() {
	for n, text := range tb.text {
		if n != nil && text != nil {
			n.DataContent = text.String()
		}
	}
	tb.text = make(map[*Node]*strings.Builder)
}

func hasNonSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			return true
		}
	}
	return false
}

func (tb *treeBuilder) insertComment(data string) {
	parent := tb.appropriateInsertionParent(tb.current())
	if !tb.allowNode(parent) {
		return
	}
	n := tb.doc.NewComment(data)
	parent.AppendChild(n)
	if tb.onParserInserted != nil {
		tb.onParserInserted(n)
	}
}

func (tb *treeBuilder) parse(html string) {
	p := &tokenizer{input: preprocessHTMLInput(html), limits: tb.limits}
	if tb.fragmentContext != nil {
		tb.consumeInitialFragmentText(p)
	}

	for p.pos < len(p.input) && tb.err == nil {
		if p.pos < len(p.input) && p.input[p.pos] == '<' {
			if p.pos+1 < len(p.input) {
				switch c := p.input[p.pos+1]; {
				case c == '/':
					tb.handleEndTag(p)
				case c == '!':
					if tb.cdataAllowed() && strings.HasPrefix(p.input[p.pos:], "<![CDATA[") {
						tb.consumeCDATA(p)
					} else {
						tb.handleCommentOrDoctype(p)
					}
				case c == '?':
					tb.consumeBogusComment(p, 1)
				case isASCIILetter(c):
					tb.handleStartTag(p)
				default:
					tb.processToken(tokenChars, "", nil, false, "<")
					p.pos++
				}
			} else {
				tb.processToken(tokenChars, "", nil, false, "<")
				p.pos++
			}
		} else {
			tb.handleText(p)
		}
	}
}

func (tb *treeBuilder) consumeInitialFragmentText(p *tokenizer) {
	context := tb.fragmentContext
	if context == nil || context.Namespace != NSHTML {
		return
	}
	tag := context.Data
	if !rawTextElements[tag] {
		return
	}
	text := p.input
	if tag == "title" || tag == "textarea" {
		text = unescapeHTMLReferences(strings.ReplaceAll(text, "\x00", "\uFFFD"))
	} else {
		text = strings.ReplaceAll(text, "\x00", "\uFFFD")
	}
	tb.insertText(text)
	p.pos = len(p.input)
}

func (tb *treeBuilder) cdataAllowed() bool {
	current := tb.adjustedCurrent()
	return current != nil && current.Namespace != NSHTML
}

func (tb *treeBuilder) consumeCDATA(p *tokenizer) {
	p.pos += len("<![CDATA[")
	start := p.pos
	end := strings.Index(p.input[p.pos:], "]]>")
	if end < 0 {
		p.pos = len(p.input)
		tb.processToken(tokenChars, "", nil, false, strings.ReplaceAll(p.input[start:], "\x00", "\uFFFD"))
		return
	}
	tb.processToken(tokenChars, "", nil, false, strings.ReplaceAll(p.input[start:p.pos+end], "\x00", "\uFFFD"))
	p.pos += end + len("]]>")
}

func (tb *treeBuilder) consumeBogusComment(p *tokenizer, prefix int) {
	p.pos += prefix
	start := p.pos
	p.skipTo('>')
	data := strings.ReplaceAll(p.input[start:p.pos], "\x00", "\uFFFD")
	if p.pos < len(p.input) {
		p.pos++
	}
	tb.processToken(tokenComment, "", nil, false, data)
}

func preprocessHTMLInput(input string) string {
	if strings.HasPrefix(input, "\uFEFF") {
		input = input[len("\uFEFF"):]
	}
	if !strings.Contains(input, "\r") {
		return input
	}
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return input
}

func unescapeHTMLReferences(input string) string {
	if !strings.Contains(input, "&") {
		return input
	}
	return xhtml.UnescapeString(input)
}

func decodeHTMLDataText(input string) string {
	if !strings.ContainsAny(input, "&\x00") {
		return input
	}
	parts := strings.Split(input, "\x00")
	for i := range parts {
		parts[i] = unescapeHTMLReferences(parts[i])
	}
	return strings.Join(parts, "")
}

func unescapeHTMLAttributeReferences(input string) string {
	if !strings.Contains(input, "&") {
		return input
	}
	input = strings.ReplaceAll(input, `"`, "&#34;")
	t := xhtml.NewTokenizer(strings.NewReader(`<x a="` + input + `">`))
	if t.Next() != xhtml.StartTagToken {
		return input
	}
	for _, attr := range t.Token().Attr {
		if attr.Key == "a" {
			return attr.Val
		}
	}
	return input
}

func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func (tb *treeBuilder) handleText(p *tokenizer) {
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '<' {
		p.pos++
	}
	text := p.input[start:p.pos]
	if strings.IndexByte(text, '&') >= 0 || strings.IndexByte(text, '\x00') >= 0 {
		text = decodeHTMLDataText(text)
	}
	if text != "" {
		tb.processToken(tokenChars, "", nil, false, text)
	}
}

func (tb *treeBuilder) handleStartTag(p *tokenizer) {
	p.pos++
	if p.pos >= len(p.input) {
		tb.processToken(tokenChars, "", nil, false, "<")
		return
	}

	tag, attrs, selfClose := p.parseTag()
	if p.err != nil {
		tb.err = p.err
		return
	}
	if tag == "" {
		tb.processToken(tokenChars, "", nil, false, "<")
		return
	}

	tag = strings.ToLower(tag)
	current := tb.adjustedCurrent()
	htmlContext := current.Namespace == NSHTML || isSVGHTMLIntegrationPoint(current) ||
		isMathMLTextIntegrationPoint(current, tag) || isMathMLHTMLIntegrationPoint(current)
	if htmlContext {
		tb.processToken(tokenStartTag, tag, attrs, selfClose, "")
	} else if current.Namespace != NSHTML {
		if tag == "svg" {
			tb.processStartTag(tag, attrs, selfClose)
			return
		}
		if shouldBreakOutOfForeignContent(tag, attrs) {
			tb.popForeignContent()
			tb.processToken(tokenStartTag, tag, attrs, selfClose, "")
		} else {
			if current.Namespace == NSMathML {
				tb.insertMathMLElement(tag, attrs, selfClose)
			} else {
				if adjusted := svgTagAdjustments[tag]; adjusted != "" {
					tag = adjusted
				}
				tb.processStartTag(tag, attrs, selfClose)
			}
		}
	} else {
		tb.processToken(tokenStartTag, tag, attrs, selfClose, "")
	}

	if htmlContext && rawTextElements[tag] && (tag != "noscript" || tb.scriptingEnabled) {
		tb.consumeRawText(p, tag)
	}
}

func shouldBreakOutOfForeignContent(tag string, attrs []Attribute) bool {
	if foreignBreakoutElements[tag] {
		return true
	}
	if tag != "font" {
		return false
	}
	for _, attr := range attrs {
		switch strings.ToLower(attr.Name) {
		case "color", "face", "size":
			return true
		}
	}
	return false
}

func isSVGHTMLIntegrationPoint(node *Node) bool {
	if node == nil || node.Namespace != NSSVG {
		return false
	}
	switch node.Data {
	case "foreignObject", "desc", "title":
		return true
	default:
		return false
	}
}

func isMathMLTextIntegrationPoint(node *Node, tag string) bool {
	if node == nil || node.Namespace != NSMathML {
		return false
	}
	switch node.Data {
	case "mi", "mo", "mn", "ms", "mtext":
		return tag != "mglyph" && tag != "malignmark"
	default:
		return false
	}
}

func isMathMLHTMLIntegrationPoint(node *Node) bool {
	if node == nil || node.Namespace != NSMathML || node.Data != "annotation-xml" {
		return false
	}
	encoding := strings.TrimSpace(node.GetAttribute("encoding"))
	return strings.EqualFold(encoding, "text/html") || strings.EqualFold(encoding, "application/xhtml+xml")
}

func (tb *treeBuilder) popForeignContent() {
	for len(tb.openStack) > 0 {
		current := tb.current()
		if current.Namespace == NSHTML || isSVGHTMLIntegrationPoint(current) ||
			isMathMLHTMLIntegrationPoint(current) || isMathMLTextIntegrationPoint(current, "") {
			break
		}
		if tb.pop().Data == "svg" && tb.svgDepth > 0 {
			tb.svgDepth--
		}
	}
}

func (tb *treeBuilder) consumeRawText(p *tokenizer, tag string) {
	var script *Node
	if tag == "script" && tb.current() != nil && tb.current().Namespace == NSHTML {
		script = tb.current()
	}
	start := p.pos
	if tag == "plaintext" {
		p.pos = len(p.input)
	} else if tag == "script" {
		p.pos = scanScriptDataEnd(p.input, p.pos)
	} else {
		for p.pos < len(p.input) {
			idx := strings.Index(p.input[p.pos:], "</")
			if idx < 0 {
				p.pos = len(p.input)
				break
			}
			p.pos += idx
			tagStart := p.pos + 2
			tagEnd := tagStart + len(tag)
			if tagEnd <= len(p.input) && strings.EqualFold(p.input[tagStart:tagEnd], tag) {
				if tagEnd >= len(p.input) || isRawTextEndDelimiter(p.input[tagEnd]) {
					break
				}
			}
			p.pos += 2
		}
	}
	if p.pos > start {
		text := strings.ReplaceAll(p.input[start:p.pos], "\x00", "\uFFFD")
		if tag == "title" || tag == "textarea" {
			text = unescapeHTMLReferences(text)
		}
		tb.insertRawText(text)
	}
	if tag != "plaintext" && p.pos < len(p.input) {
		idx := strings.IndexByte(p.input[p.pos:], '>')
		if idx >= 0 {
			p.pos += idx + 1
		} else {
			p.pos = len(p.input)
		}
	}
	tb.popUntil(tag)
	tb.mode = tb.prevMode
	if script != nil && tb.onParserScript != nil && tb.parserScriptMayExecute() {
		tb.flushText()
		tb.onParserScript(tb.doc, script)
	}
}

func scanScriptDataEnd(input string, start int) int {
	const (
		scriptData = iota
		scriptDataEscaped
		scriptDataDoubleEscaped
	)
	state := scriptData
	for i := start; i < len(input); {
		remaining := input[i:]
		switch {
		case state == scriptData && strings.HasPrefix(remaining, "<!--"):
			state = scriptDataEscaped
			i += len("<!--")
		case state == scriptDataEscaped && strings.HasPrefix(remaining, "-->"):
			state = scriptData
			i += len("-->")
		case state == scriptDataEscaped && scriptTagSequenceAt(input, i, "<script"):
			state = scriptDataDoubleEscaped
			i += len("<script")
		case state == scriptDataDoubleEscaped && scriptTagSequenceAt(input, i, "</script"):
			state = scriptDataEscaped
			i += len("</script")
		case state != scriptDataDoubleEscaped && scriptTagSequenceAt(input, i, "</script"):
			return i
		default:
			i++
		}
	}
	return len(input)
}

func scriptTagSequenceAt(input string, at int, prefix string) bool {
	if at+len(prefix) > len(input) || !strings.EqualFold(input[at:at+len(prefix)], prefix) {
		return false
	}
	end := at + len(prefix)
	return end == len(input) || isRawTextEndDelimiter(input[end])
}

func (tb *treeBuilder) parserScriptMayExecute() bool {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		node := tb.openStack[i]
		if node != nil && node.IsHTMLTemplate() {
			return node.HasAttribute("shadowrootmode") || node.HasAttribute("shadowroot")
		}
	}
	return len(tb.templateStack) == 0
}

func isRawTextEndDelimiter(c byte) bool {
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func (tb *treeBuilder) processStartTag(tag string, attrs []Attribute, selfClose bool) {
	current := tb.adjustedCurrent()
	if tag == "math" && (current == nil || current.Namespace == NSHTML || isSVGHTMLIntegrationPoint(current) || isMathMLHTMLIntegrationPoint(current) || isMathMLTextIntegrationPoint(current, tag)) {
		tb.insertMathMLElement(tag, attrs, selfClose)
		return
	}
	if tag == "svg" || tb.svgDepth > 0 {
		attrs = adjustForeignAttributes(attrs)
		if tag == "svg" {
			tb.svgDepth++
		}
		sc := selfClose
		n := tb.insertElement(tag, attrs, sc)
		if n != nil && n.Namespace == NSSVG && n.Data == "script" && selfClose {
			n.SVGScriptReady = true
			if tb.onParserScript != nil && tb.parserScriptMayExecute() {
				tb.flushText()
				tb.onParserScript(tb.doc, n)
			}
		}
		if tag == "svg" && sc {
			tb.svgDepth--
		}
		return
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
	current := tb.adjustedCurrent()
	if current != nil && current.Namespace != NSHTML && (tag == "br" || tag == "p") {
		tb.popForeignContent()
		tb.processToken(tokenEndTag, tag, nil, false, "")
		return
	}
	if current != nil && current.Namespace != NSHTML {
		adjusted := tag
		if name := svgTagAdjustments[tag]; name != "" && current.Namespace == NSSVG {
			adjusted = name
		}
		for i := len(tb.openStack) - 1; i >= 0; i-- {
			node := tb.openStack[i]
			if node.Namespace == NSHTML {
				break
			}
			if strings.EqualFold(node.Data, adjusted) {
				closedSVGScript := node.Namespace == NSSVG && node.Data == "script" && tag == "script"
				if closedSVGScript {
					node.SVGScriptReady = true
				}
				for len(tb.openStack) > i {
					if tb.pop().Data == "svg" && tb.svgDepth > 0 {
						tb.svgDepth--
					}
				}
				if closedSVGScript && tb.onParserScript != nil && tb.parserScriptMayExecute() {
					tb.flushText()
					tb.onParserScript(tb.doc, node)
				}
				return
			}
		}
		return
	}
	if tag == "br" {
		tb.insertElement("br", nil, false)
		return
	}
	if voidElements[tag] {
		return
	}
	tb.processToken(tokenEndTag, tag, nil, false, "")
}

func (tb *treeBuilder) handleCommentOrDoctype(p *tokenizer) {
	p.pos += 2
	if p.pos+1 < len(p.input) && p.input[p.pos] == '-' && p.input[p.pos+1] == '-' {
		p.pos += 2
		end := strings.Index(p.input[p.pos:], "-->")
		if end >= 0 {
			data := strings.ReplaceAll(p.input[p.pos:p.pos+end], "\x00", "\uFFFD")
			tb.processToken(tokenComment, "", nil, false, data)
			p.pos += end + 3
		} else {
			p.pos = len(p.input)
		}
		return
	}
	if len(p.input)-p.pos >= 7 && strings.EqualFold(p.input[p.pos:p.pos+7], "DOCTYPE") {
		p.pos += 7
		end := strings.Index(p.input[p.pos:], ">")
		if end < 0 {
			end = len(p.input) - p.pos
		}
		data := strings.ReplaceAll(strings.TrimSpace(p.input[p.pos:p.pos+end]), "\x00", "\uFFFD")
		tb.processToken(tokenDoctype, "", nil, false, data)
		p.pos += end
		if p.pos < len(p.input) {
			p.pos++
		}
		return
	}
	tb.consumeBogusComment(p, 0)
}

func (tb *treeBuilder) processToken(tokenType int, tag string, attrs []Attribute, selfClose bool, data string) {
	for {
		var reprocess bool
		switch tb.mode {
		case modeInitial:
			reprocess = tb.handleInitial(tokenType, tag, attrs, selfClose, data)
		case modeBeforeHTML:
			reprocess = tb.handleBeforeHTML(tokenType, tag, attrs, selfClose, data)
		case modeBeforeHead:
			reprocess = tb.handleBeforeHead(tokenType, tag, attrs, selfClose, data)
		case modeInHead:
			reprocess = tb.handleInHead(tokenType, tag, attrs, selfClose, data)
		case modeInHeadNoscript:
			reprocess = tb.handleInHeadNoscript(tokenType, tag, attrs, selfClose, data)
		case modeAfterHead:
			reprocess = tb.handleAfterHead(tokenType, tag, attrs, selfClose, data)
		case modeInBody:
			reprocess = tb.handleInBody(tokenType, tag, attrs, selfClose, data)
		case modeText:
			reprocess = tb.handleTextMode(tokenType, tag, attrs, selfClose, data)
		case modeInTable:
			reprocess = tb.handleInTable(tokenType, tag, attrs, selfClose, data)
		case modeInTableText:
			reprocess = tb.handleInTableText(tokenType, tag, attrs, selfClose, data)
		case modeInCaption:
			reprocess = tb.handleInCaption(tokenType, tag, attrs, selfClose, data)
		case modeInColumnGroup:
			reprocess = tb.handleInColumnGroup(tokenType, tag, attrs, selfClose, data)
		case modeInTableBody:
			reprocess = tb.handleInTableBody(tokenType, tag, attrs, selfClose, data)
		case modeInRow:
			reprocess = tb.handleInRow(tokenType, tag, attrs, selfClose, data)
		case modeInCell:
			reprocess = tb.handleInCell(tokenType, tag, attrs, selfClose, data)
		case modeInSelect:
			reprocess = tb.handleInSelect(tokenType, tag, attrs, selfClose, data)
		case modeInSelectInTable:
			reprocess = tb.handleInSelectInTable(tokenType, tag, attrs, selfClose, data)
		case modeInTemplate:
			reprocess = tb.handleInTemplate(tokenType, tag, attrs, selfClose, data)
		case modeAfterBody:
			reprocess = tb.handleAfterBody(tokenType, tag, attrs, selfClose, data)
		case modeInFrameset:
			reprocess = tb.handleInFrameset(tokenType, tag, attrs, selfClose, data)
		case modeAfterFrameset:
			reprocess = tb.handleAfterFrameset(tokenType, tag, attrs, selfClose, data)
		case modeAfterAfterBody:
			reprocess = tb.handleAfterAfterBody(tokenType, tag, attrs, selfClose, data)
		case modeAfterAfterFrameset:
			reprocess = tb.handleAfterAfterFrameset(tokenType, tag, attrs, selfClose, data)
		default:
			return
		}
		if !reprocess {
			return
		}
	}
}

func (tb *treeBuilder) ensureHead() {
	if tb.headElem == nil && tb.htmlElem != nil {
		tb.insertElement("head", nil, false)
	}
}

func (tb *treeBuilder) headPhase() bool {
	if tb.bodyElem != nil {
		return false
	}
	if tb.headElem == nil {
		return true
	}
	return tb.onStack(tb.headElem)
}

func (tb *treeBuilder) hasOpenTag(tag string) bool {
	for _, o := range tb.openStack {
		if o.Data == tag {
			return true
		}
	}
	return false
}

func (tb *treeBuilder) onStack(n *Node) bool {
	for _, o := range tb.openStack {
		if o == n {
			return true
		}
	}
	return false
}

func (tb *treeBuilder) ensureBody() {
	if tb.bodyElem == nil {
		tb.ensureHead()
		if tb.headElem != nil && tb.onStack(tb.headElem) {
			tb.popUntil("head")
		}
		tb.insertElement("body", nil, false)
	}
}

func (tb *treeBuilder) closeP() {
	tb.popUntil("p")
}

func (tb *treeBuilder) closePifOpen() {
	if tb.hasInScope("p") {
		tb.closeP()
	}
}

func (tb *treeBuilder) closeListItem() {
	for i := len(tb.openStack) - 1; i >= 0; i-- {
		if tb.openStack[i].Data == "li" {
			tb.popUntil("li")
			return
		}
		if tb.openStack[i].Data == "ul" || tb.openStack[i].Data == "ol" {
			return
		}
		if isScopeElement(tb.openStack[i]) {
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
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		if tagSet[top.Data] {
			tb.pop()
			return
		}
		tb.pop()
	}
}

func (tb *treeBuilder) closeTableRows(groups ...string) {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		switch top.Data {
		case "td", "th", "tr":
			tb.pop()
			continue
		case "table":
			return
		}
		for _, g := range groups {
			if top.Data == g {
				tb.pop()
			}
		}
		return
	}
}

func (tb *treeBuilder) closeTableCell() {
	for len(tb.openStack) > 0 {
		top := tb.openStack[len(tb.openStack)-1]
		if top.Data == "td" || top.Data == "th" {
			tb.pop()
			continue
		}
		return
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
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' {
				break
			}
			t.pos++
		}
		value = t.input[start:t.pos]
	}
	value = strings.ReplaceAll(value, "\x00", "\uFFFD")
	if strings.IndexByte(value, '&') >= 0 {
		value = unescapeHTMLAttributeReferences(value)
	}
	return name, value
}

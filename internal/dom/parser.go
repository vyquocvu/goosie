package dom

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Parser handles HTML parsing
type Parser struct{}

// NewParser creates a new Parser instance
func NewParser() *Parser {
	return &Parser{}
}

// ParseDocument parses HTML from an io.Reader and returns the root *html.Node.
// This enables streaming: the caller can pass an HTTP response body directly,
// avoiding an intermediate string copy. The reader is consumed fully before
// return; callers should close it afterwards.
func (p *Parser) ParseDocument(r io.Reader) (*html.Node, error) {
	return html.Parse(r)
}

// ParseBodyText extracts text content from the body element
func (p *Parser) ParseBodyText(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var bodyText strings.Builder
	var extractText func(*html.Node)
	extractText = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			p.getTextFromNode(n, &bodyText)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c)
		}
	}

	extractText(doc)
	return strings.TrimSpace(bodyText.String()), nil
}

// getTextFromNode extracts text from a node and its children
func (p *Parser) getTextFromNode(n *html.Node, builder *strings.Builder) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			if builder.Len() > 0 {
				builder.WriteString(" ")
			}
			builder.WriteString(text)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.getTextFromNode(c, builder)
	}
}

// Element represents a DOM element with its properties
type Element struct {
	TagName     string
	ID          string
	Classes     []string
	Attributes  map[string]string
	TextContent string
	Node        *html.Node
}

// GetElementByID searches for an element by ID (basic implementation)
func (p *Parser) GetElementByID(htmlContent, id string) (string, error) {
	elem, err := p.GetElementByIDFull(htmlContent, id)
	if err != nil || elem == nil {
		return "", err
	}
	return elem.TextContent, nil
}

// GetElementByIDFull returns the full Element object by ID
func (p *Parser) GetElementByIDFull(htmlContent, id string) (*Element, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	var result *Element
	var findElement func(*html.Node)
	findElement = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == id {
					result = p.nodeToElement(n)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if result == nil {
				findElement(c)
			}
		}
	}

	findElement(doc)
	return result, nil
}

// GetElementsByClassName returns all elements with the specified class name
func (p *Parser) GetElementsByClassName(htmlContent, className string) ([]*Element, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	var elements []*Element
	var findElements func(*html.Node)
	findElements = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "class" && hasMatchingClass(attr.Val, className) {
					elements = append(elements, p.nodeToElement(n))
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findElements(c)
		}
	}

	findElements(doc)
	return elements, nil
}

// GetElementsByTagName returns all elements with the specified tag name
func (p *Parser) GetElementsByTagName(htmlContent, tagName string) ([]*Element, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	tagName = strings.ToLower(tagName)
	var elements []*Element
	var findElements func(*html.Node)
	findElements = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == tagName {
			elements = append(elements, p.nodeToElement(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findElements(c)
		}
	}

	findElements(doc)
	return elements, nil
}

// QuerySelector returns the first element matching the CSS selector
func (p *Parser) QuerySelector(htmlContent, selector string) (*Element, error) {
	elements, err := p.QuerySelectorAll(htmlContent, selector)
	if err != nil {
		return nil, err
	}
	if len(elements) == 0 {
		return nil, nil
	}
	return elements[0], nil
}

// QuerySelectorAll returns all elements matching the CSS selector
func (p *Parser) QuerySelectorAll(htmlContent, selector string) ([]*Element, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	var elements []*Element
	var findElements func(*html.Node)
	findElements = func(n *html.Node) {
		if n.Type == html.ElementNode && p.matchesSelector(n, selector) {
			elements = append(elements, p.nodeToElement(n))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findElements(c)
		}
	}

	findElements(doc)
	return elements, nil
}

// hasMatchingClass checks if classAttr contains className without allocating slices.
func hasMatchingClass(classAttr, className string) bool {
	for classAttr != "" {
		i := 0
		for i < len(classAttr) && (classAttr[i] == ' ' || classAttr[i] == '\t' || classAttr[i] == '\n' || classAttr[i] == '\r' || classAttr[i] == '\f') {
			i++
		}
		classAttr = classAttr[i:]
		if classAttr == "" {
			break
		}
		j := 0
		for j < len(classAttr) && !(classAttr[j] == ' ' || classAttr[j] == '\t' || classAttr[j] == '\n' || classAttr[j] == '\r' || classAttr[j] == '\f') {
			j++
		}
		if classAttr[:j] == className {
			return true
		}
		classAttr = classAttr[j:]
	}
	return false
}

type domAttrMatcher struct {
	name     string
	operator string // "", "=", "^=", "$=", "*=", "~=", "|="
	value    string
}

type domCompoundStep struct {
	combinator string // "", " ", ">", "+", "~"
	tag        string
	ids        []string
	classes    []string
	attrs      []domAttrMatcher
	pseudos    []string
}

func getParentElem(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			return p
		}
	}
	return nil
}

func getPrevElemSibling(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	for s := n.PrevSibling; s != nil; s = s.PrevSibling {
		if s.Type == html.ElementNode {
			return s
		}
	}
	return nil
}

func getNextElemSibling(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode {
			return s
		}
	}
	return nil
}

func splitSelectorList(sel string) []string {
	var results []string
	var current strings.Builder
	inQuotes := false
	var quoteChar rune
	bracketDepth := 0

	for _, r := range sel {
		if inQuotes {
			current.WriteRune(r)
			if r == quoteChar {
				inQuotes = false
			}
			continue
		}
		if r == '"' || r == '\'' {
			inQuotes = true
			quoteChar = r
			current.WriteRune(r)
			continue
		}
		if r == '[' {
			bracketDepth++
			current.WriteRune(r)
			continue
		}
		if r == ']' {
			if bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
			continue
		}
		if r == ',' && bracketDepth == 0 {
			part := strings.TrimSpace(current.String())
			if part != "" {
				results = append(results, part)
			}
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if part := strings.TrimSpace(current.String()); part != "" {
		results = append(results, part)
	}
	return results
}

func parseAttrSelector(raw string) domAttrMatcher {
	raw = strings.TrimSpace(raw)
	for _, op := range []string{"^=", "$=", "*=", "~=", "|=", "="} {
		if idx := strings.Index(raw, op); idx != -1 {
			name := strings.TrimSpace(raw[:idx])
			val := strings.TrimSpace(raw[idx+len(op):])
			val = strings.Trim(val, "\"'")
			return domAttrMatcher{
				name:     name,
				operator: op,
				value:    val,
			}
		}
	}
	return domAttrMatcher{
		name:     strings.TrimSpace(raw),
		operator: "",
		value:    "",
	}
}

func parseCompoundStep(s string, comb string) domCompoundStep {
	step := domCompoundStep{combinator: comb}
	runes := []rune(s)
	i := 0

	var tagBuilder strings.Builder
	for i < len(runes) && runes[i] != '#' && runes[i] != '.' && runes[i] != '[' && runes[i] != ':' {
		tagBuilder.WriteRune(runes[i])
		i++
	}
	step.tag = strings.TrimSpace(tagBuilder.String())

	for i < len(runes) {
		r := runes[i]
		if r == '#' {
			i++
			var idBuilder strings.Builder
			for i < len(runes) && runes[i] != '#' && runes[i] != '.' && runes[i] != '[' && runes[i] != ':' {
				idBuilder.WriteRune(runes[i])
				i++
			}
			if id := idBuilder.String(); id != "" {
				step.ids = append(step.ids, id)
			}
		} else if r == '.' {
			i++
			var classBuilder strings.Builder
			for i < len(runes) && runes[i] != '#' && runes[i] != '.' && runes[i] != '[' && runes[i] != ':' {
				classBuilder.WriteRune(runes[i])
				i++
			}
			if cls := classBuilder.String(); cls != "" {
				step.classes = append(step.classes, cls)
			}
		} else if r == '[' {
			i++
			var attrBuilder strings.Builder
			inQuotes := false
			var quoteChar rune
			for i < len(runes) {
				ar := runes[i]
				if inQuotes {
					attrBuilder.WriteRune(ar)
					if ar == quoteChar {
						inQuotes = false
					}
					i++
					continue
				}
				if ar == '"' || ar == '\'' {
					inQuotes = true
					quoteChar = ar
					attrBuilder.WriteRune(ar)
					i++
					continue
				}
				if ar == ']' {
					i++
					break
				}
				attrBuilder.WriteRune(ar)
				i++
			}
			raw := attrBuilder.String()
			am := parseAttrSelector(raw)
			step.attrs = append(step.attrs, am)
		} else if r == ':' {
			i++
			var pseudoBuilder strings.Builder
			for i < len(runes) && runes[i] != '#' && runes[i] != '.' && runes[i] != '[' && runes[i] != ':' {
				pseudoBuilder.WriteRune(runes[i])
				i++
			}
			if p := strings.ToLower(strings.TrimSpace(pseudoBuilder.String())); p != "" {
				step.pseudos = append(step.pseudos, p)
			}
		} else {
			i++
		}
	}

	return step
}

func parseSelectorSequence(seq string) []domCompoundStep {
	var steps []domCompoundStep
	currentComb := ""
	var currentToken strings.Builder

	inQuotes := false
	var quoteChar rune
	bracketDepth := 0

	flush := func() {
		tok := strings.TrimSpace(currentToken.String())
		currentToken.Reset()
		if tok == "" {
			return
		}
		step := parseCompoundStep(tok, currentComb)
		steps = append(steps, step)
		currentComb = " "
	}

	runes := []rune(seq)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if inQuotes {
			currentToken.WriteRune(r)
			if r == quoteChar {
				inQuotes = false
			}
			continue
		}
		if r == '"' || r == '\'' {
			inQuotes = true
			quoteChar = r
			currentToken.WriteRune(r)
			continue
		}
		if r == '[' {
			bracketDepth++
			currentToken.WriteRune(r)
			continue
		}
		if r == ']' {
			if bracketDepth > 0 {
				bracketDepth--
			}
			currentToken.WriteRune(r)
			continue
		}

		if bracketDepth == 0 {
			if r == '>' || r == '+' || r == '~' {
				flush()
				currentComb = string(r)
				continue
			}
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				j := i + 1
				for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r') {
					j++
				}
				if j < len(runes) && (runes[j] == '>' || runes[j] == '+' || runes[j] == '~') {
					flush()
					currentComb = string(runes[j])
					i = j
					continue
				}
				if currentToken.Len() > 0 {
					flush()
					currentComb = " "
				}
				continue
			}
		}

		currentToken.WriteRune(r)
	}

	flush()
	return steps
}

func matchCompound(n *html.Node, step *domCompoundStep) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	if step.tag != "" && step.tag != "*" {
		if !strings.EqualFold(n.Data, step.tag) {
			return false
		}
	}
	for _, id := range step.ids {
		found := false
		for _, a := range n.Attr {
			if a.Key == "id" && a.Val == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, cls := range step.classes {
		found := false
		for _, a := range n.Attr {
			if a.Key == "class" && hasMatchingClass(a.Val, cls) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, am := range step.attrs {
		found := false
		var val string
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, am.name) {
				found = true
				val = a.Val
				break
			}
		}
		if !found {
			return false
		}
		switch am.operator {
		case "":
			// Existence
		case "=":
			if val != am.value {
				return false
			}
		case "^=":
			if am.value == "" || !strings.HasPrefix(val, am.value) {
				return false
			}
		case "$=":
			if am.value == "" || !strings.HasSuffix(val, am.value) {
				return false
			}
		case "*=":
			if am.value == "" || !strings.Contains(val, am.value) {
				return false
			}
		case "~=":
			if !hasMatchingClass(val, am.value) {
				return false
			}
		case "|=":
			if val != am.value && !strings.HasPrefix(val, am.value+"-") {
				return false
			}
		}
	}
	for _, pseudo := range step.pseudos {
		switch pseudo {
		case "first-child":
			if getPrevElemSibling(n) != nil {
				return false
			}
		case "last-child":
			if getNextElemSibling(n) != nil {
				return false
			}
		case "only-child":
			if getPrevElemSibling(n) != nil || getNextElemSibling(n) != nil {
				return false
			}
		case "scope":
			// matches
		}
	}
	return true
}

func matchSelectorSequence(n *html.Node, steps []domCompoundStep, stepIdx int) bool {
	if stepIdx < 0 {
		return true
	}
	if !matchCompound(n, &steps[stepIdx]) {
		return false
	}
	if stepIdx == 0 {
		return true
	}
	comb := steps[stepIdx].combinator
	switch comb {
	case ">":
		parent := getParentElem(n)
		if parent == nil {
			return false
		}
		return matchSelectorSequence(parent, steps, stepIdx-1)
	case " ":
		for curr := getParentElem(n); curr != nil; curr = getParentElem(curr) {
			if matchSelectorSequence(curr, steps, stepIdx-1) {
				return true
			}
		}
		return false
	case "+":
		sib := getPrevElemSibling(n)
		if sib == nil {
			return false
		}
		return matchSelectorSequence(sib, steps, stepIdx-1)
	case "~":
		for sib := getPrevElemSibling(n); sib != nil; sib = getPrevElemSibling(sib) {
			if matchSelectorSequence(sib, steps, stepIdx-1) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// matchesSelector checks if a node matches a CSS selector
func (p *Parser) matchesSelector(n *html.Node, selector string) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	selectorList := splitSelectorList(selector)
	for _, sel := range selectorList {
		steps := parseSelectorSequence(sel)
		if len(steps) == 0 {
			continue
		}
		if matchSelectorSequence(n, steps, len(steps)-1) {
			return true
		}
	}
	return false
}

// nodeToElement converts an html.Node to an Element
func (p *Parser) nodeToElement(n *html.Node) *Element {
	elem := &Element{
		TagName:    strings.ToLower(n.Data),
		Attributes: make(map[string]string),
		Node:       n,
	}

	for _, attr := range n.Attr {
		elem.Attributes[attr.Key] = attr.Val
		if attr.Key == "id" {
			elem.ID = attr.Val
		} else if attr.Key == "class" {
			elem.Classes = strings.Fields(attr.Val)
		}
	}

	var textBuilder strings.Builder
	p.getTextFromNode(n, &textBuilder)
	elem.TextContent = textBuilder.String()

	return elem
}

// ParseBodyHTML extracts HTML content from the body element and converts to markdown-like format
func (p *Parser) ParseBodyHTML(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var markdown strings.Builder
	var extractHTML func(*html.Node)
	extractHTML = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			p.convertToMarkdown(n, &markdown)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractHTML(c)
		}
	}

	extractHTML(doc)
	return strings.TrimSpace(markdown.String()), nil
}

// convertToMarkdown converts HTML nodes to markdown-like format
func (p *Parser) convertToMarkdown(n *html.Node, builder *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		text := strings.TrimSpace(n.Data)
		if text != "" {
			builder.WriteString(text)
		}
	case html.ElementNode:
		switch n.Data {
		case "h1":
			builder.WriteString("\n# ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("\n\n")
		case "h2":
			builder.WriteString("\n## ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("\n\n")
		case "h3":
			builder.WriteString("\n### ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("\n\n")
		case "p":
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("\n\n")
		case "a":
			href := ""
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
					break
				}
			}
			builder.WriteString("[")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("]")
			if href != "" {
				builder.WriteString("(")
				builder.WriteString(href)
				builder.WriteString(")")
			}
		case "strong", "b":
			builder.WriteString("**")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("**")
		case "em", "i":
			builder.WriteString("*")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("*")
		case "br":
			builder.WriteString("\n")
		case "div":
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
			builder.WriteString("\n")
		default:
			// For other elements, just process children
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.convertToMarkdown(c, builder)
			}
		}
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.convertToMarkdown(c, builder)
		}
	}
}

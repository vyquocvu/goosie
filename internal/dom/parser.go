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
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var result string
	var findElement func(*html.Node)
	findElement = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == id {
					var textBuilder strings.Builder
					p.getTextFromNode(n, &textBuilder)
					result = textBuilder.String()
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if result == "" {
				findElement(c)
			}
		}
	}

	findElement(doc)
	return result, nil
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
				if attr.Key == "class" {
					classes := strings.Fields(attr.Val)
					for _, class := range classes {
						if class == className {
							elements = append(elements, p.nodeToElement(n))
							break
						}
					}
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

// matchesSelector checks if a node matches a CSS selector (basic implementation)
func (p *Parser) matchesSelector(n *html.Node, selector string) bool {
	selector = strings.TrimSpace(selector)

	// Handle ID selector (#id)
	if strings.HasPrefix(selector, "#") {
		id := selector[1:]
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == id {
				return true
			}
		}
		return false
	}

	// Handle class selector (.class)
	if strings.HasPrefix(selector, ".") {
		className := selector[1:]
		for _, attr := range n.Attr {
			if attr.Key == "class" {
				classes := strings.Fields(attr.Val)
				for _, class := range classes {
					if class == className {
						return true
					}
				}
			}
		}
		return false
	}

	// Handle attribute selector ([attr=value])
	if strings.HasPrefix(selector, "[") && strings.HasSuffix(selector, "]") {
		attrSelector := selector[1 : len(selector)-1]
		parts := strings.Split(attrSelector, "=")
		if len(parts) == 2 {
			attrName := strings.TrimSpace(parts[0])
			attrValue := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			for _, attr := range n.Attr {
				if attr.Key == attrName && attr.Val == attrValue {
					return true
				}
			}
		}
		return false
	}

	// Handle tag selector
	return strings.ToLower(n.Data) == strings.ToLower(selector)
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

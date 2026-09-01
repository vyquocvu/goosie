package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/css"
	"golang.org/x/net/html"
)

// findBodyNode finds the body element in an HTML document.
// Copied from internal/renderer (unexported) for use in external tests.
func findBodyNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}

	if node.Type == html.ElementNode && node.Data == "body" {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findBodyNode(child); found != nil {
			return found
		}
	}

	return nil
}

// extractAndParseCSS finds all <style> tags, extracts their content, and parses it.
// Copied from internal/renderer (unexported) for use in external tests.
func extractAndParseCSS(node *html.Node) *css.StyleSheet {
	var cssContent string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "style" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					cssContent += c.Data
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(node)

	if cssContent == "" {
		return &css.StyleSheet{}
	}

	parser := css.NewParser(cssContent)
	stylesheet, err := parser.Parse()
	if err != nil {
		return &css.StyleSheet{}
	}
	return stylesheet
}

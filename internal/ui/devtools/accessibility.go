package devtools

import (
	"github.com/vyquocvu/goosie/internal/renderer"
)

type renderTreeAccessibilityProvider struct {
	root func() *renderer.RenderNode
}

func NewRenderTreeAccessibilityProvider(root func() *renderer.RenderNode) *renderTreeAccessibilityProvider {
	return &renderTreeAccessibilityProvider{root: root}
}

func (p *renderTreeAccessibilityProvider) GetAccessibilityTree() []*A11yNode {
	r := p.root()
	if r == nil {
		return nil
	}
	var result []*A11yNode
	for _, child := range r.Children {
		n := walkA11yNode(child)
		if n != nil {
			result = append(result, n)
		}
	}
	return result
}

func walkA11yNode(node *renderer.RenderNode) *A11yNode {
	if node == nil {
		return nil
	}
	n := &A11yNode{
		Tag:  node.TagName,
		Role: computeA11yRole(node),
		Name: computeA11yName(node),
	}
	if node.Type == renderer.NodeTypeText {
		n.Role = "text"
		n.Description = node.Text
		if len(node.Text) > 80 {
			n.Description = node.Text[:80] + "…"
		}
	}
	hasInteractive := false
	for _, child := range node.Children {
		cn := walkA11yNode(child)
		if cn != nil {
			n.Children = append(n.Children, cn)
			if cn.Role != "" && cn.Role != "text" {
				hasInteractive = true
			}
		}
	}
	if n.Tag == "" && n.Role == "" && len(n.Children) == 0 {
		return nil
	}
	if n.Role == "" && !hasInteractive && n.Tag != "" {
		n.Role = inferPresentationRole(node.TagName)
	}
	return n
}

func computeA11yRole(node *renderer.RenderNode) string {
	if node.Attrs != nil {
		if r, ok := node.Attrs["role"]; ok && r != "" {
			return r
		}
	}
	switch node.TagName {
	case "a":
		if _, ok := node.Attrs["href"]; ok {
			return "link"
		}
		return "anchor"
	case "button":
		return "button"
	case "img":
		return "img"
	case "input":
		t := node.Attrs["type"]
		switch t {
		case "checkbox":
			return "checkbox"
		case "radio":
			return "radio"
		case "submit", "button":
			return "button"
		default:
			return "textbox"
		}
	case "textarea":
		return "textbox"
	case "select":
		return "listbox"
	case "nav":
		return "navigation"
	case "main":
		return "main"
	case "header":
		return "banner"
	case "footer":
		return "contentinfo"
	case "aside":
		return "complementary"
	case "form":
		return "form"
	case "table":
		return "table"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "ul", "ol":
		return "list"
	case "li":
		return "listitem"
	}
	return ""
}

func computeA11yName(node *renderer.RenderNode) string {
	if node.Attrs != nil {
		if l, ok := node.Attrs["aria-label"]; ok && l != "" {
			return l
		}
		if l, ok := node.Attrs["aria-labelledby"]; ok && l != "" {
			return "[labelledby: " + l + "]"
		}
		if l, ok := node.Attrs["title"]; ok && l != "" {
			return l
		}
		if l, ok := node.Attrs["alt"]; ok && l != "" {
			return l
		}
		if l, ok := node.Attrs["placeholder"]; ok && l != "" {
			return l
		}
	}
	if node.TagName == "a" && node.Attrs != nil {
		if h, ok := node.Attrs["href"]; ok && h != "" {
			if len(h) < 60 {
				return h
			}
		}
	}
	if node.Type == renderer.NodeTypeText && node.Text != "" {
		t := node.Text
		if len(t) > 60 {
			t = t[:60] + "…"
		}
		return t
	}
	return ""
}

func inferPresentationRole(tag string) string {
	switch tag {
	case "div", "span", "section", "article", "p", "blockquote", "pre", "code",
		"figure", "figcaption", "details", "summary", "dialog", "address",
		"strong", "em", "mark", "small", "sub", "sup", "ins", "del", "s",
		"abbr", "cite", "dfn", "kbd", "samp", "var", "time", "b", "i", "u":
		return "presentation"
	}
	return "unknown"
}

func formatA11yTree(nodes []*A11yNode, indent string) string {
	var s string
	for _, n := range nodes {
		s += formatA11yNode(n, indent)
	}
	return s
}

func formatA11yNode(n *A11yNode, indent string) string {
	if n == nil {
		return ""
	}
	role := n.Role
	if role == "" {
		role = "unknown"
	}
	name := n.Name
	if name != "" {
		name = " \"" + name + "\""
	}
	extra := ""
	if n.Description != "" {
		extra = " [" + n.Description + "]"
	}
	s := indent + "<" + n.Tag + "> role=" + role + name + extra + "\n"
	for _, c := range n.Children {
		s += formatA11yNode(c, indent+"  ")
	}
	return s
}

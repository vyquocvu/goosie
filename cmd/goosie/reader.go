package main

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"unicode"

	"github.com/vyquocvu/goosie/internal/dom"
)

// Article holds distilled distraction-free content extracted from a webpage.
type Article struct {
	Title       string
	Byline      string
	ContentHTML string
	TextLength  int
	WordCount   int
	ReadingTime int // in minutes (assumes 200 WPM)
	URL         string
	Theme       string // "light", "sepia", "dark"
}

// ExtractArticle analyzes a DOM tree and extracts clean readability content.
func ExtractArticle(doc *dom.Document, pageURL, theme string) *Article {
	if theme == "" {
		theme = "light"
	}
	art := &Article{
		URL:   pageURL,
		Theme: theme,
	}

	if doc == nil {
		art.Title = "Reader Mode"
		art.ContentHTML = `<p>No readable content available.</p>`
		art.ReadingTime = 1
		return art
	}

	root := &doc.Node

	// Extract Title
	art.Title = extractTitle(root)
	if art.Title == "" && pageURL != "" {
		if u, err := url.Parse(pageURL); err == nil {
			art.Title = u.Host + u.Path
		}
	}
	if art.Title == "" {
		art.Title = "Untitled Article"
	}

	// Extract Byline
	art.Byline = extractByline(root, pageURL)

	// Find best container
	candidate := findBestArticleNode(root)
	if candidate == nil {
		candidate = findBodyNode(root)
	}

	var contentBuilder strings.Builder
	var pureTextBuilder strings.Builder
	if candidate != nil {
		renderReadableNode(&contentBuilder, &pureTextBuilder, candidate, 0)
	}

	content := contentBuilder.String()
	if strings.TrimSpace(content) == "" {
		content = `<p>Unable to extract readable article text from this page.</p>`
	}
	art.ContentHTML = content

	// Count words and reading time
	text := pureTextBuilder.String()
	art.TextLength = len(text)
	words := countWords(text)
	art.WordCount = words
	art.ReadingTime = (words + 199) / 200
	if art.ReadingTime < 1 {
		art.ReadingTime = 1
	}

	return art
}

// extractTitle finds the title from <title>, <meta og:title>, or the first <h1>.
func extractTitle(root *dom.Node) string {
	var title string
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil || title != "" {
			return
		}
		if n.Type == dom.NodeElement {
			tag := strings.ToLower(n.Data)
			if tag == "title" {
				title = strings.TrimSpace(nodeText(n))
				return
			}
			if tag == "meta" {
				prop := strings.ToLower(getAttr(n, "property"))
				if prop == "og:title" {
					c := strings.TrimSpace(getAttr(n, "content"))
					if c != "" {
						title = c
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	if title == "" {
		var findH1 func(n *dom.Node)
		findH1 = func(n *dom.Node) {
			if n == nil || title != "" {
				return
			}
			if n.Type == dom.NodeElement && strings.ToLower(n.Data) == "h1" {
				title = strings.TrimSpace(nodeText(n))
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findH1(c)
			}
		}
		findH1(root)
	}

	return title
}

// extractByline looks for author meta tags, site names, or URL host.
func extractByline(root *dom.Node, pageURL string) string {
	var byline string
	var siteName string
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Type == dom.NodeElement && strings.ToLower(n.Data) == "meta" {
			name := strings.ToLower(getAttr(n, "name"))
			prop := strings.ToLower(getAttr(n, "property"))
			content := strings.TrimSpace(getAttr(n, "content"))
			if content != "" {
				if name == "author" || prop == "article:author" {
					byline = content
				} else if prop == "og:site_name" {
					siteName = content
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	if byline != "" && siteName != "" {
		return fmt.Sprintf("%s &bull; %s", html.EscapeString(byline), html.EscapeString(siteName))
	}
	if byline != "" {
		return html.EscapeString(byline)
	}
	if siteName != "" {
		return html.EscapeString(siteName)
	}
	if pageURL != "" {
		if u, err := url.Parse(pageURL); err == nil && u.Host != "" {
			return html.EscapeString(u.Host)
		}
	}
	return ""
}

// findBodyNode locates the <body> element in the tree.
func findBodyNode(root *dom.Node) *dom.Node {
	var body *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil || body != nil {
			return
		}
		if n.Type == dom.NodeElement && strings.ToLower(n.Data) == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if body != nil {
		return body
	}
	return root
}

// findBestArticleNode finds the container with the highest readability score.
func findBestArticleNode(root *dom.Node) *dom.Node {
	var bestNode *dom.Node
	bestScore := 0

	var evaluate func(n *dom.Node)
	evaluate = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Type == dom.NodeElement {
			tag := strings.ToLower(n.Data)
			if isBoilerplateOrNoise(n) {
				return
			}

			if tag == "article" || tag == "main" || tag == "section" || tag == "div" {
				score := calculateReadabilityScore(n)
				if tag == "article" {
					score += 30
				} else if tag == "main" {
					score += 20
				}
				if score > bestScore {
					bestScore = score
					bestNode = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			evaluate(c)
		}
	}
	evaluate(root)

	if bestScore >= 20 {
		return bestNode
	}
	return nil
}

// calculateReadabilityScore estimates the article density of a container node.
func calculateReadabilityScore(node *dom.Node) int {
	score := 0
	pCount := 0
	textLen := 0
	linkTextLen := 0

	var scan func(n *dom.Node, insideLink bool)
	scan = func(n *dom.Node, insideLink bool) {
		if n == nil {
			return
		}
		if n.Type == dom.NodeElement {
			if isBoilerplateOrNoise(n) {
				return
			}
			tag := strings.ToLower(n.Data)
			if tag == "p" || tag == "blockquote" {
				pCount++
				score += 5
			} else if tag == "h2" || tag == "h3" || tag == "h4" {
				score += 3
			} else if tag == "pre" || tag == "code" {
				score += 2
			} else if tag == "li" {
				score += 1
			}
			if tag == "a" {
				insideLink = true
			}
		} else if n.Type == dom.NodeText {
			txt := strings.TrimSpace(n.Data)
			if txt == "" {
				txt = strings.TrimSpace(n.DataContent)
			}
			l := len(txt)
			textLen += l
			if insideLink {
				linkTextLen += l
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			scan(c, insideLink)
		}
	}
	scan(node, false)

	// High link density penalty (navigation lists / blog rolls)
	if textLen > 0 {
		linkDensity := float64(linkTextLen) / float64(textLen)
		if linkDensity > 0.5 {
			return 0
		}
		score += textLen / 100
	}

	return score
}

// renderReadableNode formats content into clean, semantic reader HTML.
func renderReadableNode(sb, pure *strings.Builder, n *dom.Node, depth int) {
	if n == nil {
		return
	}
	if n.Type == dom.NodeText {
		txt := strings.TrimSpace(n.Data)
		if txt == "" {
			txt = strings.TrimSpace(n.DataContent)
		}
		if txt != "" {
			sb.WriteString(html.EscapeString(txt))
			pure.WriteString(txt)
			pure.WriteString(" ")
		}
		return
	}

	if n.Type != dom.NodeElement {
		return
	}

	if isBoilerplateOrNoise(n) {
		return
	}

	tag := strings.ToLower(n.Data)
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		sb.WriteString(fmt.Sprintf("\n<%s>", tag))
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString(fmt.Sprintf("</%s>\n", tag))
	case "p":
		sb.WriteString("\n<p>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</p>\n")
	case "blockquote":
		sb.WriteString("\n<blockquote>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</blockquote>\n")
	case "ul":
		sb.WriteString("\n<ul>\n")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</ul>\n")
	case "ol":
		sb.WriteString("\n<ol>\n")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</ol>\n")
	case "li":
		sb.WriteString("<li>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</li>\n")
	case "pre":
		sb.WriteString("\n<pre><code>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</code></pre>\n")
	case "code":
		sb.WriteString("<code>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</code>")
	case "b", "strong":
		sb.WriteString("<b>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</b>")
	case "i", "em":
		sb.WriteString("<i>")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString("</i>")
	case "a":
		href := getAttr(n, "href")
		if href != "" {
			sb.WriteString(fmt.Sprintf(`<a href="%s">`, html.EscapeString(href)))
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				renderReadableNode(sb, pure, c, depth+1)
			}
			sb.WriteString("</a>")
		} else {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				renderReadableNode(sb, pure, c, depth+1)
			}
		}
	case "img":
		src := getAttr(n, "src")
		alt := getAttr(n, "alt")
		if src != "" {
			sb.WriteString(fmt.Sprintf(`<img src="%s" alt="%s">`, html.EscapeString(src), html.EscapeString(alt)))
		}
	case "hr":
		sb.WriteString("\n<hr>\n")
	case "table", "tbody", "thead", "tr", "th", "td":
		sb.WriteString(fmt.Sprintf("<%s>", tag))
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
		sb.WriteString(fmt.Sprintf("</%s>", tag))
	default:
		// For other elements (e.g. div, span, section), render children
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderReadableNode(sb, pure, c, depth+1)
		}
	}
}

// isBoilerplateOrNoise checks if an element represents noise or layout junk.
func isBoilerplateOrNoise(n *dom.Node) bool {
	if n == nil || n.Type != dom.NodeElement {
		return false
	}
	tag := strings.ToLower(n.Data)
	switch tag {
	case "script", "style", "noscript", "iframe", "nav", "header", "footer", "aside",
		"form", "button", "input", "select", "textarea", "svg", "canvas", "dialog", "menu":
		return true
	}
	cls := strings.ToLower(getAttr(n, "class"))
	id := strings.ToLower(getAttr(n, "id"))
	combined := cls + " " + id
	noisePatterns := []string{
		"comment", "disqus", "share", "social", "sidebar", "ad-", "ads", "advert",
		"banner", "cookie", "popup", "modal", "widget", "newsletter", "subscribe",
		"footer", "nav", "menu", "promo", "sponsor",
	}
	for _, pat := range noisePatterns {
		if strings.Contains(combined, pat) {
			return true
		}
	}
	return false
}

func getAttr(n *dom.Node, name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if strings.EqualFold(a.Name, name) {
			return a.Value
		}
	}
	return ""
}

func nodeText(n *dom.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == dom.NodeText {
		t := strings.TrimSpace(n.Data)
		if t == "" {
			t = strings.TrimSpace(n.DataContent)
		}
		return t
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s := nodeText(c)
		if s != "" {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(s)
		}
	}
	return sb.String()
}

func countWords(s string) int {
	words := 0
	inWord := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	return words
}

// RenderReaderPageHTML generates the full distraction-free reading experience.
func RenderReaderPageHTML(art *Article) string {
	bg := "#fdfdfd"
	fg := "#222222"
	linkCol := "#1a73e8"
	barBg := "#ffffff"
	barBorder := "#e8eaed"
	preBg := "#f1f3f4"
	quoteBorder := "#1a73e8"

	switch art.Theme {
	case "sepia":
		bg = "#fbf0d9"
		fg = "#5f4b32"
		linkCol = "#8b4513"
		barBg = "#f4ecd8"
		barBorder = "#e2d7be"
		preBg = "#ede3ca"
		quoteBorder = "#c49a45"
	case "dark":
		bg = "#1e1e24"
		fg = "#dcdfe4"
		linkCol = "#61afef"
		barBg = "#282c34"
		barBorder = "#3e4451"
		preBg = "#2c313a"
		quoteBorder = "#61afef"
	}

	themeURL := func(t string) string {
		return fmt.Sprintf("goosie://reader?url=%s&theme=%s", url.QueryEscape(art.URL), t)
	}

	backURL := art.URL
	if backURL == "" {
		backURL = "goosie://version"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>%s &mdash; Reader Mode</title>
<style>
body {
  margin: 0;
  padding: 0;
  background-color: %s;
  color: %s;
  font-family: Georgia, Cambria, "Times New Roman", serif;
  font-size: 19px;
  line-height: 1.8;
  -webkit-font-smoothing: antialiased;
}
.reader-bar {
  position: sticky;
  top: 0;
  background-color: %s;
  border-bottom: 1px solid %s;
  padding: 10px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: 13px;
  z-index: 100;
}
.reader-bar a {
  text-decoration: none;
  color: %s;
  font-weight: 500;
}
.reader-bar .theme-group {
  display: flex;
  gap: 8px;
}
.theme-pill {
  padding: 4px 10px;
  border-radius: 12px;
  border: 1px solid %s;
  cursor: pointer;
}
.theme-pill.active {
  font-weight: bold;
  border-width: 2px;
}
.article-container {
  max-width: 680px;
  margin: 40px auto 80px auto;
  padding: 0 24px;
}
h1.article-title {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: 34px;
  line-height: 1.25;
  font-weight: 700;
  margin-bottom: 12px;
}
.article-meta {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: 14px;
  color: #70757a;
  margin-bottom: 36px;
  padding-bottom: 16px;
  border-bottom: 1px solid %s;
}
p {
  margin-top: 0;
  margin-bottom: 24px;
}
h2, h3, h4 {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  margin-top: 36px;
  margin-bottom: 16px;
  line-height: 1.3;
}
blockquote {
  border-left: 4px solid %s;
  margin: 24px 0;
  padding: 8px 16px;
  font-style: italic;
}
pre {
  background-color: %s;
  padding: 16px;
  border-radius: 6px;
  overflow-x: auto;
  font-family: Consolas, Monaco, monospace;
  font-size: 14px;
  line-height: 1.5;
}
code {
  font-family: Consolas, Monaco, monospace;
  font-size: 15px;
}
img {
  max-width: 100%%;
  height: auto;
  border-radius: 6px;
  margin: 16px 0;
}
a {
  color: %s;
}
ul, ol {
  margin-bottom: 24px;
  padding-left: 28px;
}
li {
  margin-bottom: 8px;
}
</style>
</head>
<body>
<div class="reader-bar">
  <div>
    <a href="%s">&larr; Back to original page</a>
  </div>
  <div style="color: #70757a;">
    ⏱️ %d min read &bull; %d words
  </div>
  <div class="theme-group">
    <a href="%s" class="theme-pill%s">Light</a>
    <a href="%s" class="theme-pill%s">Sepia</a>
    <a href="%s" class="theme-pill%s">Dark</a>
  </div>
</div>

<div class="article-container">
  <h1 class="article-title">%s</h1>
  <div class="article-meta">
    %s
  </div>
  <div class="article-content">
    %s
  </div>
</div>
</body>
</html>`,
		html.EscapeString(art.Title),
		bg, fg, barBg, barBorder, linkCol, barBorder,
		barBorder, quoteBorder, preBg, linkCol,
		html.EscapeString(backURL),
		art.ReadingTime, art.WordCount,
		themeURL("light"), activeThemeClass("light", art.Theme),
		themeURL("sepia"), activeThemeClass("sepia", art.Theme),
		themeURL("dark"), activeThemeClass("dark", art.Theme),
		html.EscapeString(art.Title),
		art.Byline,
		art.ContentHTML,
	)
}

func activeThemeClass(target, current string) string {
	if target == current {
		return " active"
	}
	return ""
}

// Package viewsource formats and syntax-highlights source code for view-source presentation.
package viewsource

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const viewSourcePrefix = "view-source:"

// IsViewSourceURL checks if rawURL starts with the view-source: scheme.
// If so, it returns the unadorned target URL and true.
func IsViewSourceURL(rawURL string) (targetURL string, isViewSource bool) {
	if strings.HasPrefix(strings.ToLower(rawURL), viewSourcePrefix) {
		return strings.TrimSpace(rawURL[len(viewSourcePrefix):]), true
	}
	return rawURL, false
}

// WrapViewSourceURL formats targetURL into a view-source: URL.
func WrapViewSourceURL(targetURL string) string {
	if strings.HasPrefix(strings.ToLower(targetURL), viewSourcePrefix) {
		return targetURL
	}
	return viewSourcePrefix + targetURL
}

var (
	reComment = regexp.MustCompile(`<!--[\s\S]*?-->`)
	reDoctype = regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>`)
	reTag     = regexp.MustCompile(`</?[a-zA-Z0-9:-]+(?:\s+[^>]*)?/?>`)
	reAttr    = regexp.MustCompile(`([a-zA-Z0-9:-]+)(?:=("(?:[^"]*)"|'(?:[^']*)'|[^>\s]+))?`)
)

// HighlightHTML applies syntax highlighting to raw HTML markup.
func HighlightHTML(src, baseURL string) string {
	var b strings.Builder
	idx := 0
	n := len(src)

	resolve := func(link string) string {
		link = strings.Trim(link, "\"'")
		if strings.HasPrefix(link, "javascript:") || strings.HasPrefix(link, "#") {
			return link
		}
		if baseURL == "" {
			return link
		}
		base, err := url.Parse(baseURL)
		if err != nil {
			return link
		}
		ref, err := url.Parse(link)
		if err != nil {
			return link
		}
		return base.ResolveReference(ref).String()
	}

	for idx < n {
		if strings.HasPrefix(src[idx:], "<!--") {
			loc := reComment.FindStringIndex(src[idx:])
			if loc != nil {
				commentText := src[idx : idx+loc[1]]
				b.WriteString(`<span class="token-comment">` + html.EscapeString(commentText) + `</span>`)
				idx += loc[1]
				continue
			}
		}

		if strings.HasPrefix(strings.ToLower(src[idx:]), "<!doctype") {
			loc := reDoctype.FindStringIndex(src[idx:])
			if loc != nil {
				dtText := src[idx : idx+loc[1]]
				b.WriteString(`<span class="token-doctype">` + html.EscapeString(dtText) + `</span>`)
				idx += loc[1]
				continue
			}
		}

		if src[idx] == '<' {
			end := strings.IndexByte(src[idx:], '>')
			if end != -1 {
				rawTag := src[idx : idx+end+1]
				highlightedTag := highlightSingleTag(rawTag, resolve)
				b.WriteString(highlightedTag)
				idx += end + 1
				continue
			}
		}

		// Text token
		nextOpen := strings.IndexByte(src[idx:], '<')
		if nextOpen == -1 {
			b.WriteString(html.EscapeString(src[idx:]))
			break
		}
		b.WriteString(html.EscapeString(src[idx : idx+nextOpen]))
		idx += nextOpen
	}

	return b.String()
}

func highlightSingleTag(rawTag string, resolveLink func(string) string) string {
	// Parse opening < or </
	prefix := "<"
	inner := rawTag[1 : len(rawTag)-1]
	suffix := ">"
	if strings.HasPrefix(rawTag, "</") {
		prefix = "</"
		inner = rawTag[2 : len(rawTag)-1]
	}
	if strings.HasSuffix(inner, "/") {
		inner = inner[:len(inner)-1]
		suffix = "/>"
	}

	// Extract tag name
	tagNameEnd := strings.IndexAny(inner, " \t\r\n")
	tagName := inner
	attrsPart := ""
	if tagNameEnd != -1 {
		tagName = inner[:tagNameEnd]
		attrsPart = inner[tagNameEnd:]
	}

	var b strings.Builder
	b.WriteString(`<span class="token-tag">` + html.EscapeString(prefix) + html.EscapeString(tagName))

	if attrsPart != "" {
		matches := reAttr.FindAllStringSubmatchIndex(attrsPart, -1)
		lastPos := 0
		for _, m := range matches {
			if m[0] > lastPos {
				b.WriteString(html.EscapeString(attrsPart[lastPos:m[0]]))
			}
			attrName := attrsPart[m[2]:m[3]]
			b.WriteString(`<span class="token-attr-name">` + html.EscapeString(attrName) + `</span>`)

			if m[4] != -1 && m[5] != -1 {
				valPart := attrsPart[m[4]:m[5]]
				eqVal := "="
				valContent := valPart
				if strings.HasPrefix(valPart, "=") {
					valContent = valPart[1:]
				}
				b.WriteString(html.EscapeString(eqVal))

				isLinkAttr := strings.EqualFold(attrName, "href") || strings.EqualFold(attrName, "src")
				if isLinkAttr {
					unquoted := strings.Trim(valContent, "\"'")
					resolved := resolveLink(unquoted)
					viewLink := WrapViewSourceURL(resolved)
					if strings.HasPrefix(unquoted, "http://") || strings.HasPrefix(unquoted, "https://") || strings.HasPrefix(unquoted, "/") {
						b.WriteString(fmt.Sprintf(`<a class="source-link" href="%s"><span class="token-attr-value">%s</span></a>`,
							html.EscapeString(viewLink), html.EscapeString(valContent)))
					} else {
						b.WriteString(`<span class="token-attr-value">` + html.EscapeString(valContent) + `</span>`)
					}
				} else {
					b.WriteString(`<span class="token-attr-value">` + html.EscapeString(valContent) + `</span>`)
				}
			}
			lastPos = m[1]
		}
		if lastPos < len(attrsPart) {
			b.WriteString(html.EscapeString(attrsPart[lastPos:]))
		}
	}

	b.WriteString(html.EscapeString(suffix) + `</span>`)
	return b.String()
}

// RenderHTML formats raw source code into an HTML view-source document with line numbers.
func RenderHTML(source, targetURL string, darkMode bool) string {
	lines := strings.Split(source, "\n")
	totalLines := len(lines)
	numDigits := len(strconv.Itoa(totalLines))
	if numDigits < 2 {
		numDigits = 2
	}

	bg := "#ffffff"
	fg := "#202124"
	gutterBg := "#f8f9fa"
	gutterFg := "#80868b"
	gutterBorder := "#dadce0"
	tagColor := "#1a73e8"
	attrColor := "#c5221f"
	valColor := "#188038"
	commentColor := "#5f6368"
	doctypeColor := "#8430ce"
	headerBg := "#f1f3f4"
	headerBorder := "#dadce0"
	linkHoverBg := "#e8f0fe"

	if darkMode {
		bg = "#1e1e1e"
		fg = "#e8eaed"
		gutterBg = "#252526"
		gutterFg = "#858585"
		gutterBorder = "#3c4043"
		tagColor = "#569cd6"
		attrColor = "#9cdcfe"
		valColor = "#ce9178"
		commentColor = "#6a9955"
		doctypeColor = "#c586c0"
		headerBg = "#2d2d2d"
		headerBorder = "#3c4043"
		linkHoverBg = "#37373d"
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>view-source:` + html.EscapeString(targetURL) + `</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background-color: ` + bg + `;
      color: ` + fg + `;
      font-family: monospace;
      font-size: 13px;
      line-height: 19px;
    }
    .header {
      background: ` + headerBg + `;
      border-bottom: 1px solid ` + headerBorder + `;
      padding: 8px 16px;
      font-family: sans-serif;
      font-size: 13px;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .header-title {
      font-weight: 600;
      color: ` + fg + `;
      word-break: break-all;
    }
    .header-stats {
      color: ` + gutterFg + `;
      font-size: 12px;
      white-space: nowrap;
      margin-left: 16px;
    }
    .source-container {
      display: table;
      width: 100%;
      border-collapse: collapse;
    }
    .line-row {
      display: table-row;
    }
    .line-row:hover {
      background-color: ` + linkHoverBg + `;
    }
    .line-number {
      display: table-cell;
      width: ` + strconv.Itoa(numDigits*10+24) + `px;
      padding: 0 12px 0 8px;
      text-align: right;
      color: ` + gutterFg + `;
      background-color: ` + gutterBg + `;
      border-right: 1px solid ` + gutterBorder + `;
      user-select: none;
      vertical-align: top;
      white-space: nowrap;
    }
    .line-content {
      display: table-cell;
      padding: 0 12px;
      white-space: pre-wrap;
      word-break: break-all;
      vertical-align: top;
    }
    .token-comment { color: ` + commentColor + `; font-style: italic; }
    .token-doctype { color: ` + doctypeColor + `; font-weight: bold; }
    .token-tag { color: ` + tagColor + `; font-weight: bold; }
    .token-attr-name { color: ` + attrColor + `; }
    .token-attr-value { color: ` + valColor + `; }
    .source-link {
      text-decoration: underline;
      color: inherit;
    }
    .source-link:hover {
      background-color: ` + linkHoverBg + `;
      border-radius: 2px;
    }
  </style>
</head>
<body>
  <div class="header">
    <div class="header-title">view-source:` + html.EscapeString(targetURL) + `</div>
    <div class="header-stats">` + strconv.Itoa(totalLines) + ` lines &bull; ` + strconv.Itoa(len(source)) + ` bytes</div>
  </div>
  <div class="source-container">`)

	for i, line := range lines {
		lineNum := i + 1
		highlightedLine := HighlightHTML(line, targetURL)
		if highlightedLine == "" {
			highlightedLine = " "
		}
		b.WriteString(fmt.Sprintf(`
    <div class="line-row" id="line-%d">
      <div class="line-number">%d</div>
      <div class="line-content">%s</div>
    </div>`, lineNum, lineNum, highlightedLine))
	}

	b.WriteString(`
  </div>
</body>
</html>`)

	return b.String()
}

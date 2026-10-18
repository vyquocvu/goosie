// Package pdf implements a pure Go PDF document inspector, text extractor,
// and viewer page renderer for in-browser PDF viewing without external tools.
package pdf

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Page represents a single page in a PDF document.
type Page struct {
	Number int
	Lines  []string
}

// Document represents a parsed PDF document.
type Document struct {
	Version string
	Title   string
	Author  string
	Pages   []Page
}

// IsPDF reports whether data, contentType, or URL represents a PDF document.
func IsPDF(data []byte, contentType, rawURL string) bool {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return true
	}
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "application/pdf") || strings.Contains(ct, "application/x-pdf") {
		return true
	}
	if u, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(path.Ext(u.Path))
		if ext == ".pdf" {
			return true
		}
	}
	return false
}

// Parse extracts text content and metadata from a PDF byte slice.
func Parse(data []byte, fallbackTitle string) (*Document, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		// Tolerant search for header within first 1024 bytes
		idx := bytes.Index(data, []byte("%PDF-"))
		if idx == -1 || idx > 1024 {
			return nil, fmt.Errorf("pdf: invalid PDF header")
		}
		data = data[idx:]
	}

	doc := &Document{
		Version: "1.4",
		Title:   fallbackTitle,
	}

	// Extract version from header: %PDF-1.x
	if len(data) >= 8 && string(data[:5]) == "%PDF-" {
		end := bytes.IndexByte(data[:15], '\n')
		if end == -1 {
			end = bytes.IndexByte(data[:15], '\r')
		}
		if end > 5 {
			doc.Version = strings.TrimSpace(string(data[5:end]))
		}
	}

	// Extract Title from /Title (Value)
	if titleMatch := regexp.MustCompile(`(?i)/Title\s*\(([^)]+)\)`).FindSubmatch(data); len(titleMatch) > 1 {
		doc.Title = decodePDFString(string(titleMatch[1]))
	}
	if doc.Title == "" {
		doc.Title = fallbackTitle
	}

	// Find all streams
	streams := extractStreams(data)
	var allLines []string

	for _, s := range streams {
		text := extractTextFromStream(s)
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				allLines = append(allLines, trimmed)
			}
		}
	}

	// If stream extraction produced no lines, scan for uncompressed text literals
	if len(allLines) == 0 {
		reText := regexp.MustCompile(`\(([^\r\n()]{3,})\)\s*(?:Tj|'|")`)
		matches := reText.FindAllSubmatch(data, -1)
		for _, m := range matches {
			if len(m) > 1 {
				decoded := decodePDFString(string(m[1]))
				if len(strings.TrimSpace(decoded)) > 0 {
					allLines = append(allLines, strings.TrimSpace(decoded))
				}
			}
		}
	}

	// Group extracted lines into pages (e.g. ~40 lines per page, or split on /Page markers)
	const linesPerPage = 45
	if len(allLines) == 0 {
		doc.Pages = []Page{
			{Number: 1, Lines: []string{"[PDF document contains no extractable text or consists exclusively of raster images]"}},
		}
	} else {
		for i := 0; i < len(allLines); i += linesPerPage {
			end := i + linesPerPage
			if end > len(allLines) {
				end = len(allLines)
			}
			doc.Pages = append(doc.Pages, Page{
				Number: len(doc.Pages) + 1,
				Lines:  allLines[i:end],
			})
		}
	}

	return doc, nil
}

// extractStreams locates all stream...endstream blocks and decodes /FlateDecode if compressed.
func extractStreams(data []byte) [][]byte {
	var streams [][]byte
	streamTag := []byte("stream")
	endStreamTag := []byte("endstream")

	offset := 0
	for {
		startIdx := bytes.Index(data[offset:], streamTag)
		if startIdx == -1 {
			break
		}
		absStreamStart := offset + startIdx
		start := absStreamStart + len(streamTag)
		if start < len(data) && data[start] == '\r' {
			start++
		}
		if start < len(data) && data[start] == '\n' {
			start++
		}

		endIdx := bytes.Index(data[start:], endStreamTag)
		if endIdx == -1 {
			break
		}
		streamData := data[start : start+endIdx]
		offset = start + endIdx + len(endStreamTag)

		// Check if preceding dictionary declared /FlateDecode
		dictSearchStart := absStreamStart - 200
		if dictSearchStart < 0 {
			dictSearchStart = 0
		}
		prevDict := data[dictSearchStart:absStreamStart]

		if bytes.Contains(prevDict, []byte("/FlateDecode")) || (len(streamData) > 2 && streamData[0] == 0x78) {
			zr, err := zlib.NewReader(bytes.NewReader(streamData))
			if err == nil {
				decompressed, dErr := io.ReadAll(zr)
				_ = zr.Close()
				if dErr == nil && len(decompressed) > 0 {
					streams = append(streams, decompressed)
					continue
				}
			}
		}

		streams = append(streams, streamData)
	}
	return streams
}

// extractTextFromStream parses PDF text operators: BT, ET, Tj, TJ, ', "
func extractTextFromStream(stream []byte) string {
	var sb strings.Builder
	s := string(stream)

	// Pattern 1: Tj string literals: (Hello World) Tj
	reTj := regexp.MustCompile(`\(([^()]+)\)\s*Tj`)
	matchesTj := reTj.FindAllStringSubmatch(s, -1)
	for _, m := range matchesTj {
		sb.WriteString(decodePDFString(m[1]))
		sb.WriteString("\n")
	}

	// Pattern 2: Hex string literals: <48656c6c6f> Tj
	reHex := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*Tj`)
	matchesHex := reHex.FindAllStringSubmatch(s, -1)
	for _, m := range matchesHex {
		decodedBytes, err := hex.DecodeString(m[1])
		if err == nil {
			sb.WriteString(string(decodedBytes))
			sb.WriteString("\n")
		}
	}

	// Pattern 3: Array of text chunks: [ (Chunk1) 20 (Chunk2) ] TJ
	reTJ := regexp.MustCompile(`\[([^\]]+)\]\s*TJ`)
	matchesTJ := reTJ.FindAllStringSubmatch(s, -1)
	for _, m := range matchesTJ {
		inner := m[1]
		reParts := regexp.MustCompile(`\(([^()]+)\)`)
		parts := reParts.FindAllStringSubmatch(inner, -1)
		for _, p := range parts {
			sb.WriteString(decodePDFString(p[1]))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// decodePDFString unescapes standard PDF string escapes.
func decodePDFString(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case '(', ')', '\\':
				sb.WriteByte(s[i])
			case '0', '1', '2', '3', '4', '5', '6', '7':
				// Octal escape \ddd
				oct := string(s[i])
				if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7' {
					i++
					oct += string(s[i])
					if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7' {
						i++
						oct += string(s[i])
					}
				}
				if val, err := strconv.ParseInt(oct, 8, 32); err == nil {
					sb.WriteByte(byte(val))
				}
			default:
				sb.WriteByte(s[i])
			}
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// RenderHTML generates a full HTML page for the PDF viewer.
func RenderHTML(doc *Document, rawURL string) string {
	title := doc.Title
	if title == "" {
		title = "PDF Document"
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>` + html.EscapeString(title) + ` - PDF Viewer</title>
<style>
body {
  margin: 0;
  padding: 0;
  background-color: #525659;
  color: #202124;
  font-family: sans-serif;
}
.pdf-toolbar {
  position: sticky;
  top: 0;
  left: 0;
  right: 0;
  background-color: #323639;
  color: #f1f3f4;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  box-shadow: 0 2px 6px rgba(0,0,0,0.4);
  z-index: 1000;
}
.pdf-title {
  font-size: 14px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 400px;
}
.pdf-meta {
  font-size: 13px;
  color: #bdc1c6;
}
.pdf-actions a {
  color: #ffffff;
  text-decoration: none;
  background-color: #1a73e8;
  padding: 6px 14px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: bold;
  margin-left: 12px;
}
.pdf-container {
  max-width: 860px;
  margin: 24px auto;
  padding-bottom: 48px;
}
.pdf-page {
  background-color: #ffffff;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.35);
  margin-bottom: 24px;
  padding: 56px 64px;
  min-height: 800px;
  border-radius: 2px;
  box-sizing: border-box;
}
.pdf-page-header {
  border-bottom: 1px solid #e0e0e0;
  padding-bottom: 8px;
  margin-bottom: 24px;
  font-size: 12px;
  color: #757575;
  display: flex;
  justify-content: space-between;
}
.pdf-text {
  font-family: 'Times New Roman', 'Liberation Serif', serif;
  font-size: 15px;
  line-height: 1.7;
  color: #111111;
  word-wrap: break-word;
}
.pdf-text p {
  margin: 0 0 12px 0;
}
.pdf-page-footer {
  border-top: 1px solid #f0f0f0;
  padding-top: 12px;
  margin-top: 36px;
  text-align: center;
  font-size: 12px;
  color: #9e9e9e;
}
@media (prefers-color-scheme: dark) {
  .pdf-toolbar { background-color: #202124; }
}
</style>
</head>
<body>
<div class="pdf-toolbar">
  <div class="pdf-title">` + html.EscapeString(title) + `</div>
  <div class="pdf-meta">` + fmt.Sprintf("%d Pages &bull; PDF %s", len(doc.Pages), html.EscapeString(doc.Version)) + `</div>
  <div class="pdf-actions">
    <a href="javascript:window.print()">Print</a>
  </div>
</div>
<div class="pdf-container">`)

	for _, p := range doc.Pages {
		b.WriteString(fmt.Sprintf(`
  <div class="pdf-page">
    <div class="pdf-page-header">
      <span>%s</span>
      <span>Page %d of %d</span>
    </div>
    <div class="pdf-text">`, html.EscapeString(title), p.Number, len(doc.Pages)))

		for _, line := range p.Lines {
			b.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(line)))
		}

		b.WriteString(fmt.Sprintf(`
    </div>
    <div class="pdf-page-footer">
      Page %d
    </div>
  </div>`, p.Number))
	}

	b.WriteString(`</div>
</body>
</html>`)

	return b.String()
}

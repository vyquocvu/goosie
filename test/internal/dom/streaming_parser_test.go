package dom_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/dom/atom"
	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// storeTree returns a human-readable tree string for debugging a Document.
func storeTree(t *testing.T, doc *dom.Document) string {
	t.Helper()
	var sb strings.Builder
	var walk func(id dom.NodeID, depth int)
	walk = func(id dom.NodeID, depth int) {
		if id == dom.NodeNone {
			return
		}
		for i := 0; i < depth; i++ {
			sb.WriteString("  ")
		}
		kind := doc.Store.Kind(id)
		switch kind {
		case dom.NodeKindDocument:
			sb.WriteString("#document\n")
		case dom.NodeKindDoctype:
			sb.WriteString("<!DOCTYPE>\n")
		case dom.NodeKindElement:
			sb.WriteString("<" + doc.Store.Name(id).String() + ">")
			attrs, _ := doc.Store.Attrs(id)
			for _, a := range attrs {
				sb.WriteString(" " + a.Name.String() + "=\"" + a.Value.String() + "\"")
			}
			sb.WriteString("\n")
		case dom.NodeKindText:
			text, _ := doc.Store.Text(id)
			sb.WriteString("#text \"" + text + "\"\n")
		case dom.NodeKindComment:
			text, _ := doc.Store.Text(id)
			sb.WriteString("<!-- " + text + " -->\n")
		}
		for child := doc.Store.FirstChild(id); child != dom.NodeNone; child = doc.Store.NextSibling(child) {
			walk(child, depth+1)
		}
	}
	walk(doc.Root, 0)
	return sb.String()
}

// findElement walks the Store looking for an element with the given tag name.
// Returns NodeNone if not found.
func findElement(t *testing.T, doc *dom.Document, tagName atom.Atom) dom.NodeID {
	t.Helper()
	for id := dom.NodeID(1); id <= dom.NodeID(doc.Store.NodeCount()); id++ {
		if doc.Store.Kind(id) == dom.NodeKindElement && doc.Store.Name(id) == tagName {
			return id
		}
	}
	return dom.NodeNone
}

// findAllElements returns all element NodeIDs matching the given tag name.
func findAllElements(t *testing.T, doc *dom.Document, tagName atom.Atom) []dom.NodeID {
	t.Helper()
	var result []dom.NodeID
	for id := dom.NodeID(1); id <= dom.NodeID(doc.Store.NodeCount()); id++ {
		if doc.Store.Kind(id) == dom.NodeKindElement && doc.Store.Name(id) == tagName {
			result = append(result, id)
		}
	}
	return result
}

// collectTexts walks the Store subtree rooted at id and collects all text content.
func collectTexts(t *testing.T, store *dom.Store, id dom.NodeID) []string {
	t.Helper()
	var texts []string
	var walk func(dom.NodeID)
	walk = func(n dom.NodeID) {
		if n == dom.NodeNone {
			return
		}
		if store.Kind(n) == dom.NodeKindText {
			txt, _ := store.Text(n)
			if txt != "" {
				texts = append(texts, txt)
			}
		}
		for c := store.FirstChild(n); c != dom.NodeNone; c = store.NextSibling(c) {
			walk(c)
		}
	}
	walk(id)
	return texts
}

// collectOldTreeTexts walks an *html.Node tree and collects all text content.
func collectOldTreeTexts(n *html.Node) []string {
	var texts []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.TextNode && n.Data != "" {
			texts = append(texts, n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return texts
}

// compareStructure walks an *html.Node tree and a Store tree, comparing
// element tag names, text content, and nesting depth.
func compareStructure(t *testing.T, oldRoot *html.Node, doc *dom.Document) {
	t.Helper()

	type nodeInfo struct {
		kind  string // "element", "text", "comment", "doctype", "document"
		name  string // tag name for elements, text for text nodes
		depth int
	}

	// Collect from old tree.
	var oldNodes []nodeInfo
	var walkOld func(*html.Node, int)
	walkOld = func(n *html.Node, depth int) {
		if n == nil {
			return
		}
		switch n.Type {
		case html.DocumentNode:
			oldNodes = append(oldNodes, nodeInfo{"document", "", depth})
		case html.ElementNode:
			oldNodes = append(oldNodes, nodeInfo{"element", n.Data, depth})
		case html.TextNode:
			if strings.TrimSpace(n.Data) != "" {
				oldNodes = append(oldNodes, nodeInfo{"text", strings.TrimSpace(n.Data), depth})
			}
		case html.CommentNode:
			oldNodes = append(oldNodes, nodeInfo{"comment", n.Data, depth})
		case html.DoctypeNode:
			oldNodes = append(oldNodes, nodeInfo{"doctype", n.Data, depth})
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkOld(c, depth+1)
		}
	}
	walkOld(oldRoot, 0)

	// Collect from new store.
	var newNodes []nodeInfo
	var walkNew func(dom.NodeID, int)
	walkNew = func(id dom.NodeID, depth int) {
		if id == dom.NodeNone {
			return
		}
		switch doc.Store.Kind(id) {
		case dom.NodeKindDocument:
			newNodes = append(newNodes, nodeInfo{"document", "", depth})
		case dom.NodeKindElement:
			newNodes = append(newNodes, nodeInfo{"element", doc.Store.Name(id).String(), depth})
		case dom.NodeKindText:
			txt, _ := doc.Store.Text(id)
			if strings.TrimSpace(txt) != "" {
				newNodes = append(newNodes, nodeInfo{"text", strings.TrimSpace(txt), depth})
			}
		case dom.NodeKindComment:
			txt, _ := doc.Store.Text(id)
			newNodes = append(newNodes, nodeInfo{"comment", txt, depth})
		case dom.NodeKindDoctype:
			newNodes = append(newNodes, nodeInfo{"doctype", "", depth})
		}
		for c := doc.Store.FirstChild(id); c != dom.NodeNone; c = doc.Store.NextSibling(c) {
			walkNew(c, depth+1)
		}
	}
	walkNew(doc.Root, 0)

	// Compare element sequences (tag names in order).
	var oldElements, newElements []string
	for _, n := range oldNodes {
		if n.kind == "element" {
			oldElements = append(oldElements, n.name)
		}
	}
	for _, n := range newNodes {
		if n.kind == "element" {
			newElements = append(newElements, n.name)
		}
	}

	assert.Equal(t, oldElements, newElements,
		"element tag sequence should match between old and new parser")

	// Compare text content sequences.
	var oldTexts, newTexts []string
	for _, n := range oldNodes {
		if n.kind == "text" {
			oldTexts = append(oldTexts, n.name)
		}
	}
	for _, n := range newNodes {
		if n.kind == "text" {
			newTexts = append(newTexts, n.name)
		}
	}

	assert.Equal(t, oldTexts, newTexts,
		"text content sequence should match between old and new parser")
}

// ---------------------------------------------------------------------------
// 1. TestStreamParseSimpleHTML
// ---------------------------------------------------------------------------

func TestStreamParseSimpleHTML(t *testing.T) {
	const input = `<html><head><title>Test</title></head><body><p>Hello</p></body></html>`

	parser := dom.NewParser()

	t.Run("old API still works", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)
		require.NotNil(t, oldDoc)

		// Verify body exists in old tree.
		var foundBody bool
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "body" {
				foundBody = true
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(oldDoc)
		assert.True(t, foundBody, "old API should find body element")
	})

	t.Run("new streaming API", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)
		require.NotNil(t, doc, "Document must be non-nil")
		require.NotNil(t, doc.Store, "Document.Store must be non-nil")

		// Root is a valid document node.
		assert.NotEqual(t, dom.NodeNone, doc.Root, "Root must be a valid NodeID")
		assert.Equal(t, dom.NodeKindDocument, doc.Store.Kind(doc.Root),
			"Root must be NodeKindDocument")

		// Document has children (html element).
		htmlID := doc.Store.FirstChild(doc.Root)
		assert.NotEqual(t, dom.NodeNone, htmlID, "document should have html child")

		// Walk structure: document → html → head + body.
		headID := findElement(t, doc, atom.AtomHead)
		bodyID := findElement(t, doc, atom.AtomBody)
		assert.NotEqual(t, dom.NodeNone, headID, "should have head element")
		assert.NotEqual(t, dom.NodeNone, bodyID, "should have body element")

		// Verify text "Hello" under body → p.
		pID := findElement(t, doc, atom.AtomP)
		assert.NotEqual(t, dom.NodeNone, pID, "should have p element")
		texts := collectTexts(t, doc.Store, bodyID)
		assert.Contains(t, texts, "Hello", "body should contain text 'Hello'")
	})

	t.Run("old and new produce consistent results", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)

		newDoc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)

		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 2. TestStreamParseTextNodes
// ---------------------------------------------------------------------------

func TestStreamParseTextNodes(t *testing.T) {
	const input = `<p>Hello <b>world</b></p>`

	parser := dom.NewParser()

	t.Run("old API text extraction", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)
		texts := collectOldTreeTexts(oldDoc)
		assert.Contains(t, texts, "Hello ", "old API should have 'Hello ' text")
		assert.Contains(t, texts, "world", "old API should have 'world' text")
	})

	t.Run("new streaming API text extraction", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)
		require.NotNil(t, doc)

		allTexts := collectTexts(t, doc.Store, doc.Root)
		assert.Contains(t, allTexts, "Hello ", "should contain text 'Hello '")
		assert.Contains(t, allTexts, "world", "should contain text 'world' under b")
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 3. TestStreamParseAttributes
// ---------------------------------------------------------------------------

func TestStreamParseAttributes(t *testing.T) {
	const input = `<div id="main" class="container"><a href="/link">click</a></div>`

	parser := dom.NewParser()

	t.Run("old API attributes", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)

		var foundDiv, foundA bool
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				if n.Data == "div" {
					for _, a := range n.Attr {
						if a.Key == "id" && a.Val == "main" {
							foundDiv = true
						}
					}
				}
				if n.Data == "a" {
					for _, a := range n.Attr {
						if a.Key == "href" && a.Val == "/link" {
							foundA = true
						}
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(oldDoc)
		assert.True(t, foundDiv, "old API: div should have id=main")
		assert.True(t, foundA, "old API: a should have href=/link")
	})

	t.Run("new streaming API attributes", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)

		divID := findElement(t, doc, atom.AtomDiv)
		require.NotEqual(t, dom.NodeNone, divID)
		attrs, err := doc.Store.Attrs(divID)
		require.NoError(t, err)

		var hasID, hasClass bool
		for _, a := range attrs {
			if a.Name == atom.AttrId && a.Value.String() == "main" {
				hasID = true
			}
			if a.Name == atom.AttrClass && a.Value.String() == "container" {
				hasClass = true
			}
		}
		assert.True(t, hasID, "div should have id=main")
		assert.True(t, hasClass, "div should have class=container")

		aID := findElement(t, doc, atom.AtomA)
		require.NotEqual(t, dom.NodeNone, aID)
		aAttrs, err := doc.Store.Attrs(aID)
		require.NoError(t, err)

		var hasHref bool
		for _, a := range aAttrs {
			if a.Name == atom.AttrHref && a.Value.String() == "/link" {
				hasHref = true
			}
		}
		assert.True(t, hasHref, "a should have href=/link")
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 4. TestStreamParseVoidElements
// ---------------------------------------------------------------------------

func TestStreamParseVoidElements(t *testing.T) {
	const input = `<p>before<br>after<img src="x.png"><input type="text"></p>`

	parser := dom.NewParser()

	t.Run("old API void elements", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)

		var brCount, imgCount, inputCount int
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				switch n.Data {
				case "br":
					brCount++
				case "img":
					imgCount++
				case "input":
					inputCount++
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(oldDoc)
		assert.Equal(t, 1, brCount, "old API should have 1 br")
		assert.Equal(t, 1, imgCount, "old API should have 1 img")
		assert.Equal(t, 1, inputCount, "old API should have 1 input")
	})

	t.Run("new streaming API void elements", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)

		brs := findAllElements(t, doc, atom.AtomBr)
		imgs := findAllElements(t, doc, atom.AtomImg)
		inputs := findAllElements(t, doc, atom.AtomInput)

		assert.Len(t, brs, 1, "should have 1 br")
		assert.Len(t, imgs, 1, "should have 1 img")
		assert.Len(t, inputs, 1, "should have 1 input")

		// Void elements have no children.
		for _, id := range brs {
			assert.Equal(t, dom.NodeNone, doc.Store.FirstChild(id), "br should have no children")
		}
		for _, id := range imgs {
			assert.Equal(t, dom.NodeNone, doc.Store.FirstChild(id), "img should have no children")
			// Verify img has src attribute.
			attrs, err := doc.Store.Attrs(id)
			require.NoError(t, err)
			var hasSrc bool
			for _, a := range attrs {
				if a.Name == atom.AttrSrc && a.Value.String() == "x.png" {
					hasSrc = true
				}
			}
			assert.True(t, hasSrc, "img should have src=x.png")
		}
		for _, id := range inputs {
			assert.Equal(t, dom.NodeNone, doc.Store.FirstChild(id), "input should have no children")
		}
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 5. TestStreamParseNestedElements
// ---------------------------------------------------------------------------

func TestStreamParseNestedElements(t *testing.T) {
	const input = `<div><div><div><span>deep</span></div></div></div>`

	parser := dom.NewParser()

	t.Run("old API nesting", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)

		// Find the span and walk up to verify depth.
		var spanDepth int
		var walkOld func(*html.Node, int)
		walkOld = func(n *html.Node, depth int) {
			if n.Type == html.ElementNode && n.Data == "span" {
				spanDepth = depth
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walkOld(c, depth+1)
			}
		}
		walkOld(oldDoc, 0)
		assert.Greater(t, spanDepth, 2, "span should be deeply nested in old tree")
	})

	t.Run("new streaming API nesting", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)

		spanID := findElement(t, doc, atom.AtomSpan)
		require.NotEqual(t, dom.NodeNone, spanID)

		// Walk up parents to verify depth.
		depth := 0
		for p := doc.Store.Parent(spanID); p != dom.NodeNone; p = doc.Store.Parent(p) {
			depth++
		}
		assert.Greater(t, depth, 2, "span should be deeply nested (depth > 2)")

		// Verify parent chain: span → div → div → div → ...
		parent := doc.Store.Parent(spanID)
		for i := 0; i < 3; i++ {
			assert.NotEqual(t, dom.NodeNone, parent, "should have at least 3 div ancestors")
			assert.Equal(t, atom.AtomDiv, doc.Store.Name(parent),
				"ancestor should be div")
			parent = doc.Store.Parent(parent)
		}
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 6. TestStreamParseMalformedHTML
// ---------------------------------------------------------------------------

func TestStreamParseMalformedHTML(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"unclosed div with p", `<div><p>text</div>`},
		{"double p tags", `<p>first<p>second`},
		{"missing closing tags", `<html><body><p>text`},
		{"mismatched tags", `<div><span>text</div></span>`},
	}

	parser := dom.NewParser()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Old API: should not error.
			oldDoc, err := parser.ParseDocument(strings.NewReader(tc.input))
			require.NoError(t, err, "old API should not error on malformed HTML")
			require.NotNil(t, oldDoc)

			// New API: should not error.
			newDoc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(tc.input), dom.ParseConfig{})
			require.NoError(t, err, "new API should not error on malformed HTML")
			require.NotNil(t, newDoc)

			// Both produce valid document roots.
			assert.Equal(t, dom.NodeKindDocument, newDoc.Store.Kind(newDoc.Root))

			// Consistency: both produce equivalent structure.
			compareStructure(t, oldDoc, newDoc)
		})
	}
}

// ---------------------------------------------------------------------------
// 7. TestStreamParseEmptyReader
// ---------------------------------------------------------------------------

func TestStreamParseEmptyReader(t *testing.T) {
	parser := dom.NewParser()

	t.Run("old API empty", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(""))
		require.NoError(t, err)
		require.NotNil(t, oldDoc)
	})

	t.Run("new streaming API empty", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(""), dom.ParseConfig{})
		require.NoError(t, err)
		require.NotNil(t, doc)
		assert.Equal(t, dom.NodeKindDocument, doc.Store.Kind(doc.Root),
			"root should be NodeKindDocument even for empty input")

		// Minimal node count: at least the document root.
		assert.GreaterOrEqual(t, doc.Store.NodeCount(), 1,
			"should have at least 1 node (document root)")
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(""))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(""), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 8. TestStreamParseCancellation
// ---------------------------------------------------------------------------

func TestStreamParseCancellation(t *testing.T) {
	parser := dom.NewParser()
	const input = `<html><body><p>Hello</p></body></html>`

	t.Run("pre-cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		doc, err := parser.ParseDocumentCtx(ctx, strings.NewReader(input), dom.ParseConfig{})
		// Should return an error related to cancellation.
		assert.Error(t, err, "should return error for cancelled context")
		assert.Nil(t, doc, "should return nil document on cancellation")
	})

	t.Run("slow reader with cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// A reader that yields one byte at a time and checks cancellation.
		data := []byte(input)
		idx := 0
		slowReader := readerFunc(func(p []byte) (int, error) {
			if idx >= len(data) {
				return 0, io.EOF
			}
			// After reading some bytes, cancel.
			if idx > 10 {
				cancel()
			}
			p[0] = data[idx]
			idx++
			return 1, nil
		})

		doc, err := parser.ParseDocumentCtx(ctx, slowReader, dom.ParseConfig{})
		// Should return context error.
		assert.Error(t, err, "should return error when context cancelled mid-parse")
		assert.Nil(t, doc)
	})

	t.Run("old API unaffected by context tests", func(t *testing.T) {
		// Verify old API still works fine regardless.
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)
		require.NotNil(t, oldDoc)
	})
}

// readerFunc adapts a function to io.Reader.
type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// ---------------------------------------------------------------------------
// 9. TestStreamParseMaxBuf
// ---------------------------------------------------------------------------

func TestStreamParseMaxBuf(t *testing.T) {
	parser := dom.NewParser()

	// Generate HTML larger than 50 bytes.
	largeInput := `<html><body>` + strings.Repeat("<p>paragraph content</p>", 20) + `</body></html>`

	t.Run("exceeds MaxBuf", func(t *testing.T) {
		cfg := dom.ParseConfig{
			MaxBuf: 50,
		}
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(largeInput), cfg)
		// Should either stop parsing early or return an error.
		if err != nil {
			// Error is acceptable — buffer limit exceeded.
			assert.Nil(t, doc, "should return nil doc when buffer exceeded")
		} else {
			// If no error, the parse should have stopped early (incomplete tree).
			require.NotNil(t, doc)
			assert.Less(t, doc.Store.NodeCount(), 50,
				"should have fewer nodes than a full parse when buffer limited")
		}
	})

	t.Run("old API unaffected", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(largeInput))
		require.NoError(t, err)
		require.NotNil(t, oldDoc, "old API should parse large input without issues")
	})

	t.Run("zero MaxBuf means unlimited", func(t *testing.T) {
		// Build a document well beyond the default 1MB tokenizer buffer.
		var sb strings.Builder
		sb.WriteString("<html><body>")
		for i := 0; i < 5000; i++ {
			sb.WriteString("<p>paragraph content that adds some bytes</p>")
		}
		sb.WriteString("</body></html>")
		html2MB := sb.String()

		cfg := dom.ParseConfig{MaxBuf: 0} // zero → no limit
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(html2MB), cfg)
		require.NoError(t, err)
		require.NotNil(t, doc)
		// 5000 <p> + <html> + <head> + <body> + text nodes => well over 10 000 nodes.
		assert.Greater(t, doc.Store.NodeCount(), 10000,
			"MaxBuf=0 should impose no buffer limit and parse the full document")
	})

	t.Run("generous MaxBuf succeeds", func(t *testing.T) {
		cfg := dom.ParseConfig{
			MaxBuf: 1 << 20, // 1MB — plenty
		}
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(largeInput), cfg)
		require.NoError(t, err)
		require.NotNil(t, doc)
		assert.Greater(t, doc.Store.NodeCount(), 5,
			"with generous MaxBuf, should parse fully")
	})
}

// ---------------------------------------------------------------------------
// 10. TestStreamParseResourceDiscovery
// ---------------------------------------------------------------------------

func TestStreamParseResourceDiscovery(t *testing.T) {
	const input = `<html><head><link rel="stylesheet" href="style.css"><script src="app.js"></script></head><body><img src="photo.jpg"></body></html>`

	parser := dom.NewParser()

	t.Run("new API discovers resources", func(t *testing.T) {
		var resources []dom.Resource
		cfg := dom.ParseConfig{
			OnResource: func(r dom.Resource) {
				resources = append(resources, r)
			},
		}

		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
		require.NoError(t, err)
		require.NotNil(t, doc)

		// Expect 3 resources: CSS, Script, Image.
		require.Len(t, resources, 3, "should discover 3 resources")

		// Verify resource kinds and URLs.
		var hasCSS, hasScript, hasImage bool
		for _, r := range resources {
			switch r.Kind {
			case dom.ResourceCSS:
				assert.Equal(t, "style.css", r.URL)
				hasCSS = true
			case dom.ResourceScript:
				assert.Equal(t, "app.js", r.URL)
				hasScript = true
			case dom.ResourceImage:
				assert.Equal(t, "photo.jpg", r.URL)
				hasImage = true
			}
		}
		assert.True(t, hasCSS, "should discover CSS resource")
		assert.True(t, hasScript, "should discover Script resource")
		assert.True(t, hasImage, "should discover Image resource")
	})

	t.Run("old API has no resource callback", func(t *testing.T) {
		// Old API doesn't have resource discovery, but should still parse fine.
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)
		require.NotNil(t, oldDoc)
	})
}

// ---------------------------------------------------------------------------
// 11. TestStreamParseSelfClosingTags
// ---------------------------------------------------------------------------

func TestStreamParseSelfClosingTags(t *testing.T) {
	const input = `<div/><span/>`

	parser := dom.NewParser()

	t.Run("old API self-closing", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)
		require.NotNil(t, oldDoc)
	})

	t.Run("new streaming API self-closing", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)
		require.NotNil(t, doc)

		divs := findAllElements(t, doc, atom.AtomDiv)
		spans := findAllElements(t, doc, atom.AtomSpan)

		assert.Len(t, divs, 1, "should have exactly 1 div")
		assert.Len(t, spans, 1, "should have exactly 1 span")

		// HTML5: self-closing slash on non-void elements is ignored.
		// <div/><span/> is parsed as <div><span></span></div>.
		// span should be nested inside div, not a sibling.
		if len(divs) == 1 && len(spans) == 1 {
			spanParent := doc.Store.Parent(spans[0])
			assert.Equal(t, divs[0], spanParent,
				"span should be a child of div (self-closing ignored for non-void)")
		}
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 12. TestStreamParseDoctypeAndComments
// ---------------------------------------------------------------------------

func TestStreamParseDoctypeAndComments(t *testing.T) {
	const input = `<!DOCTYPE html><html><!-- comment --><body></body></html>`

	parser := dom.NewParser()

	t.Run("old API doctype and comments", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)

		var hasDoctype, hasComment bool
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.DoctypeNode {
				hasDoctype = true
			}
			if n.Type == html.CommentNode {
				hasComment = true
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(oldDoc)
		assert.True(t, hasDoctype, "old API should preserve doctype")
		assert.True(t, hasComment, "old API should preserve comment")
	})

	t.Run("new streaming API doctype and comments", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)
		require.NotNil(t, doc)

		// Check for doctype node.
		var hasDoctype, hasComment bool
		for id := dom.NodeID(1); id <= dom.NodeID(doc.Store.NodeCount()); id++ {
			switch doc.Store.Kind(id) {
			case dom.NodeKindDoctype:
				hasDoctype = true
			case dom.NodeKindComment:
				hasComment = true
				txt, _ := doc.Store.Text(id)
				assert.Equal(t, " comment ", txt, "comment text should match")
			}
		}
		assert.True(t, hasDoctype, "should have doctype node")
		assert.True(t, hasComment, "should have comment node")
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 13. TestStreamParseLargeDocument
// ---------------------------------------------------------------------------

func TestStreamParseLargeDocument(t *testing.T) {
	// Generate HTML with 1000+ <p> elements.
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&sb, "<p>Paragraph %d</p>", i)
	}
	sb.WriteString("</body></html>")
	input := sb.String()

	parser := dom.NewParser()

	t.Run("old API large doc", func(t *testing.T) {
		oldDoc, err := parser.ParseDocument(strings.NewReader(input))
		require.NoError(t, err)
		require.NotNil(t, oldDoc)
	})

	t.Run("new streaming API large doc", func(t *testing.T) {
		doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		require.NoError(t, err)
		require.NotNil(t, doc)

		// Verify reasonable node count: 1000 p elements + 1000 text nodes + structural nodes.
		assert.GreaterOrEqual(t, doc.Store.NodeCount(), 2000,
			"should have at least 2000 nodes (1000 p + 1000 text + overhead)")

		pNodes := findAllElements(t, doc, atom.AtomP)
		assert.Len(t, pNodes, 1000, "should have exactly 1000 p elements")
	})

	t.Run("consistency", func(t *testing.T) {
		oldDoc, _ := parser.ParseDocument(strings.NewReader(input))
		newDoc, _ := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		compareStructure(t, oldDoc, newDoc)
	})
}

// ---------------------------------------------------------------------------
// 14. TestStreamParseStoreAppendAttrs
// ---------------------------------------------------------------------------

func TestStreamParseStoreAppendAttrs(t *testing.T) {
	s := dom.NewStore(16)

	// Allocate two element nodes.
	div, err := s.Allocate()
	require.NoError(t, err)
	require.NoError(t, s.SetKind(div, dom.NodeKindElement))
	require.NoError(t, s.SetName(div, atom.AtomDiv))

	span, err := s.Allocate()
	require.NoError(t, err)
	require.NoError(t, s.SetKind(span, dom.NodeKindElement))
	require.NoError(t, s.SetName(span, atom.AtomSpan))

	// AppendAttrs for div: 2 attributes.
	divAttrs := []dom.Attr{
		{Name: atom.AttrId, Value: atom.Intern("main")},
		{Name: atom.AttrClass, Value: atom.Intern("container")},
	}
	require.NoError(t, s.AppendAttrs(div, divAttrs))

	// AppendAttrs for span: 1 attribute.
	spanAttrs := []dom.Attr{
		{Name: atom.AttrHref, Value: atom.Intern("/link")},
	}
	require.NoError(t, s.AppendAttrs(span, spanAttrs))

	// Verify div attributes.
	gotDiv, err := s.Attrs(div)
	require.NoError(t, err)
	require.Len(t, gotDiv, 2)
	assert.Equal(t, atom.AttrId, gotDiv[0].Name)
	assert.Equal(t, "main", gotDiv[0].Value.String())
	assert.Equal(t, atom.AttrClass, gotDiv[1].Name)
	assert.Equal(t, "container", gotDiv[1].Value.String())

	// Verify span attributes.
	gotSpan, err := s.Attrs(span)
	require.NoError(t, err)
	require.Len(t, gotSpan, 1)
	assert.Equal(t, atom.AttrHref, gotSpan[0].Name)
	assert.Equal(t, "/link", gotSpan[0].Value.String())

	// Verify total attr count.
	assert.Equal(t, 3, s.AttrCount(), "total attr count should be 3")

	// Append more attrs to div — should not corrupt span's offsets.
	extraAttrs := []dom.Attr{
		{Name: atom.AttrName, Value: atom.Intern("extra")},
	}
	require.NoError(t, s.AppendAttrs(div, extraAttrs))

	// Re-verify span attributes are still correct after appending to div.
	gotSpan2, err := s.Attrs(span)
	require.NoError(t, err)
	require.Len(t, gotSpan2, 1)
	assert.Equal(t, atom.AttrHref, gotSpan2[0].Name, "span attr should be uncorrupted")

	// Verify div now has 3 attrs.
	gotDiv2, err := s.Attrs(div)
	require.NoError(t, err)
	assert.Len(t, gotDiv2, 3, "div should have 3 attrs after second append")
}

// ---------------------------------------------------------------------------
// 15. TestStreamParseStoreAppendText
// ---------------------------------------------------------------------------

func TestStreamParseStoreAppendText(t *testing.T) {
	s := dom.NewStore(16)

	// Allocate two text nodes.
	t1, err := s.Allocate()
	require.NoError(t, err)
	require.NoError(t, s.SetKind(t1, dom.NodeKindText))

	t2, err := s.Allocate()
	require.NoError(t, err)
	require.NoError(t, s.SetKind(t2, dom.NodeKindText))

	// AppendText for t1.
	require.NoError(t, s.AppendText(t1, "Hello"))

	// AppendText for t2.
	require.NoError(t, s.AppendText(t2, "World"))

	// Verify t1 text.
	got1, err := s.Text(t1)
	require.NoError(t, err)
	assert.Equal(t, "Hello", got1)

	// Verify t2 text.
	got2, err := s.Text(t2)
	require.NoError(t, err)
	assert.Equal(t, "World", got2)

	// Verify total text bytes.
	assert.Equal(t, 10, s.TextBytes(), "total text bytes should be 10")

	// Append more text to t1 — should not corrupt t2's offset.
	require.NoError(t, s.AppendText(t1, " there"))

	// Re-verify t2 is still correct.
	got2Again, err := s.Text(t2)
	require.NoError(t, err)
	assert.Equal(t, "World", got2Again, "t2 text should be uncorrupted after appending to t1")

	// Verify t1 now has appended text.
	got1Again, err := s.Text(t1)
	require.NoError(t, err)
	assert.Equal(t, "Hello there", got1Again, "t1 should have appended text")
}

// ---------------------------------------------------------------------------
// 16. TestStreamParseNilContext
// ---------------------------------------------------------------------------

func TestStreamParseNilContext(t *testing.T) {
	parser := dom.NewParser()
	const input = `<html><body><p>Hello</p></body></html>`

	// Passing nil context should use context.Background() internally.
	doc, err := parser.ParseDocumentCtx(nil, strings.NewReader(input), dom.ParseConfig{})
	require.NoError(t, err, "nil context should not cause error")
	require.NotNil(t, doc, "should return valid document")
	assert.Equal(t, dom.NodeKindDocument, doc.Store.Kind(doc.Root))

	// Verify it parsed correctly.
	pID := findElement(t, doc, atom.AtomP)
	assert.NotEqual(t, dom.NodeNone, pID, "should find p element")
}

// ---------------------------------------------------------------------------
// 17. TestStreamParseNoOnResourceCallback
// ---------------------------------------------------------------------------

func TestStreamParseNoOnResourceCallback(t *testing.T) {
	parser := dom.NewParser()
	const input = `<html><head><link rel="stylesheet" href="style.css"><script src="app.js"></script></head><body><img src="photo.jpg"></body></html>`

	// No OnResource set in config — should not panic.
	cfg := dom.ParseConfig{}
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err, "should not panic or error without OnResource callback")
	require.NotNil(t, doc)

	// Verify the parse still produced a valid tree.
	assert.Equal(t, dom.NodeKindDocument, doc.Store.Kind(doc.Root))
	assert.Greater(t, doc.Store.NodeCount(), 3, "should have parsed some nodes")
}

// ---------------------------------------------------------------------------
// 18. TestStreamParseCompareWithParseDocument
// ---------------------------------------------------------------------------

func TestStreamParseCompareWithParseDocument(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			"simple page",
			`<html><head><title>Test</title></head><body><p>Hello</p></body></html>`,
		},
		{
			"nested divs",
			`<div><div><div><span>deep</span></div></div></div>`,
		},
		{
			"attributes",
			`<div id="main" class="container"><a href="/link">click</a></div>`,
		},
		{
			"void elements",
			`<p>before<br>after<img src="x.png"></p>`,
		},
		{
			"mixed content",
			`<html><body><h1>Title</h1><p>Para <b>bold</b> text</p><ul><li>one</li><li>two</li></ul></body></html>`,
		},
		{
			"malformed",
			`<p>unclosed<div>nested`,
		},
	}

	parser := dom.NewParser()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse with old API.
			oldDoc, err := parser.ParseDocument(strings.NewReader(tc.input))
			require.NoError(t, err)
			require.NotNil(t, oldDoc)

			// Parse with new streaming API.
			newDoc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(tc.input), dom.ParseConfig{})
			require.NoError(t, err)
			require.NotNil(t, newDoc)

			// Compare element and text sequences.
			compareStructure(t, oldDoc, newDoc)
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkStreamParseDocumentCtx(b *testing.B) {
	parser := dom.NewParser()
	input := strings.Repeat("<p>Paragraph content for benchmark testing.</p>", 100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDocumentVsStream(b *testing.B) {
	parser := dom.NewParser()
	input := strings.Repeat("<p>Benchmark paragraph content.</p>", 100)

	b.Run("old_ParseDocument", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := parser.ParseDocument(strings.NewReader(input))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("new_ParseDocumentCtx", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

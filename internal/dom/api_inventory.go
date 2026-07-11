package dom

// M2.1 API Inventory — *html.Node Pointer Dependencies
//
// This file documents every production API that depends directly on
// *html.Node pointers from golang.org/x/net/html.  These APIs must be
// migrated to the compact NodeID-based DOM store (M2.3) or adapted through
// the compatibility adapter (M2.5).
//
// == internal/dom ==
//
//   Parser.ParseDocument(io.Reader) (*html.Node, error)
//     Entry point.  Returns the root *html.Node from html.Parse.
//     Migration: return a Document handle backed by the compact store.
//
//   Parser.ParseBodyText(string) (string, error)
//   Parser.ParseBodyHTML(string) (string, error)
//     Internal html.Parse + recursive walk on *html.Node.
//     Migration: walk compact store by NodeID.
//
//   Parser.GetElementByID(string, string) (string, error)
//   Parser.GetElementByIDFull(string, string) (*Element, error)
//   Parser.GetElementsByClassName(string, string) ([]*Element, error)
//   Parser.GetElementsByTagName(string, string) ([]*Element, error)
//   Parser.QuerySelector(string, string) (*Element, error)
//   Parser.QuerySelectorAll(string, string) ([]*Element, error)
//     Each re-parses HTML and walks *html.Node tree.  Element.Node field
//     holds a direct *html.Node pointer.
//     Migration: query compact store; Element.Node becomes optional via adapter.
//
//   Parser.getTextFromNode(*html.Node, *strings.Builder)
//   Parser.nodeToElement(*html.Node) *Element
//   Parser.matchesSelector(*html.Node, string) bool
//   Parser.convertToMarkdown(*html.Node, *strings.Builder)
//     Internal helpers operating on *html.Node subtrees.
//     Migration: operate on NodeID + attribute slice.
//
//   Element.Node *html.Node
//     Public field that exposes the underlying pointer.
//     Migration: remove or gate behind adapter (M2.5).
//
// == internal/renderer ==
//
//   Renderer.RenderHTML(ctx, string) (fyne.CanvasObject, error)
//     Uses html.ParseFragment to build a *html.Node tree, then constructs
//     the render tree from it.
//
//   findBodyNode(*html.Node) *html.Node
//     Recursive walk to locate the <body> element.
//
//   Renderer.loadExternalCSS(ctx, *html.Node)
//   extractExternalLinks(*html.Node) []string
//   extractAndParseCSS(*html.Node) *css.StyleSheet
//     Walk *html.Node to find <link> and <style> elements.
//
//   countHTMLNodes(*html.Node) int
//     Diagnostic node counter for metrics recording.
//
// == internal/js ==
//
//   Runtime.populateJSNode(*html.Node, *goja.Object)
//     Binds a Go *html.Node to a JavaScript object for DOM API access.
//
//   Runtime.convertGoNodeToJS(*html.Node) goja.Value
//     Wraps a *html.Node in a Goja proxy for JavaScript interop.
//
//   html.ParseFragment usage in innerHTML setter
//     Creates *html.Node fragments for DOM manipulation from JavaScript.
//
//   Runtime DOM tree initialization (findNodes)
//     Walks *html.Node to locate <html>, <head>, <body> for the JS runtime.
//
// == cmd/browser ==
//
//   Title extraction (crawler func)
//     Walks *html.Node to find <title> text for tab labels.
//
//   Script extraction (extractExternalScriptSrcs)
//     Walks *html.Node to discover <script src="..."> tags.
//
//   Render tree walk
//     Recursive *html.Node walk during initial page render.
//
// == Migration plan ==
//
//   Phase 1 (M2.3): Build compact store alongside *html.Node adapter. [DONE]
//   Phase 2 (M2.4): Streaming tokenizer writes directly to compact store. [DONE]
//   Phase 3 (M2.5): Adapter exposes *html.Node for unmigrated consumers. [DONE]
//   Phase 4 (M5.4): Remove adapter; all consumers use NodeID. [DONE]

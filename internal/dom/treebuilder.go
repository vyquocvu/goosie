package dom

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom/atom"
	"golang.org/x/net/html"
)

// Document is the result of a streaming parse.
type Document struct {
	Store *Store
	Root  NodeID
}

// ResourceKind identifies the type of discovered resource.
type ResourceKind uint8

const (
	ResourceCSS ResourceKind = iota + 1
	ResourceScript
	ResourceImage
)

// ScriptMode identifies how a discovered <script> should be executed.
// The streaming parser reports the mode it observed in the markup;
// downstream code (e.g. internal/engine/documentloader) decides what
// to do with each mode. The zero value is ScriptModeClassic so callers
// can omit the field for parser-blocking scripts.
type ScriptMode uint8

const (
	// ScriptModeClassic is a parser-blocking classic script.
	ScriptModeClassic ScriptMode = iota
	// ScriptModeAsync executes when ready without preserving order.
	ScriptModeAsync
	// ScriptModeDefer executes in document order after parsing.
	ScriptModeDefer
	// ScriptModeModule is an ES module. Out of scope for M2.
	ScriptModeModule
)

// String returns a stable label for the script mode.
func (m ScriptMode) String() string {
	switch m {
	case ScriptModeClassic:
		return "classic"
	case ScriptModeAsync:
		return "async"
	case ScriptModeDefer:
		return "defer"
	case ScriptModeModule:
		return "module"
	default:
		return "unknown"
	}
}

// Resource is a discovered external resource during parsing.
//
// The shape intentionally carries more fields than M0/M1 of the engine
// pipeline consumed; M2 of plan.md wires these fields into the
// ResourceCoordinator via the internal/engine/documentloader bridge.
//
// Source is currently empty for inline scripts. The streaming parser
// reports the existence of an inline script (Inline=true, Source=nil);
// downstream code may re-traverse the parsed document to extract the
// script body. Capturing inline script bodies during the streaming
// tokenizer pass is deferred to a later milestone.
type Resource struct {
	Kind        ResourceKind
	URL         string
	Position    int        // document-order index assigned by the parser
	ScriptMode  ScriptMode // meaningful only when Kind == ResourceScript
	Inline      bool       // true for <script>...</script> with no src
	Integrity   string     // SRI hash, empty when absent
	CrossOrigin string     // crossorigin attribute, empty when absent
}

// UnsupportedFeatureKind identifies the type of unsupported engine feature.
type UnsupportedFeatureKind uint8

const (
	FeatureCanvas UnsupportedFeatureKind = iota + 1
	FeatureVideo
	FeatureAudio
	FeatureIframe
	FeatureESModule
	FeatureObject
	FeatureEmbed
	FeaturePWAManifest
	FeatureWebSocket
	FeatureWebWorker
	FeatureServiceWorker
)

func (k UnsupportedFeatureKind) String() string {
	switch k {
	case FeatureCanvas:
		return "canvas"
	case FeatureVideo:
		return "video"
	case FeatureAudio:
		return "audio"
	case FeatureIframe:
		return "iframe"
	case FeatureESModule:
		return "es-module"
	case FeatureObject:
		return "object"
	case FeatureEmbed:
		return "embed"
	case FeaturePWAManifest:
		return "pwa-manifest"
	case FeatureWebSocket:
		return "websocket"
	case FeatureWebWorker:
		return "web-worker"
	case FeatureServiceWorker:
		return "service-worker"
	default:
		return fmt.Sprintf("UnsupportedFeatureKind(%d)", k)
	}
}

// UnsupportedFeature describes a detected feature the engine does not support.
type UnsupportedFeature struct {
	Kind UnsupportedFeatureKind
}

// ParseConfig controls streaming parse behavior.
type ParseConfig struct {
	MaxBuf               int
	OnResource           func(Resource)
	OnUnsupportedFeature func(UnsupportedFeature)
}

// voidElements must not be pushed onto the open stack.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true,
	"embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

// blocksClosableP lists block-level elements that auto-close an open <p>.
var blocksClosableP = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"details": true, "div": true, "dl": true, "fieldset": true,
	"figcaption": true, "figure": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hgroup": true, "hr": true, "main": true, "menu": true,
	"nav": true, "ol": true, "p": true, "pre": true, "section": true,
	"table": true, "ul": true,
}

// headRelatedTags are elements that belong inside <head>.
var headRelatedTags = map[string]bool{
	"title": true, "base": true, "link": true, "meta": true,
	"style": true, "script": true, "noscript": true, "template": true,
}

// tagLookup and attrLookup are separate static lookup tables that avoid
// collisions between tag names and attribute names sharing the same string
// (e.g. "span" is both AtomSpan=90 and AttrSpan=244).
var (
	tagLookup  map[string]atom.Atom
	attrLookup map[string]atom.Atom
)

func init() {
	tagLookup = map[string]atom.Atom{
		"a": atom.AtomA, "abbr": atom.AtomAbbr, "address": atom.AtomAddress,
		"area": atom.AtomArea, "article": atom.AtomArticle, "aside": atom.AtomAside,
		"audio": atom.AtomAudio, "b": atom.AtomB, "base": atom.AtomBase,
		"bdi": atom.AtomBdi, "bdo": atom.AtomBdo, "blockquote": atom.AtomBlockquote,
		"body": atom.AtomBody, "br": atom.AtomBr, "button": atom.AtomButton,
		"canvas": atom.AtomCanvas, "caption": atom.AtomCaption, "cite": atom.AtomCite,
		"code": atom.AtomCode, "col": atom.AtomCol, "colgroup": atom.AtomColgroup,
		"data": atom.AtomData, "datalist": atom.AtomDatalist, "dd": atom.AtomDd,
		"del": atom.AtomDel, "details": atom.AtomDetails, "dfn": atom.AtomDfn,
		"dialog": atom.AtomDialog, "div": atom.AtomDiv, "dl": atom.AtomDl,
		"dt": atom.AtomDt, "em": atom.AtomEm, "embed": atom.AtomEmbed,
		"fieldset": atom.AtomFieldset, "figcaption": atom.AtomFigcaption,
		"figure": atom.AtomFigure, "footer": atom.AtomFooter, "form": atom.AtomForm,
		"h1": atom.AtomH1, "h2": atom.AtomH2, "h3": atom.AtomH3,
		"h4": atom.AtomH4, "h5": atom.AtomH5, "h6": atom.AtomH6,
		"head": atom.AtomHead, "header": atom.AtomHeader, "hgroup": atom.AtomHgroup,
		"hr": atom.AtomHr, "html": atom.AtomHtml, "i": atom.AtomI,
		"iframe": atom.AtomIframe, "img": atom.AtomImg, "input": atom.AtomInput,
		"ins": atom.AtomIns, "kbd": atom.AtomKbd, "label": atom.AtomLabel,
		"legend": atom.AtomLegend, "li": atom.AtomLi, "link": atom.AtomLink,
		"main": atom.AtomMain, "map": atom.AtomMap, "mark": atom.AtomMark,
		"menu": atom.AtomMenu, "meta": atom.AtomMeta, "meter": atom.AtomMeter,
		"nav": atom.AtomNav, "noscript": atom.AtomNoscript, "object": atom.AtomObject,
		"ol": atom.AtomOl, "optgroup": atom.AtomOptgroup, "option": atom.AtomOption,
		"output": atom.AtomOutput, "p": atom.AtomP, "param": atom.AtomParam,
		"picture": atom.AtomPicture, "pre": atom.AtomPre, "progress": atom.AtomProgress,
		"q": atom.AtomQ, "rp": atom.AtomRp, "rt": atom.AtomRt,
		"ruby": atom.AtomRuby, "s": atom.AtomS, "samp": atom.AtomSamp,
		"script": atom.AtomScript, "section": atom.AtomSection, "select": atom.AtomSelect,
		"slot": atom.AtomSlot, "small": atom.AtomSmall, "source": atom.AtomSource,
		"span": atom.AtomSpan, "strong": atom.AtomStrong, "style": atom.AtomStyle,
		"sub": atom.AtomSub, "summary": atom.AtomSummary, "sup": atom.AtomSup,
		"table": atom.AtomTable, "tbody": atom.AtomTbody, "td": atom.AtomTd,
		"template": atom.AtomTemplate, "textarea": atom.AtomTextarea,
		"tfoot": atom.AtomTfoot, "th": atom.AtomTh, "thead": atom.AtomThead,
		"time": atom.AtomTime, "title": atom.AtomTitle, "tr": atom.AtomTr,
		"track": atom.AtomTrack, "u": atom.AtomU, "ul": atom.AtomUl,
		"var": atom.AtomVar, "video": atom.AtomVideo, "wbr": atom.AtomWbr,
	}
	attrLookup = map[string]atom.Atom{
		"id": atom.AttrId, "class": atom.AttrClass, "href": atom.AttrHref,
		"src": atom.AttrSrc, "alt": atom.AttrAlt, "style": atom.AttrStyle,
		"type": atom.AttrType, "name": atom.AttrName, "value": atom.AttrValue,
		"action": atom.AttrAction, "method": atom.AttrMethod, "rel": atom.AttrRel,
		"charset": atom.AttrCharset, "content": atom.AttrContent, "lang": atom.AttrLang,
		"width": atom.AttrWidth, "height": atom.AttrHeight, "title": atom.AttrTitle,
		"placeholder": atom.AttrPlaceholder, "disabled": atom.AttrDisabled,
		"checked": atom.AttrChecked, "role": atom.AttrRole,
		"aria-label": atom.AttrAriaLabel, "for": atom.AttrFor,
		"tabindex": atom.AttrTabindex, "target": atom.AttrTarget,
		"data-": atom.AttrDataPrefix, "srcset": atom.AttrSrcset,
		"sizes": atom.AttrSizes, "media": atom.AttrMedia,
		"datetime": atom.AttrDatetime, "open": atom.AttrOpen,
		"max": atom.AttrMax, "min": atom.AttrMin, "step": atom.AttrStep,
		"pattern": atom.AttrPattern, "required": atom.AttrRequired,
		"readonly": atom.AttrReadonly, "autofocus": atom.AttrAutofocus,
		"autocomplete": atom.AttrAutocomplete, "enctype": atom.AttrEnctype,
		"colspan": atom.AttrColspan, "rowspan": atom.AttrRowspan,
		"scope": atom.AttrScope, "span": atom.AttrSpan, "dir": atom.AttrDir,
		"draggable": atom.AttrDraggable, "hidden": atom.AttrHidden,
	}
}

// treeBuilder tracks state during a streaming parse.
type treeBuilder struct {
	store         *Store
	stack         []NodeID
	rootID        NodeID
	onRes         func(Resource)
	onUnsupported func(UnsupportedFeature)
	htmlSeen      bool
	headSeen      bool
	bodySeen      bool
	headID        NodeID
	bodyID        NodeID
	resPos        int // next document-order index for discovered resources
}

func internTag(name string) atom.Atom {
	if a, ok := tagLookup[name]; ok {
		return a
	}
	return atom.Intern(name)
}

func internAttr(name string) atom.Atom {
	if a, ok := attrLookup[name]; ok {
		return a
	}
	return atom.Intern(name)
}

// ParseDocumentCtx performs a streaming HTML parse with context cancellation
// and resource discovery. Returns nil Document and non-nil error when the
// context is cancelled or the tokenizer hits an unrecoverable error.
func (p *Parser) ParseDocumentCtx(ctx context.Context, r io.Reader, cfg ParseConfig) (*Document, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tokenizer := html.NewTokenizer(r)
	if cfg.MaxBuf > 0 {
		tokenizer.SetMaxBuf(cfg.MaxBuf)
	}

	tb := &treeBuilder{
		store:         NewStore(256),
		onRes:         cfg.OnResource,
		onUnsupported: cfg.OnUnsupportedFeature,
	}

	rootID, err := tb.store.Allocate()
	if err != nil {
		return nil, err
	}
	if err := tb.store.SetKind(rootID, NodeKindDocument); err != nil {
		return nil, err
	}
	tb.rootID = rootID
	tb.stack = []NodeID{rootID}

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		switch tt {
		case html.StartTagToken:
			if err := tb.handleStartTag(tokenizer, false); err != nil {
				return nil, err
			}
		case html.EndTagToken:
			tb.handleEndTag(tokenizer)
		case html.TextToken:
			tok := tokenizer.Token()
			if len(tb.stack) == 1 && strings.TrimSpace(tok.Data) == "" {
				continue
			}
			if err := tb.insertText(tok.Data, NodeKindText); err != nil {
				return nil, err
			}
		case html.CommentToken:
			if err := tb.ensureHtml(); err != nil {
				return nil, err
			}
			tok := tokenizer.Token()
			if err := tb.insertText(tok.Data, NodeKindComment); err != nil {
				return nil, err
			}
		case html.DoctypeToken:
			did, err := tb.store.Allocate()
			if err != nil {
				return nil, err
			}
			if err := tb.store.SetKind(did, NodeKindDoctype); err != nil {
				return nil, err
			}
			// Doctype is always a child of the document, not <html>.
			if err := tb.store.AppendChild(tb.rootID, did); err != nil {
				return nil, err
			}
		case html.SelfClosingTagToken:
			if err := tb.handleStartTag(tokenizer, true); err != nil {
				return nil, err
			}
		}
	}

	if err := tokenizer.Err(); err != nil && err != io.EOF {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Ensure html/head/body always exist (matches html.Parse behavior).
	if err := tb.ensureBody(); err != nil {
		return nil, err
	}

	return &Document{Store: tb.store, Root: tb.rootID}, nil
}

// ---------------------------------------------------------------------------
// Auto-insertion of html / head / body
// ---------------------------------------------------------------------------

// ensureHtml auto-inserts the <html> element if not yet present.
func (tb *treeBuilder) ensureHtml() error {
	if tb.htmlSeen {
		return nil
	}
	id, err := tb.store.Allocate()
	if err != nil {
		return err
	}
	if err := tb.store.SetKind(id, NodeKindElement); err != nil {
		return err
	}
	if err := tb.store.SetName(id, atom.AtomHtml); err != nil {
		return err
	}
	if err := tb.store.AppendChild(tb.rootID, id); err != nil {
		return err
	}
	tb.stack = append(tb.stack, id)
	tb.htmlSeen = true
	return nil
}

// ensureHead auto-inserts the <head> element under <html>.
func (tb *treeBuilder) ensureHead() error {
	if tb.headSeen {
		return nil
	}
	if err := tb.ensureHtml(); err != nil {
		return err
	}
	id, err := tb.store.Allocate()
	if err != nil {
		return err
	}
	if err := tb.store.SetKind(id, NodeKindElement); err != nil {
		return err
	}
	if err := tb.store.SetName(id, atom.AtomHead); err != nil {
		return err
	}
	htmlID := tb.stack[len(tb.stack)-1] // html is top after ensureHtml
	if err := tb.store.AppendChild(htmlID, id); err != nil {
		return err
	}
	tb.headID = id
	tb.headSeen = true
	return nil
}

// ensureBody auto-inserts <head> (if needed) and <body> under <html>.
func (tb *treeBuilder) ensureBody() error {
	if tb.bodySeen {
		return nil
	}
	if !tb.headSeen {
		if err := tb.ensureHead(); err != nil {
			return err
		}
	}
	id, err := tb.store.Allocate()
	if err != nil {
		return err
	}
	if err := tb.store.SetKind(id, NodeKindElement); err != nil {
		return err
	}
	if err := tb.store.SetName(id, atom.AtomBody); err != nil {
		return err
	}
	// Body is a child of html. Find html on the stack.
	htmlID := tb.stack[len(tb.stack)-1]
	if tb.headSeen && tb.store.Name(htmlID) != atom.AtomHtml {
		// Walk up to find html.
		for i := len(tb.stack) - 1; i >= 1; i-- {
			if tb.store.Name(tb.stack[i]) == atom.AtomHtml {
				htmlID = tb.stack[i]
				break
			}
		}
	}
	if err := tb.store.AppendChild(htmlID, id); err != nil {
		return err
	}
	// Reset stack to [root, html] then push body.
	tb.stack = tb.stack[:0]
	tb.stack = append(tb.stack, tb.rootID, htmlID, id)
	tb.bodyID = id
	tb.bodySeen = true
	return nil
}

// autoStructure auto-inserts html/head/body based on the incoming tag.
func (tb *treeBuilder) autoStructure(tagName string) error {
	switch tagName {
	case "html":
		return nil // handled separately
	case "head":
		return tb.ensureHtml()
	case "body":
		return tb.ensureHead()
	default:
		if headRelatedTags[tagName] {
			if !tb.bodySeen {
				return tb.ensureHead()
			}
		} else {
			return tb.ensureBody()
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Token handlers
// ---------------------------------------------------------------------------

func (tb *treeBuilder) handleStartTag(tokenizer *html.Tokenizer, selfClosing bool) error {
	tok := tokenizer.Token()
	tagName := tok.Data

	// HTML5: self-closing slash on non-void elements is a parse error and must be ignored.
	if selfClosing && !voidElements[tagName] {
		selfClosing = false
	}

	// Auto-insert structural elements.
	if tagName == "html" {
		if !tb.htmlSeen {
			if err := tb.ensureHtml(); err != nil {
				return err
			}
		}
		return nil
	}
	if tagName == "head" {
		if !tb.headSeen {
			if err := tb.autoStructure(tagName); err != nil {
				return err
			}
			if err := tb.ensureHead(); err != nil {
				return err
			}
			if !selfClosing {
				tb.stack = append(tb.stack, tb.headID)
			}
		}
		return nil
	}
	if tagName == "body" {
		if !tb.bodySeen {
			if err := tb.autoStructure(tagName); err != nil {
				return err
			}
			if err := tb.ensureBody(); err != nil {
				return err
			}
		}
		return nil
	}

	if err := tb.autoStructure(tagName); err != nil {
		return err
	}

	// HTML5: auto-close <p> when a block-level element appears.
	if blocksClosableP[tagName] {
		autoCloseP(tb.store, &tb.stack)
	}

	elemID, err := tb.store.Allocate()
	if err != nil {
		return err
	}
	if err := tb.store.SetKind(elemID, NodeKindElement); err != nil {
		return err
	}
	if err := tb.store.SetName(elemID, internTag(tagName)); err != nil {
		return err
	}

	if len(tok.Attr) > 0 {
		attrs := make([]Attr, len(tok.Attr))
		for i, a := range tok.Attr {
			attrs[i] = Attr{Name: internAttr(a.Key), Value: atom.Intern(a.Val)}
		}
		if err := tb.store.AppendAttrs(elemID, attrs); err != nil {
			return err
		}
	}

	parent := tb.stack[len(tb.stack)-1]
	if err := tb.store.AppendChild(parent, elemID); err != nil {
		return err
	}

	if !selfClosing && !voidElements[tagName] {
		tb.stack = append(tb.stack, elemID)
	}

	if tb.onRes != nil {
		discoverResources(tagName, tok.Attr, tb.resPos, tb.onRes)
		// Advance position for every start tag so that document order is
		// preserved even when a tag does not yield a discovered resource.
		tb.resPos++
	}
	tb.detectUnsupportedFeatures(tagName, tok.Attr)
	return nil
}

func (tb *treeBuilder) handleEndTag(tokenizer *html.Tokenizer) {
	tok := tokenizer.Token()
	tagAtom := internTag(tok.Data)

	for i := len(tb.stack) - 1; i >= 1; i-- {
		if tb.store.Name(tb.stack[i]) == tagAtom {
			tb.stack = tb.stack[:i]
			return
		}
	}
}

func (tb *treeBuilder) insertText(data string, kind NodeKind) error {
	id, err := tb.store.Allocate()
	if err != nil {
		return err
	}
	if err := tb.store.SetKind(id, kind); err != nil {
		return err
	}
	if err := tb.store.AppendText(id, data); err != nil {
		return err
	}
	parent := tb.stack[len(tb.stack)-1]
	return tb.store.AppendChild(parent, id)
}

func autoCloseP(store *Store, stack *[]NodeID) {
	s := *stack
	for i := len(s) - 1; i >= 1; i-- {
		name := store.Name(s[i])
		if name == atom.AtomP {
			*stack = s[:i]
			return
		}
		// Stop at non-p block elements.
		if name != atom.AtomNone {
			str := name.String()
			if blocksClosableP[str] {
				return
			}
		}
	}
}

// detectUnsupportedFeatures calls onUnsupported when the tag is a known
// engine-unsupported feature. Nil callback is a no-op.
func (tb *treeBuilder) detectUnsupportedFeatures(tagName string, tokAttrs []html.Attribute) {
	if tb.onUnsupported == nil {
		return
	}
	var kind UnsupportedFeatureKind
	switch tagName {
	case "canvas":
		kind = FeatureCanvas
	case "video":
		kind = FeatureVideo
	case "audio":
		kind = FeatureAudio
	case "iframe":
		kind = FeatureIframe
	case "object":
		kind = FeatureObject
	case "embed":
		kind = FeatureEmbed
	case "script":
		for _, a := range tokAttrs {
			if strings.EqualFold(a.Key, "type") && strings.EqualFold(a.Val, "module") {
				kind = FeatureESModule
				break
			}
		}
	case "link":
		for _, a := range tokAttrs {
			if strings.EqualFold(a.Key, "rel") && strings.EqualFold(a.Val, "manifest") {
				kind = FeaturePWAManifest
				break
			}
		}
	}
	if kind != 0 {
		tb.onUnsupported(UnsupportedFeature{Kind: kind})
	}
}

func discoverResources(tagName string, tokAttrs []html.Attribute, position int, onResource func(Resource)) {
	switch tagName {
	case "link":
		var rel, href, integrity, crossorigin string
		for _, a := range tokAttrs {
			switch a.Key {
			case "rel":
				rel = a.Val
			case "href":
				href = a.Val
			case "integrity":
				integrity = a.Val
			case "crossorigin":
				crossorigin = a.Val
			}
		}
		if strings.EqualFold(rel, "stylesheet") && href != "" {
			onResource(Resource{
				Kind:        ResourceCSS,
				URL:         href,
				Position:    position,
				Integrity:   integrity,
				CrossOrigin: crossorigin,
			})
		}
	case "script":
		var src string
		var typeAttr string
		mode := ScriptModeClassic
		integrity := ""
		crossorigin := ""
		hasAsync := false
		hasDefer := false
		hasModule := false
		for _, a := range tokAttrs {
			switch a.Key {
			case "src":
				src = a.Val
			case "type":
				typeAttr = a.Val
				if strings.EqualFold(strings.TrimSpace(a.Val), "module") {
					hasModule = true
				}
			case "async":
				hasAsync = true
			case "defer":
				hasDefer = true
			case "integrity":
				integrity = a.Val
			case "crossorigin":
				crossorigin = a.Val
			}
		}
		if !IsJavaScriptMIMEType(typeAttr) {
			return
		}
		// Mode precedence: type=module > async > defer > classic.
		switch {
		case hasModule:
			mode = ScriptModeModule
		case hasAsync:
			mode = ScriptModeAsync
		case hasDefer:
			mode = ScriptModeDefer
		}
		if src != "" {
			onResource(Resource{
				Kind:        ResourceScript,
				URL:         src,
				Position:    position,
				ScriptMode:  mode,
				Integrity:   integrity,
				CrossOrigin: crossorigin,
			})
			return
		}
		// Inline script: report its existence with Inline=true. The
		// streaming tokenizer does not currently capture the body;
		// downstream code may re-traverse the parsed Document to
		// extract it.
		onResource(Resource{
			Kind:        ResourceScript,
			Position:    position,
			ScriptMode:  mode,
			Inline:      true,
			Integrity:   integrity,
			CrossOrigin: crossorigin,
		})
	case "img":
		var src, crossorigin string
		for _, a := range tokAttrs {
			switch a.Key {
			case "src":
				src = a.Val
			case "crossorigin":
				crossorigin = a.Val
			}
		}
		if src != "" {
			onResource(Resource{
				Kind:        ResourceImage,
				URL:         src,
				Position:    position,
				CrossOrigin: crossorigin,
			})
		}
	}
}

// IsJavaScriptMIMEType returns true if the specified script type attribute represents
// an executable JavaScript or module script according to the HTML specification.
// An empty MIME type defaults to true (classic JavaScript).
func IsJavaScriptMIMEType(typeAttr string) bool {
	typeAttr = strings.TrimSpace(typeAttr)
	if typeAttr == "" {
		return true
	}
	if idx := strings.IndexByte(typeAttr, ';'); idx != -1 {
		typeAttr = strings.TrimSpace(typeAttr[:idx])
	}
	typeAttr = strings.ToLower(typeAttr)
	switch typeAttr {
	case "text/javascript",
		"text/ecmascript",
		"application/javascript",
		"application/ecmascript",
		"text/jscript",
		"text/livescript",
		"javascript",
		"module":
		return true
	default:
		return false
	}
}

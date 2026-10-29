// Package engine wires the front-end (DOM, CSS, style, layout, paint) into the
// v2 frame path.
//
// The engine is the top of the dependency stack: it imports every front-end
// package and produces a paint.LayerDL that the surface can display. A Session
// owns the pipeline for one document; the pipeline is Parse → Style → Layout →
// Paint, and each stage is a pure function of its input so a change to one
// stage does not force the others to be rebuilt.
package engine

import (
	"encoding/base64"
	"errors"
	"fmt"
	stdimage "image"
	"math"
	"mime"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	imgdec "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/internal/userscript"
)

// Session is one document's pipeline state.
//
// A session holds the parsed DOM, the resolved styles, and the layout arena;
// each field is the output of the previous stage. The pipeline is re-run from
// the stage that changed: a style sheet edit re-runs style and below, a DOM
// edit re-runs everything. The session does not own a network client; the
// caller passes in fetched bytes so the engine stays testable without a network.
type Session struct {
	Doc          *dom.Document
	Styles       map[dom.NodeID]*style.ComputedStyle
	PseudoStyles map[style.PseudoKey]*style.ComputedStyle
	Arena        *layout.Arena

	metrics   layout.Metrics
	viewportW float32
	viewportH float32
	linkBase  string
	linker    CSSLinker

	scriptBase       string
	scriptFetch      ScriptFetcher
	contentLanguage  string
	documentEncoding string
	iframeLoader     IFrameDocumentLoader
	iframeFormSubmit IFrameFormSubmitter

	// sheets are the parsed stylesheets kept for RefreshAfterFonts: when
	// deferred fonts land, styles must be re-resolved against the now-populated
	// customFonts table so text picks the web faces instead of system fallbacks.
	sheets []*css.Stylesheet
	// styleResolver retains the compiled selector index for the current sheet
	// set, avoiding a stylesheet-wide index rebuild on each interaction pass.
	styleResolver *style.Resolver

	imgBase  string
	imgFetch ImageFetcher
	natural  map[dom.NodeID]layout.NaturalSize
	images   map[dom.NodeID]stdimage.Image
	// scriptImgs holds the pixels a script successfully assigned to an
	// in-document <img>, keyed by node, and pendingScriptImgs holds this
	// turn's results awaiting reconciliation. scriptImgsLayoutPending records
	// that attached or dropped pixels still owe the arena a rebuild; PumpJS
	// forces exactly one Reflow and then clears it. All three are guarded by
	// imgMu: the pixel sink runs on the JS goroutine while deferred loads and
	// their reflows can move the same tables. See script_image.go.
	scriptImgs              map[dom.NodeID]scriptImagePixels
	pendingScriptImgs       map[dom.NodeID]scriptImagePixels
	scriptImgsLayoutPending bool
	bgImages                map[dom.NodeID]stdimage.Image
	// pseudoBgImages holds background pixels declared on ::before/::after
	// styles. A pseudo box is an anonymous layout object, so the image is
	// keyed by the computed style pointer the layout object carries.
	pseudoBgImages    map[*style.ComputedStyle]stdimage.Image
	pseudoBgVectorRef map[*style.ComputedStyle]string
	// borderImages holds decoded border-image sources by node, attached to
	// the layout object's BorderImage like backgrounds (never natural).
	borderImages map[dom.NodeID]stdimage.Image
	// masks holds decoded mask-image sources by node, attached to the
	// layout object's MaskImage like backgrounds (never natural).
	masks map[dom.NodeID]stdimage.Image
	// bgVectorSrc holds raw bytes for vector backgrounds without intrinsic
	// dimensions or ratio: their raster size is the laid-out tile, known
	// only after layout, so they decode in the Reflow attachment instead of
	// the fetch pool. Keyed by URL; bgVectorRef maps nodes to it.
	bgVectorSrc map[string][]byte
	bgVectorRef map[dom.NodeID]string
	bgNoRatio   map[string]bool
	// imgReservation budgets decoded pixels across fetch and lazy vector
	// rasterization alike; bgVectorUsed tracks the lazy share so each
	// Reflow releases the previous tiles before reserving the new ones.
	imgReservation *imageReservation
	bgVectorUsed   int64
	// scriptImgAttempts and scriptImgDeadline bound the script-created image
	// path (`new Image()`/HTMLImageElement.src). They count attempts, not
	// successes, against the same maxLoadedImages ceiling and ImageFetchBudget
	// window the parse-time pool uses, so an unbounded `new Image()` loop in a
	// script cannot outspend a page full of <img> tags. Guarded by imgMu.
	scriptImgAttempts int
	scriptImgDeadline time.Time
	// sub budgets raw subresource bytes across linked sheets, @imports,
	// fonts and images alike: per-class caps bound each family, but only a
	// shared total stops a page from spending the maximum on every family
	// at once.
	sub *subBudget

	// imgMu guards the image maps and pendingImgs. LoadDeferredImages runs on
	// the caller's goroutine and writes these maps while the session owner
	// keeps reflowing, so every read and write crosses here.
	imgMu sync.Mutex

	// Progressive paint state. WithDeferredImages collects the document's
	// image references without fetching them, so the first frame goes out
	// after document, CSS and fonts only; the refs wait here until
	// LoadDeferredImages drains them.
	deferImages bool
	pendingImgs []imgRef

	fontBase  string
	fontFetch FontFetcher
	fontReg   FontRegistry

	// Deferred font loading mirrors deferred images: the first frame renders
	// with system fallback fonts while @font-face files download in the
	// background; LoadDeferredFonts re-resolves styles and reflows when they
	// arrive so text swaps to the web faces without holding up first paint.
	deferFonts   bool
	pendingFonts []fontJob

	// customFonts is this document's @font-face table: the families it declared
	// and the registry indices that document's rasterizer handed back. The
	// indices only mean something inside s.fontReg, so sharing one process-wide
	// table made a second document's faces overwrite the first one's.
	customFonts *style.CustomFonts

	// Focus state for form editing. focus survives Reflow because reflow
	// rebuilds only the layout arena, not the DOM; a new session per
	// navigation starts unfocused. marked tracks the active IME text; input and
	// textarea values and contenteditable Text nodes include it during
	// composition when their editing caret is representable. focusValue records
	// the value when a control gained focus; focusUserChanged records whether a
	// user edit happened during that focus period, independently of script writes.
	focus                  *dom.Node
	focusState             atomic.Pointer[dom.Node]
	hovered                *dom.Node
	active                 *dom.Node
	keyboardActive         *dom.Node
	keyboardActiveMarked   []*dom.Node
	caret                  int
	controlSelectionAnchor int
	controlSelectionActive bool
	marked                 string
	markedNode             dom.NodeID
	markedHasNode          bool
	markedOffset           int
	imeComposing           bool
	focusValue             string
	focusUserChanged       bool
	focusStylePending      bool
	submittedForm          *FormSubmission
	rangePointerNode       *dom.Node
	rangePointerMoved      bool

	// Text selection state. selActive and the two ends index the current
	// arena's word list, so Reflow clears them the same rebuild that moves
	// every word; a new session starts with nothing selected.
	selActive bool
	selAnchor selPos
	selHead   selPos

	// Find highlight state. findMarks are the rects from the most recent Find,
	// each tied to the word box that paints it, and findActive selects the
	// match painted in the active color. Like the selection, the marks point
	// into the current arena, so Reflow clears them with it.
	findMarks  []findMark
	findActive int

	// JS runtime for this document's scripts. nil when the caller did not
	// supply WithJS, so a session without scripting works exactly as before.
	jsRT *js.Runtime
	// providedDoc is a bounded document tree from a non-HTML parser. It is used
	// by MIME-aware loaders for XML/XHTML, which must not pass through the HTML
	// tokenizer.
	providedDoc *dom.Document
	// parserExecutedScripts prevents the post-parse fallback pass from running
	// inline or blocking external scripts that already ran at their parser pause.
	parserExecutedScripts map[*dom.Node]bool
	parserExternalCount   int
	runtimeBound          bool
	documentStartRan      bool

	// drag is the in-flight drag-and-drop operation, or nil when nothing is
	// being dragged. Owned by StartDrag/DragTo/EndDrag in drag.go and
	// guarded by opMu like the rest of the interaction state.
	drag           *dragState
	testDriverDrag *testDriverDragCandidate

	// scriptErrs is a bounded window of the document's uncaught script errors
	// and scriptErrTotal is how many there were. Errors still do not stop the
	// load - that behaviour is deliberate and unchanged - they are only no
	// longer thrown away: nine real pages painted nothing with rc 0 and an
	// empty stderr because the reason lived in errors this function discarded
	// (GOOSIE-LIVE-001). A page's error text is untrusted input, so both the
	// number retained and the length of each message are capped; the total
	// keeps reporting honest when a document throws a thousand times.
	scriptErrMu    sync.Mutex
	scriptErrs     []ScriptError
	scriptErrTotal int

	// jsNodeBudget gates script-driven DOM insertions against this document's
	// admission caps (node count, depth, attributes) before they happen, so a
	// hostile page cannot grow the tree past the caps and have the overrun
	// noticed only later by validateArena/walkDocument. nil when the caller did
	// not supply WithJS.
	jsNodeBudget *docNodeBudget

	// media owns this document's video playback: one controller per element,
	// the downloaded files behind them, and the backend that decodes them. nil
	// when the caller did not supply WithMedia, so a session without a playback
	// backend leaves <video> as the placeholder paint already draws.
	media *mediaRegistry

	// Transitions tracks active CSS transitions for animated property changes.
	// nil when no transitions have been started.
	Transitions *TransitionTracker

	// Animations tracks active @keyframes animations. nil when no animations
	// have been started.
	Animations *AnimationController

	// workCounts are per-session pipeline counters. They are intentionally
	// monotonic so tests and diagnostics can take before/after snapshots around
	// user interactions without relying on wall-clock timings.
	workCounts pipelineWorkCounts
	// styleDirty marks DOM mutations since the last style resolve. CSSOM reads
	// synchronously refresh the cascade when a script asks for computed values.
	styleDirty atomic.Bool
	// layoutDirty marks document work not reflected in the layout arena yet.
	// CSSOM View geometry reads synchronously flush this work.
	layoutDirty atomic.Bool

	// inv is the caller's invalidation batch. When non-nil, JSMutationSink
	// records DOM mutations into it so the pipeline knows to re-style and
	// re-layout. Set via WithInvalidation.
	inv *Invalidation

	// Appearance preferences for media query evaluation.
	darkMode     bool
	highContrast bool

	// userScripts configured for injection into this document.
	userScripts []*userscript.Script

	// opMu serializes pipeline operations across threads: the UI pump
	// (input handlers, the JS pump, repaints) and navigation drains
	// (session install/teardown, deferred completions) share sessions, and
	// pipeline state (Styles, Arena, focus, selection) must move atomically
	// between them. Lock order is opMu before imgMu; nothing takes those in
	// reverse, and no opMu-guarded method calls another one. Leaf helpers
	// (layout/paint/metrics, the JS runtime, invalidation records) never
	// take opMu.
	opMu sync.Mutex
	// deferredActive counts in-flight deferred font/image loads, which write
	// image and font tables outside opMu by design. PumpJS skips reflowing
	// while it is nonzero and retries next tick, so a pump reflow never
	// reads a half-attached document.
	deferredActive int32
}

// WorkCounts reports how many expensive pipeline stages this session has run.
// The counters are useful for deterministic performance assertions: tests can
// compare snapshots around an interaction and catch redundant restyles,
// relayouts, or display-list builds without timing a machine-dependent path.
// StyleNodeVisits counts nodes actually given to the style resolver; an
// incremental interaction can resolve fewer than the full document.
type WorkCounts struct {
	StyleResolves     uint64
	StyleNodeVisits   uint64
	LayoutBuilds      uint64
	DisplayListBuilds uint64
}

type pipelineWorkCounts struct {
	styleResolves     atomic.Uint64
	styleNodeVisits   atomic.Uint64
	layoutBuilds      atomic.Uint64
	displayListBuilds atomic.Uint64
}

// WorkCounts returns a concurrency-safe snapshot of this session's pipeline
// work counters.
func (s *Session) WorkCounts() WorkCounts {
	if s == nil {
		return WorkCounts{}
	}
	return WorkCounts{
		StyleResolves:     s.workCounts.styleResolves.Load(),
		StyleNodeVisits:   s.workCounts.styleNodeVisits.Load(),
		LayoutBuilds:      s.workCounts.layoutBuilds.Load(),
		DisplayListBuilds: s.workCounts.displayListBuilds.Load(),
	}
}

// CSSLinker fetches one linked style sheet. base is the document URL the href
// resolves against; the returned string is the sheet's CSS text. A linker that
// fails simply contributes no rules for that href; document rendering does not
// stop for one missing sheet. Linked sheets are fetched concurrently, so a
// linker may be called from several goroutines at once.
type CSSLinker func(base, href string) (string, error)

// IFrameDocumentLoader fetches and parses a child HTML document for an iframe.
// The embedding layer owns network policy and byte decoding; engine supplies
// the parent URL and the iframe's source attribute.
type IFrameDocumentLoader func(base, src string) (*dom.Document, error)

// IFrameFormSubmitter posts an encoded form body to a child browsing context
// and returns its bounded parsed response document. The caller owns transport
// and origin policy.
type IFrameFormSubmitter func(base, target, contentType string, body []byte) (*dom.Document, error)

// ImageFetcher fetches one image subresource's raw bytes. base is the document
// URL the src resolves against; the returned bytes are the encoded image. A
// fetcher that fails leaves that image undecoded, and the box keeps the layout
// space its attributes reserve, so a missing image does not stop rendering.
type ImageFetcher func(base, url string) ([]byte, error)

// FontResult is one fetched @font-face resource: raw bytes plus the
// response headers the CORS check reads. Headers may be nil for
// non-network sources, which the check treats as same-origin-safe.
type FontResult struct {
	Data    []byte
	Headers map[string]string
}

// FontFetcher fetches one @font-face resource. Same shape as ImageFetcher,
// but the result carries response headers: cross-origin faces require an
// Access-Control-Allow-Origin grant (see fontCORSAllowed), so the engine
// must see more than bytes.
type FontFetcher func(base, url string) (FontResult, error)

// FontRegistry accepts parsed font bytes and returns a 1-based index. The
// engine calls it for every @font-face src it fetches; raster.Fonts satisfies
// this interface.
type FontRegistry interface {
	Register(data []byte) (uint16, error)
}

// Option adjusts how a Session is built.
type Option func(*Session)

// WithMetrics supplies real font measurements so the inline pass measures word
// widths and paint emits per-glyph advances from the same source. Without it,
// layout falls back to a half-em-per-glyph estimate and paint stretches text.
func WithMetrics(m layout.Metrics) Option {
	return func(s *Session) { s.metrics = m }
}

// WithViewportH supplies the height the document's viewport is laid out
// against. A fixed box resolves `bottom` and a percentage height against it, and
// the viewport units are a share of it. Without it the initial containing block
// has no height, so a bottom-pinned box lands above the visible area and `vh`
// falls back to the size the CSS parser assumed.
func WithViewportH(h float32) Option {
	return func(s *Session) {
		if finite(float64(h)) && h > 0 {
			s.viewportH = h
		}
	}
}

func validateDocumentTreeLimits(doc *dom.Document, limits dom.ParseLimits) error {
	if doc == nil {
		return fmt.Errorf("pre-parsed document is nil")
	}
	nodes, attributes, attributeBytes := 0, 0, 0
	var visit func(*dom.Node, int) error
	visit = func(node *dom.Node, depth int) error {
		if node == nil {
			return nil
		}
		nodes++
		if nodes > limits.Nodes {
			return fmt.Errorf("document node limit exceeded (%d)", limits.Nodes)
		}
		if depth > limits.Depth {
			return fmt.Errorf("document tree depth limit exceeded (%d)", limits.Depth)
		}
		attributes += len(node.Attr)
		for _, attr := range node.Attr {
			attributeBytes += len(attr.Name) + len(attr.Value)
		}
		if attributes > limits.Attributes {
			return fmt.Errorf("document attribute limit exceeded (%d)", limits.Attributes)
		}
		if attributeBytes > limits.AttributeBytes {
			return fmt.Errorf("document attribute byte limit exceeded (%d)", limits.AttributeBytes)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(&doc.Node, 0)
}

// WithLinkedCSS lets the session resolve and fetch <link rel=stylesheet>
// sheets during the same document walk that collects <style> blocks, so the
// sheets keep their document-order position in the cascade. base is the
// document's final URL (after redirects); href resolution happens here so the
// fetcher only ever sees absolute URLs.
func WithLinkedCSS(base string, fetch CSSLinker) Option {
	return func(s *Session) {
		s.linkBase = base
		s.linker = fetch
	}
}

// WithImages lets the session fetch and decode the <img> subresources a
// document references during construction, so layout can size each image box
// from its intrinsic size and paint can draw it. base is the document's final
// URL, which srcs resolve against.
func WithImages(base string, fetch ImageFetcher) Option {
	return func(s *Session) {
		s.imgBase = base
		s.imgFetch = fetch
	}
}

// WithDeferredImages is WithImages for the progressive-paint path: the
// document's image references are collected during construction but nothing
// is fetched, so the caller can paint the first frame immediately and call
// LoadDeferredImages to bring the images in afterwards.
func WithDeferredImages(base string, fetch ImageFetcher) Option {
	return func(s *Session) {
		s.imgBase = base
		s.imgFetch = fetch
		s.deferImages = true
	}
}

// WithCustomFontLoading lets the session fetch @font-face resources and
// register them before style resolution. base is the document URL that font
// src URLs resolve against; reg receives the raw font bytes and returns a
// 1-based index threaded through FontSlot.CustomIdx.
func WithCustomFontLoading(base string, fetch FontFetcher, reg FontRegistry) Option {
	return func(s *Session) {
		s.fontBase = base
		s.fontFetch = fetch
		s.fontReg = reg
	}
}

// WithDeferredCustomFontLoading is WithCustomFontLoading for the
// progressive-paint path: @font-face descriptors are collected during
// construction but the font files are not fetched, so the first frame renders
// with system fallback fonts and LoadDeferredFonts brings the web faces in
// afterwards without holding up first paint.
func WithDeferredCustomFontLoading(base string, fetch FontFetcher, reg FontRegistry) Option {
	return func(s *Session) {
		s.fontBase = base
		s.fontFetch = fetch
		s.fontReg = reg
		s.deferFonts = true
	}
}

// WithScripts lets the session fetch <script src> bundles during construction
// so external scripts run in document order between the inline ones. base is
// the document's final URL (after redirects); srcs resolve against it and the
// fetcher only ever sees absolute http(s) URLs. Without it, only inline
// scripts run and bundle-booted pages stay shells.
func WithScripts(base string, fetch ScriptFetcher) Option {
	return func(s *Session) {
		s.scriptBase = base
		s.scriptFetch = fetch
	}
}

// WithIFrameDocumentLoader supplies the child-document loader used by iframe
// browsing contexts.
func WithIFrameDocumentLoader(loader IFrameDocumentLoader) Option {
	return func(s *Session) { s.iframeLoader = loader }
}

// WithIFrameFormSubmitter supplies the host transport for POST forms whose
// target names a child browsing context.
func WithIFrameFormSubmitter(submit IFrameFormSubmitter) Option {
	return func(s *Session) { s.iframeFormSubmit = submit }
}

// WithContentLanguage supplies the response's Content-Language value to the
// document language algorithm used by selectors when no element declares one.
func WithContentLanguage(value string) Option {
	return func(s *Session) { s.contentLanguage = value }
}

// WithDocumentEncoding supplies the encoding already selected while decoding
// a network response. The parser receives Unicode text, so its meta scan must
// not replace the byte decoder's result (especially when a BOM or transport
// charset took precedence).
func WithDocumentEncoding(label string) Option {
	return func(s *Session) { s.documentEncoding = documentEncodingLabel(label) }
}

func documentEncodingLabel(label string) string {
	if strings.EqualFold(label, "utf-8") {
		return "UTF-8"
	}
	if strings.HasPrefix(strings.ToLower(label), "iso-8859-") {
		return strings.ToUpper(label)
	}
	return label
}

// WithJS supplies a JavaScript runtime for executing the document's classic
// scripts. The engine walks <script> elements in document order after the DOM
// is built and runs each one through the runtime. Script errors are recorded
// but do not stop the pipeline: a page with a broken ad script still renders.
// console receives the runtime's console output; pass nil to discard it.
func WithJS(rt *js.Runtime) Option {
	return func(s *Session) { s.jsRT = rt }
}

// WithDocumentTree supplies a document already parsed by a MIME-aware parser.
// The session still applies its tree admission limits before styling or script
// execution. The raw source should also be passed as NewSession's first
// argument so the document byte limit is enforced.
func WithDocumentTree(doc *dom.Document) Option {
	return func(s *Session) { s.providedDoc = doc }
}

// WithInvalidation supplies the invalidation batch that JSMutationSink records
// DOM mutations into. Without it, JSMutationSink is a no-op and the engine
// does not learn about script-driven DOM changes.
func WithInvalidation(inv *Invalidation) Option {
	return func(s *Session) { s.inv = inv }
}

// WithDarkMode configures whether the session matches (prefers-color-scheme: dark).
func WithDarkMode(enabled bool) Option {
	return func(s *Session) {
		s.darkMode = enabled
	}
}

// WithHighContrast configures whether the session matches (prefers-contrast: more).
func WithHighContrast(enabled bool) Option {
	return func(s *Session) {
		s.highContrast = enabled
	}
}

// WithUserScripts configures user scripts to execute during the document lifecycle.
func WithUserScripts(scripts []*userscript.Script) Option {
	return func(s *Session) {
		s.userScripts = scripts
	}
}

// fontJob is one @font-face descriptor set queued for fetching.
type fontJob struct {
	family, srcURL, weightStr, styleStr string
	// base is the document URL the src resolved against: the CORS origin
	// the response headers are judged against.
	base string
}

// NewSession parses HTML and builds the pipeline state. The caller provides the
// raw HTML and any author style sheets; <style> blocks inside the document are
// collected automatically, and the UA stylesheet is added internally.
// The returned session has a fully laid-out arena ready for paint.
func NewSession(html string, authorCSS []string, viewportW float32, opts ...Option) (*Session, error) {
	s := &Session{sub: &subBudget{}}
	for _, opt := range opts {
		opt(s)
	}
	if err := validateWidth(viewportW); err != nil {
		return nil, err
	}
	s.viewportW = viewportW
	if len(html) > MaxDocumentBytes {
		return nil, fmt.Errorf("HTML byte limit exceeded (%d)", MaxDocumentBytes)
	}
	parseLimits := dom.ParseLimits{
		Nodes: MaxDocumentNodes, Depth: MaxDocumentDepth,
		Attributes: MaxAttributes, AttributeBytes: MaxAttributeBytes,
	}
	var doc *dom.Document
	var err error
	if s.providedDoc != nil {
		doc = s.providedDoc
		if err = validateDocumentTreeLimits(doc, parseLimits); err != nil {
			return nil, err
		}
	} else if s.jsRT != nil {
		doc, err = dom.ParseBoundedWithParserCallbacks(html, parseLimits,
			func(node *dom.Node, attrs []dom.Attribute) *dom.Node {
				if s.runtimeBound && s.jsRT != nil {
					return s.jsRT.UpgradeParserElementWithAttributes(node, attrs)
				}
				return node
			},
			func(node *dom.Node, attr dom.Attribute) {
				if s.runtimeBound && s.jsRT != nil {
					s.jsRT.ParserElementAttributeChanged(node, attr)
				}
			},
			func(node *dom.Node) {
				if s.runtimeBound && s.jsRT != nil {
					s.jsRT.ParserNodeInserted(node)
				}
			},
			func(doc *dom.Document, node *dom.Node) {
				if s.documentEncoding != "" {
					doc.CharacterSet = s.documentEncoding
				}
				doc.ContentLanguage = s.contentLanguage
				s.runParserScript(doc, node, authorCSS)
			},
		)
	} else {
		doc, err = dom.ParseBounded(html, parseLimits)
	}
	if err != nil {
		return nil, err
	}
	if s.documentEncoding != "" {
		doc.CharacterSet = s.documentEncoding
	}
	s.Doc = doc
	doc.ContentLanguage = s.contentLanguage
	// Bind the completed document as well, so iframes and other DOM objects
	// created after the last parser script are visible to the runtime.
	s.bindJSRuntime(doc)
	// The image maps and decoded-pixel reservation exist before deferred scripts
	// or image loading; parser-time scripts used the same reservation from the
	// first parser pause.
	s.initImageMaps()
	// Inline scripts can synchronously read getComputedStyle. Collect the
	// stylesheets before running page scripts, then lazily resolve the cascade on
	// the first CSSOM read. The normal final style phase resolves it once for
	// documents that do not query computed style during script execution.
	preScriptSheets, err := checkedSheets(doc, authorCSS, s.linkBase, s.linker, s.mediaOptions(), s.sub)
	if err != nil {
		return nil, err
	}
	preScriptStyleSignature := stylesheetDOMSignature(doc)
	s.sheets = preScriptSheets
	s.styleResolver = style.NewResolver(preScriptSheets)
	s.styleDirty.Store(true)
	s.layoutDirty.Store(true)
	s.runScripts()
	s.opMu.Lock()
	s.clearDetachedFocusLocked()
	s.opMu.Unlock()
	sheets := preScriptSheets
	if stylesheetDOMSignature(doc) != preScriptStyleSignature {
		sheets, err = checkedSheets(doc, authorCSS, s.linkBase, s.linker, s.mediaOptions(), s.sub)
		if err != nil {
			return nil, err
		}
	}
	s.loadFonts(sheets)
	s.sheets = sheets
	s.styleResolver = style.NewResolver(sheets)
	s.workCounts.styleResolves.Add(1)
	s.Styles = s.styleResolver.ResolveViewport(doc, s.styleViewport(), s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
	s.PseudoStyles = s.styleResolver.ResolvePseudoElements(doc, s.Styles, s.customFonts)
	s.styleDirty.Store(false)
	s.startAnimations()
	s.loadImages()
	if err := s.Reflow(viewportW); err != nil {
		return nil, err
	}
	// The document is parsed, styled and laid out, so now run the lifecycle
	// events a page's own boot registered on: DOMContentLoaded on the document
	// (whose propagation path ends at window, where four of vnexpress's
	// listeners sit) and then load on the window, with readyState moved to
	// "interactive" and "complete" in between.
	//
	// Firing here rather than at the end of the parse is a deliberate deviation
	// from browser timing, and the reason is measurable: Goosie runs every
	// script after the whole document exists, and a ready handler that reads a
	// computed style or measures a box must see the load cascade, not an empty
	// style map. The cost is that the handler's writes arrive after the first
	// layout pass, so a mutation re-runs style and layout here - exactly what
	// the pump does for a later timer callback.
	startupTaskRan := s.jsRT != nil && s.jsRT.TickDeferredTimer()
	lifecycleMutated := false
	if s.jsRT != nil {
		s.jsRT.AutofocusDocument()
		lifecycleMutated = s.jsRT.FireDocumentDOMContentLoaded()
		// HTML's load event waits for child navigables. Resolve parser-discovered
		// iframe documents after DOMContentLoaded and before firing document load.
		s.jsRT.FlushIFrameLoads()
		lifecycleMutated = s.jsRT.FireDocumentLoad() || lifecycleMutated
	}
	if startupTaskRan || lifecycleMutated {
		if err := s.RestyleAndReflow(viewportW); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func stylesheetDOMSignature(doc *dom.Document) string {
	if doc == nil {
		return ""
	}
	var signature strings.Builder
	appendField := func(value string) {
		fmt.Fprintf(&signature, "%d:%s", len(value), value)
	}
	doc.Node.Walk(func(node *dom.Node) bool {
		if !node.Element() {
			return true
		}
		switch node.Data {
		case "style":
			appendField("style")
			appendField(node.GetAttribute("type"))
			appendField(node.GetAttribute("media"))
			appendField(node.TextContent())
		case "link":
			appendField("link")
			appendField(node.GetAttribute("rel"))
			appendField(node.GetAttribute("href"))
			appendField(node.GetAttribute("media"))
		}
		return true
	})
	return signature.String()
}

// bindJSRuntime connects the live parser document to its realm. The first call
// installs document-scoped providers; later calls refresh the DOM wrapper and
// mutation-budget baseline after parser construction has added more nodes.
func (s *Session) bindJSRuntime(doc *dom.Document) {
	s.Doc = doc
	s.initImageMaps()
	if s.jsRT == nil || doc == nil {
		return
	}
	firstBind := !s.runtimeBound
	if firstBind {
		if s.iframeLoader != nil {
			s.jsRT.SetIFrameLoaderWithBase(s.iframeLoader)
		}
		if s.iframeFormSubmit != nil {
			s.jsRT.SetIFrameFormSubmitter(s.iframeFormSubmit)
		}
		if s.scriptFetch != nil {
			s.jsRT.SetIFrameScriptFetcher(s.scriptFetch)
			s.jsRT.SetDynamicScriptLimit(maxExternalScripts)
			s.jsRT.SetDynamicScriptFetcher(func(base, src string) ([]byte, error) {
				abs, ok := resolveSubresourceURL(base, src)
				if !ok || !s.jsRT.AllowsScriptURL(abs, base) {
					return nil, errors.New("external script blocked")
				}
				return s.scriptFetch(base, abs)
			})
		}
		s.jsRT.SetIFrameModuleRunner(func(runtime *js.Runtime, childDoc *dom.Document, documentURL string) {
			if childDoc == nil {
				return
			}
			root := childDoc.HTML
			if root == nil {
				root = &childDoc.Node
			}
			s.runModuleScripts(runtime, root, documentURL, maxExternalScripts, time.Now().Add(ScriptFetchBudget))
		})
		s.jsRT.SetComputedStyleProvider(s.computedStyleValues)
		s.jsRT.SetPseudoStyleProvider(s.pseudoStyleValues)
		s.installLayoutProviders()
		s.jsRT.SetSubresourceBudget(s.sub)
		s.jsRT.SetImageLoader(s)
		// The pixels a successful script load produced come back through this
		// sink. It only records them; PumpJS attaches them after the turn and
		// forces the layout that makes an in-document <img> paint (see
		// script_image.go). Installed next to the loader, before any script
		// runs, so a synchronous during-parse assignment is recorded too.
		s.jsRT.SetImagePixelSink(s.retainScriptImage)
		s.jsRT.SetViewportRect(0, 0, s.viewportW, s.viewportH)
		s.jsRT.ChainActiveElementProvider(s.ActiveElement)
		s.jsRT.ChainOnFocusChange(func(node *dom.Node, focused bool) {
			if focused {
				s.FocusNode(node)
			} else {
				s.BlurNode(node)
			}
		})
		s.jsRT.ChainOnFocusRestore(func(node *dom.Node) {
			s.RestoreFocusNode(node)
		})
		var testDriverClipboardText, testDriverClipboardHTML string
		s.jsRT.ChainOnTestDriverKeyDefault(func(key string, shift, ctrl, alt, meta bool) {
			if meta && !ctrl && !alt && key == "Delete" && s.FocusedEditable() {
				s.deleteContentEditableSoftLineForward()
				return
			}
			if alt && (key == "Backspace" || key == "Delete") {
				s.deleteContentEditableByWord(key == "Delete")
				return
			}
			if (ctrl || meta) && !alt {
				switch strings.ToLower(key) {
				case "backspace":
					s.deleteContentEditableByWord(false)
				case "delete":
					s.deleteContentEditableByWord(true)
				case "a":
					if s.FocusedEditable() {
						if !s.SelectAllControl() && s.jsRT != nil {
							_, _ = s.jsRT.Evaluate(`(function(){var h=document.activeElement,sel=getSelection();if(h&&h.isContentEditable&&sel)sel.selectAllChildren(h)})()`)
						}
					} else {
						s.SelectAll()
					}
				case "c":
					testDriverClipboardText, testDriverClipboardHTML = s.CopySelectionData()
				case "x":
					testDriverClipboardText, testDriverClipboardHTML, _ = s.CutSelectionData()
				case "v":
					s.PasteData(testDriverClipboardText, testDriverClipboardHTML)
				case "z":
					if host := s.Focused(); s.FocusedEditable() && host != nil && s.jsRT != nil {
						inputType := "historyUndo"
						if shift {
							inputType = "historyRedo"
						}
						if s.jsRT.DispatchTrustedInputEvent(host, nil, inputType) {
							s.jsRT.DispatchTrustedInput(host, nil, inputType, false)
						}
					}
				case "b":
					if host := s.Focused(); s.FocusedEditable() && host != nil && s.jsRT != nil {
						if s.jsRT.DispatchTrustedInputEvent(host, nil, "formatBold") {
							_, _ = s.jsRT.EvaluateUserInput(`document.execCommand("bold")`, "formatBold")
						}
					}
				}
				return
			}
			if shift && !ctrl && !alt && !meta && key == "Enter" {
				s.EditWithShift(EditEnter, 0, true)
				return
			}
			if ctrl || alt || meta {
				return
			}
			switch key {
			case "Backspace":
				s.Edit(EditBackspace, 0)
			case "Delete":
				s.Edit(EditDelete, 0)
			case "ArrowLeft":
				s.Edit(EditLeft, 0)
			case "ArrowRight":
				s.Edit(EditRight, 0)
			case "Enter":
				s.Edit(EditEnter, '\n')
			default:
				runes := []rune(key)
				if len(runes) == 1 && runes[0] >= 0x20 {
					s.Edit(EditRune, runes[0])
				}
			}
		})
		s.jsRT.ChainOnTestDriverPointer(s.TestDriverPointerAction)
		s.jsRT.ChainOnEscapeKey(func() bool {
			return s.CancelTestDriverDrag() || s.CloseTopmostAutoPopover()
		})
		s.jsRT.ChainOnUserInput(s.MarkUserInput)
		if s.media != nil {
			s.jsRT.SetMediaProvider(s.MediaController)
		}
		s.jsRT.ChainOnMutation(s.JSMutationSink())
		s.jsRT.ChainOnMutation(func() {
			s.styleDirty.Store(true)
			s.layoutDirty.Store(true)
		})
	}
	if firstBind {
		s.jsRT.SetDOM(doc)
		s.runtimeBound = true
	} else {
		// The same document is being appended by the tree builder. Rebinding it
		// would discard live wrapper, image-load, and iframe state caches.
		s.jsRT.QueueExistingIFrames()
	}
	s.jsRT.RefreshDocumentTreeReferences()
	s.jsNodeBudget = newDocNodeBudget(doc)
	s.jsRT.SetMutationBudget(s.jsNodeBudget)
	if !s.documentStartRan {
		s.runUserScripts(userscript.RunAtDocumentStart)
		s.documentStartRan = true
	}
}

// runParserScript is called by the tree builder at each parser pause. It
// executes parser-inserted classic inline scripts and blocking external scripts
// against the partially constructed live document before the next token.
func (s *Session) runParserScript(doc *dom.Document, node *dom.Node, authorCSS []string) {
	if s.jsRT == nil || doc == nil || node == nil || !node.Element() || node.Data != "script" || !isClassicScript(node) {
		return
	}
	if node.Namespace != dom.NSHTML && (node.Namespace != dom.NSSVG || !node.SVGScriptReady) {
		return
	}
	if s.parserExecutedScripts == nil {
		s.parserExecutedScripts = make(map[*dom.Node]bool)
	}
	if s.parserExecutedScripts[node] {
		return
	}
	s.bindJSRuntime(doc)
	src := scriptSrc(node)
	if src != "" {
		if s.parserExternalCount >= maxExternalScripts {
			s.parserExecutedScripts[node] = true
			return
		}
		s.parserExternalCount++
	}
	if src != "" && (node.HasAttribute("async") || node.HasAttribute("defer")) {
		return
	}
	source, name := node.TextContent(), "inline"
	if src != "" {
		name = src
		if strings.HasPrefix(strings.ToLower(src), "data:") {
			var ok bool
			source, ok = decodeDataScriptURL(src)
			if !ok {
				s.parserExecutedScripts[node] = true
				return
			}
		} else {
			if s.scriptFetch == nil {
				s.parserExecutedScripts[node] = true
				return
			}
			abs, ok := resolveSubresourceURL(s.scriptBase, src)
			if !ok {
				s.parserExecutedScripts[node] = true
				return
			}
			name = abs
			data, err := s.scriptFetch(s.scriptBase, abs)
			if err != nil {
				s.parserExecutedScripts[node] = true
				return
			}
			source = string(data)
		}
	}
	s.parserExecutedScripts[node] = true
	if source == "" {
		return
	}
	// Stylesheets preceding this script are already in the live tree. Install
	// their current cascade so synchronous CSSOM reads in the script see them.
	sheets, err := checkedSheets(doc, authorCSS, s.linkBase, s.linker, s.mediaOptions(), s.sub)
	if err != nil {
		s.recordScriptError(err)
	} else {
		s.sheets = sheets
		s.styleResolver = style.NewResolver(sheets)
		s.styleDirty.Store(true)
		s.layoutDirty.Store(true)
	}
	if err := s.jsRT.RunParserScript(source, name); err != nil {
		s.recordScriptError(err)
	}
}

// loadFonts extracts @font-face rules from every stylesheet, fetches the font
// files, registers them with the font registry, and keeps the resulting
// family→index table on the session so style resolution can match custom family
// names against this document's own faces.
//
// Fetching runs through a bounded worker pool, the same shape as loadImages:
// pages declare several families and weights, and a sequential walk spends
// the whole wall-clock budget on the first handful. Registration stays on
// this goroutine in document order so registry indices are deterministic and
// a later rule overwrites the same family exactly as the sequential walk did.
func (s *Session) loadFonts(sheets []*css.Stylesheet) {
	if s.fontFetch == nil || s.fontReg == nil {
		return
	}
	jobs := collectFontJobs(sheets, s.fontBase)
	if len(jobs) == 0 {
		return
	}
	if s.deferFonts {
		s.pendingFonts = jobs
		return
	}
	s.applyFontJobs(jobs)
}

// DeferredFontsPending reports how many @font-face references were collected
// but not yet fetched.
func (s *Session) DeferredFontsPending() int {
	return len(s.pendingFonts)
}

// LoadDeferredFonts fetches and registers the @font-face resources that
// WithDeferredCustomFontLoading postponed, then calls onReady with the number
// of fonts that registered successfully. The caller runs it on its own
// goroutine; onReady fires once every font has settled, after which the caller
// should re-resolve styles and reflow so text swaps to the web faces.
func (s *Session) LoadDeferredFonts(onReady func(loaded int)) {
	jobs := s.pendingFonts
	s.pendingFonts = nil
	if len(jobs) == 0 {
		if onReady != nil {
			onReady(0)
		}
		return
	}
	// In-flight deferred work gates PumpJS reflows (see deferredActive):
	// the font table attaches outside opMu by design.
	atomic.AddInt32(&s.deferredActive, 1)
	defer atomic.AddInt32(&s.deferredActive, -1)
	s.applyFontJobs(jobs)
	if onReady != nil {
		onReady(1)
	}
}

// RefreshAfterFonts re-resolves styles against the now-populated customFonts
// table. Call it after LoadDeferredFonts completes so text picks up the web
// faces instead of the system fallbacks the first paint used.
func (s *Session) RefreshAfterFonts() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if len(s.sheets) == 0 {
		return
	}
	s.workCounts.styleResolves.Add(1)
	if s.styleResolver == nil {
		s.styleResolver = style.NewResolver(s.sheets)
	}
	s.Styles = s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
	s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, s.Styles, s.customFonts)
}

// applyFontJobs fetches font bytes through a bounded worker pool and registers
// each successful result with the font registry, populating s.customFonts.
func (s *Session) applyFontJobs(jobs []fontJob) {
	data := make([]FontResult, len(jobs))
	deadline := time.Now().Add(FontFetchBudget)
	const fontWorkers = 8
	sem := make(chan struct{}, fontWorkers)
	var wg sync.WaitGroup
	for i := range jobs {
		if time.Now().After(deadline) {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fetched, err := s.fontFetch(s.fontBase, jobs[i].srcURL)
			if err != nil {
				return
			}
			// The shared subresource total prices fonts against the same
			// bytes stylesheets and images spend: over budget, the face
			// stays unfetched and text keeps its fallback.
			if !s.sub.take(len(fetched.Data)) {
				return
			}
			data[i] = fetched
		}(i)
	}
	wg.Wait()

	fonts := &style.CustomFonts{}
	var fetchedBytes int64
	for i, j := range jobs {
		if len(data[i].Data) == 0 {
			continue
		}
		fetchedBytes += int64(len(data[i].Data))
		if fetchedBytes > maxFontBytes {
			return
		}
		// Bytes transferred still count above; use is a separate question.
		// A cross-origin face without a CORS grant never registers, and
		// text keeps its fallback exactly as for a corrupt download.
		if !fontCORSAllowed(jobs[i].base, jobs[i].srcURL, data[i].Headers) {
			continue
		}
		idx, err := s.fontReg.Register(data[i].Data)
		if err != nil {
			continue
		}
		bold := j.weightStr == "bold" || j.weightStr == "700" || j.weightStr == "800" || j.weightStr == "900"
		light := j.weightStr == "300" || j.weightStr == "200" || j.weightStr == "100"
		italic := j.styleStr == "italic" || j.styleStr == "oblique"
		fonts.Register(j.family, idx, bold, italic, light)
	}
	// The table publishes under opMu so a concurrent pump reflow resolving
	// styles never reads a half-attached table. Fetching above stays
	// outside the lock: holding it across network I/O would stall the pump.
	s.opMu.Lock()
	s.customFonts = fonts
	s.opMu.Unlock()
}

// collectFontJobs walks every stylesheet's @font-face rules and builds the
// fetch job list: family, resolved src URL, weight and style descriptors.
func collectFontJobs(sheets []*css.Stylesheet, fontBase string) []fontJob {
	var jobs []fontJob
	for _, sheet := range sheets {
		for _, ff := range sheet.FontFaces {
			if len(jobs) >= maxFontRequests {
				break
			}
			var family, srcURL, weightStr, styleStr string
			for _, d := range ff.Declarations {
				switch d.Property {
				case "font-family":
					family = strings.Trim(d.Value, "\"'")
				case "src":
					srcURL = extractFontURL(d.Value)
				case "font-weight":
					weightStr = strings.TrimSpace(d.Value)
				case "font-style":
					styleStr = strings.TrimSpace(d.Value)
				}
			}
			if family == "" || srcURL == "" {
				continue
			}
			abs, ok := resolveSubresourceURL(fontBase, srcURL)
			if !ok {
				continue
			}
			jobs = append(jobs, fontJob{family: family, srcURL: abs, weightStr: weightStr, styleStr: styleStr, base: fontBase})
		}
	}
	return jobs
}

// extractFontURL pulls the first url(...) reference out of a @font-face src
// value. The src property can carry multiple fallback formats; the engine only
// serves TrueType and OpenType, so the first url() is the one to try.
func extractFontURL(v string) string {
	t, ok := css.ScanURL(v)
	if !ok {
		return ""
	}
	return t.Payload
}

// maxFontRequests caps how many @font-face sources one document may fetch so a
// stylesheet that references dozens of weights cannot stall first paint.
const maxFontRequests = 16

// maxFontBytes caps the total font payload one document may download. A single
// woff2 is typically 20–100 KiB; 10 MiB covers generous real-world pages while
// blocking a hostile stylesheet that references gigabytes of font data.
const maxFontBytes = 10 << 20

// FontFetchBudget is the wall-clock deadline for all font fetching. After this
// elapses, remaining @font-face rules are skipped and the document falls back
// to system fonts.
//
// It bounds spawn scheduling inside the engine; hosts also cancel in-flight
// work with it (a deadline context around their fetchers), because a hung
// host would otherwise ride out the transport timeout instead.
const FontFetchBudget = 6 * time.Second

// maxLoadedImages bounds how many image subresources one document may fetch,
// so a page that references hundreds cannot stall the first frame.
const maxLoadedImages = 256

// ScriptFetcher fetches one external classic script. base is the document URL
// the src resolves against; the returned bytes are the script source. A
// fetcher that fails leaves that bundle unexecuted, and the page keeps the
// globals its other scripts set, so one missing bundle does not stop
// rendering. Concurrent calls must be safe; the engine fetches through a
// bounded pool.
type ScriptFetcher func(base, url string) ([]byte, error)

// maxExternalScripts bounds how many <script src> bundles one document may
// fetch, mirroring maxLinkedSheets: a page that references hundreds cannot
// stall the first frame behind them.
const maxExternalScripts = 32

// ScriptFetchBudget stops external-script fetching past a wall-clock
// deadline; bundles left unfetched are skipped exactly like failed ones.
const ScriptFetchBudget = 6 * time.Second

// ImageFetchBudget stops subresource fetching past a wall-clock deadline so a
// slow or unreachable host cannot push the first render past its time limit;
// images left unfetched simply reserve their box and paint nothing.
//
// Like FontFetchBudget it bounds spawn scheduling; hosts cancel in-flight
// work with it around their fetchers for the same reason.
const ImageFetchBudget = 6 * time.Second

// MaxDocumentDecodedPixels bounds the total decoded pixels one document may
// retain across all its images and backgrounds.
const MaxDocumentDecodedPixels = 33554432

// imageReservation tracks per-document decoded pixel reservations.
type imageReservation struct {
	limit    int64
	reserved int64
	mu       sync.Mutex
}

func newImageReservation(limit int64) *imageReservation {
	return &imageReservation{limit: limit}
}

func (r *imageReservation) reserve(pixels int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pixels > r.limit-r.reserved {
		return false
	}
	r.reserved += pixels
	return true
}

func (r *imageReservation) release(pixels int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reserved -= pixels
}

// imgRef is one image subresource reference found in the document: the node
// that wants it, the absolute URL it resolves to, and whether it is a CSS
// background rather than an <img>.
type imgRef struct {
	id  dom.NodeID
	abs string
	bg  bool
	// pseudo is set for a generated ::before/::after background. The host
	// node ID is not used: the layout object the pixels attach to is the
	// pseudo box, which is keyed by this style pointer.
	pseudo *style.ComputedStyle
	// border marks a border-image source: fetched and budgeted like a
	// background (no layout natural size), attached to BorderImage.
	border bool
	// mask marks a mask-image source: fetched and budgeted like a
	// background (no layout natural size), attached to MaskImage.
	mask bool
}

// loadImages collects the document's image references during NewSession.
// With WithImages it fetches and applies them before the first layout; with
// WithDeferredImages it only stores the references for LoadDeferredImages.
func (s *Session) loadImages() {
	if s.Doc == nil {
		return
	}
	// Document-inline data: images resolve without a fetcher, so sessions
	// without one (headless, tests) still render them; network URLs simply
	// find nothing to call and stay empty.
	s.initImageMaps()
	if s.deferImages {
		s.pendingImgs = s.collectImageRefs()
		return
	}
	refs := s.collectImageRefs()
	s.applyImages(refs, s.fetchImages(refs))
}

// initImageMaps creates the image tables and the per-document pixel
// reservation. It is idempotent: NewSession calls it before scripts so a
// script-created image draws on the same reservation as a parsed <img>, and
// loadImages calls it again only when a session was built some other way.
func (s *Session) initImageMaps() {
	if s.imgReservation != nil {
		return
	}
	s.natural = make(map[dom.NodeID]layout.NaturalSize)
	s.images = make(map[dom.NodeID]stdimage.Image)
	s.scriptImgs = make(map[dom.NodeID]scriptImagePixels)
	s.pendingScriptImgs = make(map[dom.NodeID]scriptImagePixels)
	s.bgImages = make(map[dom.NodeID]stdimage.Image)
	s.pseudoBgImages = make(map[*style.ComputedStyle]stdimage.Image)
	s.pseudoBgVectorRef = make(map[*style.ComputedStyle]string)
	s.borderImages = make(map[dom.NodeID]stdimage.Image)
	s.masks = make(map[dom.NodeID]stdimage.Image)
	s.bgVectorSrc = make(map[string][]byte)
	s.bgVectorRef = make(map[dom.NodeID]string)
	s.bgNoRatio = make(map[string]bool)
	s.imgReservation = newImageReservation(MaxDocumentDecodedPixels)
	s.bgVectorUsed = 0
}

// DeferredImagesPending reports how many image references WithDeferredImages
// collected but has not fetched yet.
func (s *Session) DeferredImagesPending() int {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	return len(s.pendingImgs)
}

// LoadDeferredImages fetches and applies the image references that
// WithDeferredImages postponed, then calls onReady with the number of images
// that decoded successfully. It blocks for the duration of the fetch, so the
// interactive caller runs it on its own goroutine; onReady fires from that
// goroutine once every image has settled, after which a Reflow sees the new
// intrinsic sizes. Calling it on a session without deferred work reports 0
// immediately.
func (s *Session) LoadDeferredImages(onReady func(loaded int)) {
	s.imgMu.Lock()
	refs := s.pendingImgs
	s.pendingImgs = nil
	s.imgMu.Unlock()
	loaded := 0
	if len(refs) > 0 && s.imgFetch != nil {
		atomic.AddInt32(&s.deferredActive, 1)
		defer atomic.AddInt32(&s.deferredActive, -1)
		byURL := s.fetchImages(refs)
		s.applyImages(refs, byURL)
		loaded = len(byURL)
	}
	if onReady != nil {
		onReady(loaded)
	}
}

// collectImageRefs walks the styled document for <img> srcs and
// background-image URLs in document order. A URL can back several nodes (an
// <img> and a background), so each unique target is fetched once and fanned
// back out to all of them.
func (s *Session) collectImageRefs() []imgRef {
	var refs []imgRef
	var uniq []string
	seen := make(map[string]bool)
	_, _ = walkDocument(s.Doc, func(n *dom.Node) error {
		if !n.Element() {
			return nil
		}
		if n.Data == "img" {
			if src := strings.TrimSpace(resolveImgSource(n, s.viewportW)); src != "" {
				// data: payloads are document-inline bytes, not network
				// subresources: they bypass URL resolution and decode locally.
				if strings.HasPrefix(src, "data:") {
					refs = append(refs, imgRef{id: n.ID, abs: src})
					if !seen[src] {
						seen[src] = true
						uniq = append(uniq, src)
					}
					return nil
				}
				if abs, ok := resolveSubresourceURL(s.imgBase, src); ok {
					refs = append(refs, imgRef{id: n.ID, abs: abs})
					if !seen[abs] {
						seen[abs] = true
						uniq = append(uniq, abs)
					}
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.BackgroundImage != "" {
			bg := st.BackgroundImage
			if strings.HasPrefix(bg, "data:") {
				refs = append(refs, imgRef{id: n.ID, abs: bg, bg: true})
				if !seen[bg] {
					seen[bg] = true
					uniq = append(uniq, bg)
				}
			} else if abs, ok := resolveSubresourceURL(s.imgBase, bg); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, bg: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		for _, pseudo := range []string{"before", "after", "first-letter"} {
			ps, ok := s.PseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: pseudo}]
			if !ok || ps.BackgroundImage == "" {
				continue
			}
			bg := ps.BackgroundImage
			if strings.HasPrefix(bg, "data:") {
				refs = append(refs, imgRef{abs: bg, bg: true, pseudo: ps})
				if !seen[bg] {
					seen[bg] = true
					uniq = append(uniq, bg)
				}
			} else if abs, ok := resolveSubresourceURL(s.imgBase, bg); ok {
				refs = append(refs, imgRef{abs: abs, bg: true, pseudo: ps})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.MaskImage != "" {
			m := st.MaskImage
			if strings.HasPrefix(m, "data:") {
				refs = append(refs, imgRef{id: n.ID, abs: m, mask: true})
				if !seen[m] {
					seen[m] = true
					uniq = append(uniq, m)
				}
			} else if abs, ok := resolveSubresourceURL(s.imgBase, m); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, mask: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.BorderImage.Source != "" {
			src := st.BorderImage.Source
			if strings.HasPrefix(src, "data:") {
				refs = append(refs, imgRef{id: n.ID, abs: src, bg: true, border: true})
				if !seen[src] {
					seen[src] = true
					uniq = append(uniq, src)
				}
			} else if abs, ok := resolveSubresourceURL(s.imgBase, src); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, bg: true, border: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		return nil
	})
	return refs
}

// fetchImageBytes resolves one image reference to raw bytes: data: URIs
// decode inline (no fetcher needed, no network), everything else goes through
// the host fetcher. A nil fetcher therefore still renders document-inline
// images, which is what headless and fetcher-less test sessions need.
func (s *Session) fetchImageBytes(u string) ([]byte, error) {
	if strings.HasPrefix(u, "data:") {
		return decodeDataImageURL(u)
	}
	if s.imgFetch == nil {
		return nil, fmt.Errorf("engine: no image fetcher for %q", u)
	}
	return s.imgFetch(s.imgBase, u)
}

// fetchImages downloads and decodes each unique reference through a bounded
// worker pool. A page can carry dozens of images; a sequential walk spends
// the whole wall-clock budget on the first handful (at ~0.8s each, a 6s
// budget covers only ~7), leaving the rest to reserve their box and paint
// nothing. Fetching concurrently lets the same deadline cover the page, which
// is what a real browser does and what the reference renders assume.
func (s *Session) fetchImages(refs []imgRef) map[string]stdimage.Image {
	uniq := make([]string, 0, len(refs))
	seen := make(map[string]bool)
	bgOnly := make(map[string]bool)
	for _, r := range refs {
		if !seen[r.abs] {
			seen[r.abs] = true
			uniq = append(uniq, r.abs)
			bgOnly[r.abs] = r.bg
		} else if !r.bg {
			bgOnly[r.abs] = false
		}
	}
	byURL := make(map[string]stdimage.Image, len(uniq))
	var mu sync.Mutex
	loaded := 0
	// The attempt ceiling counts attempts, not successes: only the first
	// maxLoadedImages unique references in document order ever reach a worker.
	// Failed requests do not replenish the allowance, so 257 failing URLs cost
	// 256 fetches rather than an unbounded burst of dials. The in-worker
	// loaded check below stays as defense in depth.
	if len(uniq) > maxLoadedImages {
		uniq = uniq[:maxLoadedImages]
	}
	deadline := time.Now().Add(ImageFetchBudget)
	const imageWorkers = 12
	sem := make(chan struct{}, imageWorkers)
	var wg sync.WaitGroup
	s.imgMu.Lock()
	if s.imgReservation == nil {
		s.imgReservation = newImageReservation(MaxDocumentDecodedPixels)
	}
	reservation := s.imgReservation
	s.imgMu.Unlock()
	for _, u := range uniq {
		u := u
		if time.Now().After(deadline) {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			mu.Lock()
			full := loaded >= maxLoadedImages
			mu.Unlock()
			if full {
				return
			}
			data, err := s.fetchImageBytes(u)
			if err != nil || len(data) == 0 {
				return
			}
			// Raw image bytes join the shared subresource total before
			// anything decodes them: the pixel reservation bounds decoded
			// cost, but 256 individually-capped responses could still spend
			// gigabytes encoded without this.
			if !s.sub.take(len(data)) {
				return
			}
			// Vector backgrounds without intrinsic dimensions or ratio
			// rasterize after layout at their tile size (see Reflow):
			// decoding now could only guess a viewport. So do
			// preserveAspectRatio=none images, which never keep a ratio
			// and always tile the area. Stash the bounded bytes instead;
			// anything else decodes as before. URLs backing <img>
			// elements always decode immediately for layout.
			if bgOnly[u] && imgdec.IsSVG(data) {
				hasDims, hasRatio := imgdec.SVGIntrinsicKind(data)
				if (!hasDims && !hasRatio) || imgdec.SVGPreserveNone(data) {
					mu.Lock()
					if s.bgVectorSrc == nil {
						s.bgVectorSrc = make(map[string][]byte)
					}
					s.bgVectorSrc[u] = data
					if !hasDims && !hasRatio {
						if s.bgNoRatio == nil {
							s.bgNoRatio = make(map[string]bool)
						}
						s.bgNoRatio[u] = true
					}
					mu.Unlock()
					return
				}
			}
			cfg, err := imgdec.Probe(data)
			if err != nil {
				return
			}
			pixels := int64(cfg.Width) * int64(cfg.Height)
			if !reservation.reserve(pixels) {
				return
			}
			img, err := imgdec.Decode(data)
			if err != nil {
				reservation.release(pixels)
				return
			}
			mu.Lock()
			loaded++
			byURL[u] = img
			mu.Unlock()
		}()
	}
	wg.Wait()
	return byURL
}

// applyImages fans decoded pixels back out to the nodes that referenced them
// and records intrinsic sizes for layout.
func (s *Session) applyImages(refs []imgRef, byURL map[string]stdimage.Image) {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	for _, r := range refs {
		// Masks resolve before the vector-background deferral below: an
		// intrinsic-less mask SVG has no tile-size raster path yet, so it
		// stays unattached (painting unmasked) rather than joining one.
		if r.mask {
			if img, ok := byURL[r.abs]; ok {
				s.masks[r.id] = img
			}
			continue
		}
		if r.bg {
			if _, ok := s.bgVectorSrc[r.abs]; ok {
				// Intrinsic-less SVGs deferred for tile-size raster have
				// no border-image use; drop them instead of misattaching.
				if r.border {
					continue
				}
				if r.pseudo != nil {
					s.pseudoBgVectorRef[r.pseudo] = r.abs
				} else {
					s.bgVectorRef[r.id] = r.abs
				}
				continue
			}
		}
		img, ok := byURL[r.abs]
		if !ok {
			continue
		}
		if r.border {
			s.borderImages[r.id] = img
			continue
		}
		if r.bg {
			if r.pseudo != nil {
				s.pseudoBgImages[r.pseudo] = img
			} else {
				s.bgImages[r.id] = img
			}
			continue
		}
		if b := img.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
			s.natural[r.id] = layout.NaturalSize{W: float32(b.Dx()), H: float32(b.Dy())}
			s.images[r.id] = img
		}
	}
}

// attachVectorBackground rasterizes one vector background without intrinsic
// dimensions at its laid-out tile size and attaches it with the vector flag,
// so paint negotiates the same tile both sides of the boundary. The caller
// holds imgMu; admission clamps the tile before a pixel is allocated, and
// the tile joins the document pixel budget (releasing happens wholesale per
// Reflow, see the attach loop).
func (s *Session) attachVectorBackground(candidate *layout.Arena, i int, abs string) {
	data, ok := s.bgVectorSrc[abs]
	if !ok || len(data) == 0 {
		return
	}
	obj := &candidate.Objects[i]
	if obj.Style == nil {
		return
	}
	// The tile negotiates against the background-positioning area, exactly
	// like paint: anything else decodes a raster paint would slice apart.
	x0, y0, x1, y1 := obj.BgAreaRect(obj.Style.BackgroundOrigin)
	areaW, areaH := x1-x0, y1-y0
	if areaW <= 0 || areaH <= 0 {
		return
	}
	// Natural dimensions feed the standard negotiation branches (auto with
	// partial dims, explicit lengths); the vector branches ignore them.
	// A source deferring both sides has nothing to size from, so auto
	// tiles the area instead of the 300x150 fallback.
	var natW, natH float32
	if cfg, err := imgdec.Probe(data); err == nil {
		natW, natH = float32(cfg.Width), float32(cfg.Height)
	}
	if obj.Style.BackgroundSize == style.BgSizeAuto && imgdec.SVGDefersBothSides(data) {
		natW, natH = areaW, areaH
	}
	// Cover and contain scale by ratio: without full intrinsic dimensions
	// use the true viewBox ratio, not pixel-completed dimensions, so an
	// extreme ratio still sizes to ~zero (contain drops) or astronomic
	// (cover clamps to the area) instead of a rounded fabrication. Full
	// dimensions govern their own tile.
	if bs := obj.Style.BackgroundSize; bs == style.BgSizeCover || bs == style.BgSizeContain {
		if hasDims, _ := imgdec.SVGIntrinsicKind(data); !hasDims {
			if rw, rh, ok := imgdec.SVGRatio(data); ok {
				natW, natH = float32(rw), float32(rh)
			}
		}
	}
	tileW, tileH := paint.BGTileSize(obj.Style, areaW, areaH, natW, natH, s.bgNoRatio[abs])
	tw, th := int(tileW+0.5), int(tileH+0.5)
	if tw <= 0 || th <= 0 {
		return
	}
	if tw > imgdec.MaxImageDimension || th > imgdec.MaxImageDimension {
		// Only cover can overshoot admission (its tile covers the area by
		// scaling up): shrink the raster ratio-preserving instead of
		// dropping the background. Paint recomputes the same tile geometry
		// from the clamped raster's preserved ratio and upscales. Lengths
		// stay fail-closed so paint cannot negotiate a different tile.
		if obj.Style.BackgroundSize != style.BgSizeCover {
			return
		}
		scale := float64(imgdec.MaxImageDimension) / float64(tw)
		if s := float64(imgdec.MaxImageDimension) / float64(th); s < scale {
			scale = s
		}
		tw = int(float64(tw)*scale + 0.5)
		th = int(float64(th)*scale + 0.5)
		if tw <= 0 || th <= 0 {
			// An extreme ratio collapses a side to nothing even clamped:
			// rasterize the area itself, stretched. Paint places an auto
			// tile 1:1 at the same area.
			tw = int(areaW + 0.5)
			th = int(areaH + 0.5)
		}
		if tw <= 0 || th <= 0 || tw > imgdec.MaxImageDimension || th > imgdec.MaxImageDimension {
			return
		}
	}
	pixels := int64(tw) * int64(th)
	if s.imgReservation == nil || !s.imgReservation.reserve(pixels) {
		return
	}
	img, err := imgdec.DecodeSVGAt(data, tw, th)
	if err != nil {
		s.imgReservation.release(pixels)
		return
	}
	s.bgVectorUsed += pixels
	obj.BgImage = img
	obj.BgVector = true
	obj.BgNoRatio = s.bgNoRatio[abs]
}

// styleViewport is the frame the cascade resolves the viewport units against. A
// session created without a size leaves them to the parser's fallback.
func (s *Session) styleViewport() style.Viewport {
	return style.Viewport{W: s.viewportW, H: s.viewportH, DarkMode: s.darkMode}
}

// recordViewportWidth keeps the session's viewport in step with a layout pass
// that was handed a new width. A width that is not a usable size is ignored
// rather than baked into the viewport units, which would poison every length
// downstream.
func (s *Session) recordViewportWidth(w float32) {
	if finite(float64(w)) && w > 0 && w <= MaxGeometry {
		s.viewportW = w
	}
}

// Viewport returns the configured viewport width and height of the session.
func (s *Session) Viewport() (w, h float32) {
	return s.viewportW, s.viewportH
}

// Reflow runs layout only, retaining the document, computed style identities,
// and font metrics. The old arena is untouched unless the candidate validates.
// Like Paint, this method must be called by the session's single owner.
func (s *Session) Reflow(viewportW float32) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	// imgMu comes off before intersection dispatch below. An error return
	// earlier in the function still ends with it held, so the unlock is
	// conditional rather than a second defer.
	s.imgMu.Lock()
	imgLocked := true
	defer func() {
		if imgLocked {
			s.imgMu.Unlock()
		}
	}()
	if err := validateWidth(viewportW); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("missing session")
	}
	s.viewportW = viewportW
	if err := s.validateLayoutInput(); err != nil {
		return err
	}
	s.workCounts.layoutBuilds.Add(1)
	candidate := layout.Build(s.Doc, s.Styles, s.PseudoStyles)
	candidate.Metrics = s.metrics
	candidate.NaturalSizes = s.natural
	layout.Block(candidate, layout.ObjectID(1), viewportW, s.viewportH)
	if err := validateArena(candidate); err != nil {
		return err
	}
	layout.Inline(candidate, layout.ObjectID(1))
	s.applyImplicitPopoverAnchors(candidate)
	layout.Positioning(candidate, layout.ObjectID(1), viewportW, s.viewportH)
	if err := validateArena(candidate); err != nil {
		return err
	}
	// Hand each laid-out image box its decoded pixels so paint can draw it. The
	// arena is rebuilt every reflow, so this attachment has to run each time.
	// Lazy vector tiles release wholesale first: they re-rasterize per Reflow
	// while fetched pixels stay retained, so only the lazy share resets.
	if s.imgReservation != nil {
		s.imgReservation.release(s.bgVectorUsed)
		s.bgVectorUsed = 0
	}
	if s.images != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil {
				if img, ok := s.images[n.ID]; ok {
					candidate.Objects[i].Image = img
				}
				if img, ok := s.bgImages[n.ID]; ok {
					candidate.Objects[i].BgImage = img
				}
				if img, ok := s.pseudoBgImages[candidate.Objects[i].Style]; ok {
					candidate.Objects[i].BgImage = img
				}
				if img, ok := s.masks[n.ID]; ok {
					candidate.Objects[i].MaskImage = img
				}
				if img, ok := s.borderImages[n.ID]; ok {
					candidate.Objects[i].BorderImage = img
				}
				// Vector backgrounds without intrinsics rasterize here, at
				// the laid-out tile size, because only now is the tile
				// known. Skipped boxes (zero area, over-limit tiles, decode
				// failures) keep no image and paint nothing, like any
				// missing subresource.
				if abs, ok := s.bgVectorRef[n.ID]; ok {
					s.attachVectorBackground(candidate, i, abs)
				}
				if abs, ok := s.pseudoBgVectorRef[candidate.Objects[i].Style]; ok {
					s.attachVectorBackground(candidate, i, abs)
				}
			}
		}
	}
	// Hand each video box its current presentation state. The arena is rebuilt
	// here while frames arrive between reflows, so without this a style change
	// would blank a playing video until its next frame.
	if s.media != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil && n.Data == "video" {
				if c := s.media.controllerByID(n.ID); c != nil {
					candidate.Objects[i].Media = c.View()
				}
			}
		}
	} else {
		// No backend was installed, so nothing in this document can ever play.
		// The view says so rather than leaving the element looking like a video
		// that has not started yet, and the builder drops the control bar with
		// it: a bar that can never do anything is the worse of the two.
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil && n.Data == "video" {
				candidate.Objects[i].Media = paint.VideoView{
					State:       paint.VideoFailed,
					Message:     "playback unsupported",
					Unsupported: true,
				}
			}
		}
	}
	// Render canvas elements: replay recorded ops into a bitmap and attach it
	// as the box's Image so the paint pipeline draws it like any other image.
	if s.jsRT != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil && n.Data == "canvas" {
				ops := s.jsRT.CanvasOps(int(n.ID))
				if len(ops) > 0 {
					w, h := s.jsRT.CanvasDimensions(int(n.ID))
					if w > 0 && h > 0 {
						b := frame.NewBitmap(w, h)
						frameOps := make([]frame.CanvasOp, len(ops))
						for j, op := range ops {
							frameOps[j] = frame.CanvasOp{Method: op.Method, Args: op.Args}
						}
						frame.ReplayCanvasOps(b, frameOps, w, h)
						candidate.Objects[i].Image = b.AsRGBA()
						// Store the rendered pixels on the JS context so
						// getImageData can read actual pixel values.
						s.jsRT.SetCanvasBitmap(int(n.ID), b.RGBA, w, h)
					}
				}
			}
		}
	}
	// Update intersection observer geometry and check intersections.
	//
	// CheckIntersections runs the page's callbacks on this goroutine, and a
	// callback that assigns img.src re-enters the loader (LoadScriptImage ->
	// beginScriptImage), which takes imgMu. Go mutexes are not reentrant, so
	// the lock ends here: holding it across the dispatch deadlocked every page
	// whose observer loaded an image, and vneconomy.vn hung to its 120 s sweep
	// deadline with beginScriptImage parked in Lock.
	if s.jsRT != nil {
		s.imgMu.Unlock()
		imgLocked = false
		s.jsRT.SetViewportRect(0, 0, s.viewportW, s.viewportH)
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil {
				s.jsRT.SetElementRect(int(n.ID), candidate.Objects[i].X, candidate.Objects[i].Y, candidate.Objects[i].W, candidate.Objects[i].H)
			}
		}
		s.jsRT.CheckIntersections()
	}
	s.Arena = candidate
	s.layoutDirty.Store(false)
	// Selection ends index the old arena's word list; the rebuild just moved
	// every word, so the highlight would paint on the wrong boxes.
	s.selActive = false
	// Find marks hold old-arena word pointers, for the same reason.
	s.findMarks = nil
	s.findActive = 0
	return nil
}

// HitTestLink returns the href of the nearest ancestor <a> at the given
// document-coordinate point (CSS pixels), or empty string if no link is there.
func (s *Session) HitTestLink(x, y float32) string {
	if s.Arena == nil {
		return ""
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if cur.Style != nil && cur.Style.PointerEvents == "none" {
				break
			}
			if cur.Node != nil && cur.Node.DataAtom == dom.AtomA {
				if href := cur.Node.GetAttribute("href"); href != "" {
					return href
				}
			}
		}
	}
	return ""
}

// HitTestImage returns the src of the nearest <img> at the given document point.
func (s *Session) HitTestImage(x, y float32) string {
	if s.Arena == nil {
		return ""
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		if obj.Node != nil && obj.Node.DataAtom == dom.AtomImg {
			if src := resolveImgSource(obj.Node, s.viewportW); src != "" {
				return src
			}
		}
	}
	return ""
}

// resolveImgSource returns the effective image URL for an <img> element,
// evaluating any parent <picture> and <source> elements against the current viewport.
func resolveImgSource(n *dom.Node, viewportW float32) string {
	if n.Parent != nil && n.Parent.Data == "picture" {
		for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
			if c == n {
				break
			}
			if c.Element() && c.Data == "source" {
				if typ := c.GetAttribute("type"); typ != "" && !isSupportedImageType(typ) {
					continue
				}
				if media := c.GetAttribute("media"); media != "" {
					if !css.MatchMedia(media, viewportW) {
						continue
					}
				}
				if srcset := strings.TrimSpace(c.GetAttribute("srcset")); srcset != "" {
					fields := strings.Fields(srcset)
					if len(fields) > 0 {
						return fields[0]
					}
				}
			}
		}
	}
	return n.GetAttribute("src")
}

func isSupportedImageType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp", "image/svg+xml", "image/bmp":
		return true
	}
	return false
}

// ClickAt handles a click at the document-coordinate point. It finds an
// activatable control at the point, fires a "click" event, and performs the
// default action if the event was not prevented:
//   - checkbox/radio: toggle the control
//   - submit button: submit the form
//   - reset button: reset the form
//
// Returns true if an activatable control was found at the point.
func (s *Session) ClickAt(x, y float32) bool {
	hitNode := s.nodeAt(x, y)
	dismissed := s.dismissAutoPopoversExcept(hitNode)
	if hitNode != nil && !hitNode.Element() {
		for hitNode != nil && !hitNode.Element() {
			hitNode = hitNode.Parent
		}
	}
	if isRangeInput(hitNode) && !dom.IsFormControlDisabled(hitNode) {
		if s.jsRT != nil {
			s.jsRT.SetTransientUserActivation(true)
		}
		click := dom.NewTrustedEvent("click", true, true)
		if !dom.DispatchEvent(hitNode, click) {
			return true
		}
		s.opMu.Lock()
		changed := s.setRangeValueAtLocked(hitNode, x)
		rt := s.jsRT
		s.opMu.Unlock()
		if changed {
			if rt != nil {
				rt.DispatchTrustedInput(hitNode, nil, "", false)
			} else {
				event := dom.NewTrustedInputEvent("input", true, false, nil, "", false)
				dom.DispatchEvent(hitNode, &event.Event)
			}
			dom.DispatchEvent(hitNode, dom.NewTrustedEvent("change", true, false))
		}
		return true
	}

	n := s.activatableAt(x, y)
	if n == nil {
		if hitNode != nil {
			if s.jsRT != nil {
				s.jsRT.SetTransientUserActivation(true)
			}
			dom.DispatchEvent(hitNode, dom.NewTrustedEvent("click", true, true))
		}
		return dismissed
	}
	if s.jsRT != nil {
		s.jsRT.SetTransientUserActivation(true)
	}
	ev := dom.NewTrustedEvent("click", true, true)
	if !dom.DispatchEvent(n, ev) {
		return true // preventDefault was called
	}
	s.runClickDefault(n)
	return true
}

// toggleDetailsWithoutRuntime flips a details open state and dispatches the
// toggle transitions the script runtime would otherwise own, including closing
// the element's named group peers.
func (s *Session) toggleDetailsWithoutRuntime(n *dom.Node) {
	wasOpen := n.HasAttribute("open")
	if wasOpen {
		n.RemoveAttribute("open")
	} else {
		n.SetAttribute("open", "")
	}
	oldState, newState := "closed", "open"
	if wasOpen {
		oldState, newState = "open", "closed"
	}
	event := dom.NewTrustedToggleEvent("toggle", false, false, oldState, newState)
	dom.DispatchEvent(n, &event.Event)
	if wasOpen {
		return
	}
	for _, peer := range dom.CloseOtherNamedDetails(n) {
		peerEvent := dom.NewTrustedToggleEvent("toggle", false, false, "open", "closed")
		dom.DispatchEvent(peer, &peerEvent.Event)
	}
}

// runClickDefault performs the default action for an activated control:
// toggling checkbox/radio, or submitting/resetting the owning form for submit
// and reset controls. Shared by pointer ClickAt and keyboard Enter activation.
func (s *Session) runClickDefault(n *dom.Node) {
	typ := strings.ToLower(n.GetAttribute("type"))
	switch n.Data {
	case "input":
		if targetID := n.GetAttribute("popovertarget"); targetID != "" {
			action := strings.ToLower(n.GetAttribute("popovertargetaction"))
			s.handlePopoverTarget(targetID, action)
			return
		}
		switch typ {
		case "checkbox", "radio":
			s.ToggleControl(n)
		case "submit":
			if form := FindForm(n); form != nil {
				s.SubmitForm(form, n)
			}
		case "reset":
			if form := FindForm(n); form != nil {
				s.ResetForm(form)
			}
		}
	case "summary":
		for p := n.Parent; p != nil; p = p.Parent {
			if p.Element() && p.Data == "details" {
				if s.jsRT != nil {
					s.jsRT.SetDetailsOpen(p, !p.HasAttribute("open"), nil)
				} else {
					s.toggleDetailsWithoutRuntime(p)
				}
				break
			}
		}
	case "details":
		hasSummary := false
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && child.Data == "summary" {
				hasSummary = true
				break
			}
		}
		if !hasSummary {
			if s.jsRT != nil {
				s.jsRT.SetDetailsOpen(n, !n.HasAttribute("open"), nil)
			} else {
				s.toggleDetailsWithoutRuntime(n)
			}
		}
	case "button":
		if targetID := n.GetAttribute("commandfor"); targetID != "" {
			action := strings.ToLower(strings.TrimSpace(n.GetAttribute("command")))
			if action == "toggle" && s.Doc != nil {
				if target := s.Doc.ElementByID(targetID); target != nil && target.Data == "details" {
					if s.jsRT != nil {
						s.jsRT.SetDetailsOpen(target, !target.HasAttribute("open"), n)
					} else {
						wasOpen := target.HasAttribute("open")
						if wasOpen {
							target.RemoveAttribute("open")
						} else {
							target.SetAttribute("open", "")
						}
						oldState, newState := "closed", "open"
						if wasOpen {
							oldState, newState = "open", "closed"
						}
						event := dom.NewTrustedToggleEvent("toggle", false, false, oldState, newState)
						event.Source = n
						dom.DispatchEvent(target, &event.Event)
					}
					return
				}
			}
			switch action {
			case "show-popover":
				s.handlePopoverTarget(targetID, "show")
				return
			case "hide-popover":
				s.handlePopoverTarget(targetID, "hide")
				return
			case "toggle-popover":
				s.handlePopoverTarget(targetID, "toggle")
				return
			}
		}
		if targetID := n.GetAttribute("popovertarget"); targetID != "" {
			action := strings.ToLower(n.GetAttribute("popovertargetaction"))
			s.handlePopoverTarget(targetID, action)
			return
		}
		btnType := strings.ToLower(n.GetAttribute("type"))
		switch btnType {
		case "submit", "":
			if form := FindForm(n); form != nil {
				s.SubmitForm(form, n)
			}
		case "reset":
			if form := FindForm(n); form != nil {
				s.ResetForm(form)
			}
		}
	}
}

// activatableAt walks the arena in reverse paint order for the box containing
// the point, then walks that box's ancestors for an activatable element.
func (s *Session) activatableAt(x, y float32) *dom.Node {
	if s.Arena == nil {
		return nil
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if cur.Style != nil && cur.Style.PointerEvents == "none" {
				break
			}
			if IsActivatable(cur.Node) {
				return cur.Node
			}
		}
	}
	return nil
}

// FormSubmission represents a form ready to be dispatched by the browser host.
type FormSubmission struct {
	Action  string
	Method  string
	Data    []FormData
	Encoded string
}

// SubmitForm fires a cancelable "submit" event on the form, then collects form
// data and encodes it. The submission details are stored on the session for
// the host to retrieve via PopSubmittedForm.
func (s *Session) SubmitForm(form, submitter *dom.Node) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.submitFormLocked(form, submitter)
}

func (s *Session) queueDialogClose(node *dom.Node) {
	if node == nil {
		return
	}
	if s.jsRT == nil {
		dom.DispatchEvent(node, dom.NewTrustedEvent("close", false, false))
		return
	}
	s.jsRT.QueueTask(func() {
		dom.DispatchEvent(node, dom.NewTrustedEvent("close", false, false))
	})
}

func (s *Session) submitFormLocked(form, submitter *dom.Node) {
	if form == nil {
		return
	}
	invalid := ValidateForm(form)
	if len(invalid) > 0 {
		for _, control := range invalid {
			dom.DispatchEvent(control, dom.NewEvent("invalid", false, false))
		}
		s.setFocusedNodeLocked(invalid[0])
		return
	}
	ev := dom.NewEvent("submit", true, true)
	if !dom.DispatchEvent(form, ev) {
		return // preventDefault was called
	}
	data := CollectFormData(form, submitter)
	method := FormMethod(form)
	if strings.EqualFold(method, "dialog") {
		for cur := form.Parent; cur != nil; cur = cur.Parent {
			if cur.Element() && cur.Data == "dialog" {
				cur.RemoveAttribute("open")
				cur.IsModal = false
				if s.jsRT != nil {
					s.restoreDialogFocusLocked(cur)
				}
				s.queueDialogClose(cur)
				return
			}
		}
		return
	}
	action := FormAction(form)
	encoded := EncodeFormURLEncoded(data)

	s.submittedForm = &FormSubmission{
		Action:  action,
		Method:  method,
		Data:    data,
		Encoded: encoded,
	}
}

// PopSubmittedForm returns and clears the pending form submission, if any.
func (s *Session) PopSubmittedForm() (*FormSubmission, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.submittedForm == nil {
		return nil, false
	}
	sub := s.submittedForm
	s.submittedForm = nil
	return sub, true
}

// ResetForm resets the form controls to their default values and fires a "reset" event.
func (s *Session) ResetForm(form *dom.Node) {
	if form == nil {
		return
	}
	ev := dom.NewTrustedEvent("reset", false, true)
	if !dom.DispatchEvent(form, ev) {
		return // preventDefault was called
	}

	var resetNode func(n *dom.Node)
	resetNode = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() {
			switch n.Data {
			case "input":
				typ := strings.ToLower(n.GetAttribute("type"))
				switch typ {
				case "checkbox", "radio":
					n.ResetInputChecked()
					if typ == "radio" && n.InputChecked() {
						reconcileFormRadioGroups(form)
					}
				case "submit", "reset", "button", "image":
					// No-op
				default:
					n.ResetInputControlValue()
					if s.jsRT != nil {
						s.jsRT.RefreshInputPatternMismatch(n)
					}
				}
			case "textarea":
				n.ResetInputControlValue()
			case "select":
				n.SelectNoSelection = false
				for _, option := range dom.SelectOptions(n) {
					option.ResetOptionSelectedness()
				}
				dom.NormalizeSelectSelectedness(n)
			case "output":
				if s.jsRT != nil {
					s.jsRT.ResetOutputElement(n)
				} else {
					value := n.ResetOutputControlValue()
					for child := n.FirstChild; child != nil; {
						next := child.NextSibling
						n.RemoveChild(child)
						child = next
					}
					if value != "" && n.Doc != nil {
						n.AppendChild(n.Doc.NewText(value))
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			resetNode(c)
		}
	}
	resetNode(form)
}

func reconcileFormRadioGroups(form *dom.Node) {
	if form == nil {
		return
	}
	checked := make(map[string]*dom.Node)
	form.Walk(func(node *dom.Node) bool {
		if !node.Element() || node.Data != "input" || !strings.EqualFold(node.GetAttribute("type"), "radio") || !node.InputChecked() {
			return true
		}
		name := node.GetAttribute("name")
		if name == "" {
			return true
		}
		if earlier := checked[name]; earlier != nil {
			earlier.SetInputChecked(false, true)
		}
		checked[name] = node
		return true
	})
}

// Match is one find result: the document-space rect of an occurrence of the
// query, in CSS pixels like every other engine coordinate.
type Match struct {
	X0, Y0, X1, Y1 float32
}

// maxFindMatches bounds the work a hostile page can make Find do.
const maxFindMatches = 1000

// findMark ties one Match rect to the word box whose paint emits the
// highlight. match is the index into the Find result, so the active match can
// switch color without re-running the search.
type findMark struct {
	obj   *layout.Object
	rect  Match
	match int
}

// Find returns the rects of every visible occurrence of query, in tree order,
// case-insensitively. Consecutive word boxes of a run are joined with single
// spaces, so a query may span word boundaries and line breaks but not element
// boundaries. The matches are also recorded for Paint to underlay with the
// highlight until ClearFind or the next Find.
func (s *Session) Find(query string) []Match {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.findMarks = nil
	s.findActive = 0
	if s.Arena == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	q := strings.ToLower(query)

	objs := s.Arena.Objects
	var matches []Match
	var marks []findMark
	var run []*layout.Object

	flushRun := func() {
		if len(matches) >= maxFindMatches || len(run) == 0 {
			run = run[:0]
			return
		}
		var words []*layout.Object
		for _, o := range run {
			if strings.TrimSpace(o.Node.DataContent) != "" {
				words = append(words, o)
			}
		}
		run = run[:0]
		if len(words) == 0 {
			return
		}
		texts := make([]string, len(words))
		off := make([]int, len(words))
		pos := 0
		for i, o := range words {
			texts[i] = strings.ToLower(o.Node.DataContent)
			off[i] = pos
			pos += len(texts[i]) + 1
		}
		joined := strings.Join(texts, " ")
		for at := 0; len(matches) < maxFindMatches; {
			idx := strings.Index(joined[at:], q)
			if idx < 0 {
				break
			}
			start := at + idx
			rect, first := matchRect(words, off, start, start+len(q))
			marks = append(marks, findMark{obj: words[first], rect: rect, match: len(matches)})
			matches = append(matches, rect)
			at = start + 1
		}
	}

	var walk func(layout.ObjectID)
	walk = func(id layout.ObjectID) {
		if id == 0 || int(id) >= len(objs) {
			return
		}
		obj := &objs[id]
		if obj.Node != nil && obj.Node.Type == dom.NodeText {
			if x0, y0, x1, y1 := obj.BorderRect(); x0 < x1 && y0 < y1 {
				if len(run) > 0 && run[len(run)-1].Parent != obj.Parent {
					flushRun()
				}
				run = append(run, obj)
			}
		}
		for k := obj.FirstKid; k != 0; k = objs[k].NextSibling {
			walk(k)
		}
	}
	walk(1)
	flushRun()
	s.findMarks = marks
	return matches
}

// SetFindActive selects which match the most recent Find paints in the active
// color. An out-of-range index leaves every match in the inactive color.
func (s *Session) SetFindActive(index int) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if index < 0 || index >= len(s.findMarks) {
		s.findActive = -1
		return
	}
	s.findActive = index
}

// ClearFind drops the highlight state left by Find.
func (s *Session) ClearFind() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.findMarks = nil
	s.findActive = 0
}

// SelectionBounds returns the union of selected word boxes in document
// pixels, or false with no selection. Repaints damage exactly this (plus
// the previous bounds across an update) instead of the whole document.
func (s *Session) SelectionBounds() (frame.Rect, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	words, from, to, ok := s.selCoverage()
	if !ok || len(words) == 0 {
		return frame.Rect{}, false
	}
	if from.word < 0 {
		from.word = 0
	}
	if to.word >= len(words) {
		to.word = len(words) - 1
	}
	if from.word > to.word {
		return frame.Rect{}, false
	}
	first := true
	var x0, y0, x1, y1 float32
	for i := from.word; i <= to.word; i++ {
		cx0, cy0, cx1, cy1 := words[i].ContentRect()
		if first || cx0 < x0 {
			x0 = cx0
		}
		if first || cy0 < y0 {
			y0 = cy0
		}
		if first || cx1 > x1 {
			x1 = cx1
		}
		if first || cy1 > y1 {
			y1 = cy1
		}
		first = false
	}
	if x1 <= x0 || y1 <= y0 {
		return frame.Rect{}, false
	}
	// Outward rounding: damage must cover every touched pixel.
	return frame.Rect4(int32(math.Floor(float64(x0))), int32(math.Floor(float64(y0))),
		int32(math.Ceil(float64(x1))), int32(math.Ceil(float64(y1)))), true
}

// ScrollToID scrolls to the first element with the given ID in the document,
// returning its Y coordinate (CSS pixels). If no matching element is found,
// returns 0. Search order is document order for first-match semantics.
func (s *Session) ScrollToID(targetID string) float32 {
	if s.Arena == nil || targetID == "" {
		return 0
	}

	objs := s.Arena.Objects
	var walk func(layout.ObjectID)
	foundY := float32(0)

	walk = func(objID layout.ObjectID) {
		if objID == 0 || int(objID) >= len(objs) {
			return
		}
		obj := &objs[objID]

		// Check this node for matching ID attribute
		if obj.Node != nil {
			for _, attr := range obj.Node.Attr {
				if strings.EqualFold(attr.Name, "id") && attr.Value == targetID {
					// Found target: get its top position
					_, y, _, _ := obj.BorderRect()
					if y > 0 {
						foundY = y
					}
					return
				}
			}
		}

		// Continue walking children (first match wins in document order)
		if foundY == 0 {
			for k := obj.FirstKid; k != 0 && foundY == 0; k = objs[k].NextSibling {
				walk(k)
			}
		}
	}

	walk(1)
	return foundY
}

// matchRect maps a match spanning joined-string offsets [start, end) back onto
// the word boxes it covers. Edges inside a box are interpolated proportionally
// to character position, which is exact for full-word matches.
func matchRect(words []*layout.Object, off []int, start, end int) (Match, int) {
	first, last := 0, len(words)-1
	for i := range words {
		if off[i] <= start {
			first = i
		}
		if off[i] < end {
			last = i
		}
	}
	x0, y0, x1, y1 := words[first].BorderRect()
	if first == last {
		if w := len(words[first].Node.DataContent); w > 0 {
			sx := x0 + (x1-x0)*float32(start-off[first])/float32(w)
			ex := x0 + (x1-x0)*float32(end-off[first])/float32(w)
			return Match{X0: sx, Y0: y0, X1: ex, Y1: y1}, first
		}
		return Match{X0: x0, Y0: y0, X1: x1, Y1: y1}, first
	}
	lx0, ly0, lx1, ly1 := words[last].BorderRect()
	if w := len(words[first].Node.DataContent); w > 0 {
		x0 = x0 + (x1-x0)*float32(start-off[first])/float32(w)
	}
	if w := len(words[last].Node.DataContent); w > 0 {
		x1 = lx0 + (lx1-lx0)*float32(end-off[last])/float32(w)
	} else {
		x1 = lx1
	}
	top, bot := y0, y1
	if ly0 < top {
		top = ly0
	}
	if ly1 > bot {
		bot = ly1
	}
	return Match{x0, top, x1, bot}, first
}

func parentObj(objs []layout.Object, o *layout.Object) *layout.Object {
	if o.Parent == 0 || int(o.Parent) >= len(objs) {
		return nil
	}
	return &objs[o.Parent]
}

// runScripts walks the DOM for <script> elements in document order and
// executes each one through the JS runtime. A script error is isolated, not
// fatal: a page with a broken script still renders, matching browser behavior
// where one bad ad tag does not take down the page. Each error is recorded on
// the session and reads back through ScriptErrors / ScriptErrorCount, so a
// document that paints nothing can be told apart from one that never loaded.
func (s *Session) runScripts() {
	if s.jsRT == nil || s.Doc == nil {
		return
	}
	root := s.Doc.HTML
	if root == nil {
		root = &s.Doc.Node
	}
	// Collect scripts in document order. Inline sources run from the tree;
	// external bundles reserve a slot, fetch concurrently below, and run
	// from their slot so execution order matches document order even though
	// the bytes arrive out of order. External classic defer scripts run after
	// parser-inserted classic scripts.
	type scriptItem struct {
		text        string // inline source, or fetched bundle once filled
		ext         string // absolute bundle URL when external, else ""
		name        string // sourceURL reported in stack traces
		deferScript bool
		node        *dom.Node
	}
	var items []scriptItem
	extLeft := maxExternalScripts - s.parserExternalCount
	var collect func(n *dom.Node)
	collect = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.Data == "script" && isClassicScript(n) && !s.parserExecutedScripts[n] &&
			(n.Namespace != dom.NSSVG || n.SVGScriptReady) {
			if src := scriptSrc(n); src != "" {
				// A src tag's inline text is ignored, per HTML.
				if strings.HasPrefix(strings.ToLower(src), "data:") {
					if text, ok := decodeDataScriptURL(src); ok {
						items = append(items, scriptItem{text: text, name: src, deferScript: n.HasAttribute("defer") && !n.HasAttribute("async"), node: n})
					}
				} else if s.scriptFetch != nil && extLeft > 0 {
					if abs, ok := resolveSubresourceURL(s.scriptBase, src); ok {
						extLeft--
						items = append(items, scriptItem{ext: abs, name: abs, deferScript: n.HasAttribute("defer") && !n.HasAttribute("async"), node: n})
					}
				}
			} else if text := n.TextContent(); text != "" {
				items = append(items, scriptItem{text: text, name: "inline", node: n})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(root)

	// External bundles fetch through a bounded pool, the same shape as
	// linked stylesheets: sequential fetching stalls the document behind
	// the first slow host. Failures leave the slot empty and execution
	// skips it, so one dead bundle never stops the page.
	deadline := time.Now().Add(ScriptFetchBudget)
	const scriptWorkers = 8
	sem := make(chan struct{}, scriptWorkers)
	var wg sync.WaitGroup
	for i, it := range items {
		if it.ext == "" || time.Now().After(deadline) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, abs string) {
			defer wg.Done()
			defer func() { <-sem }()
			if data, err := s.scriptFetch(s.scriptBase, abs); err == nil && len(data) > 0 {
				items[i].text = string(data)
			}
		}(i, it.ext)
	}
	wg.Wait()

	for _, deferred := range []bool{false, true} {
		for _, it := range items {
			if it.text == "" || it.deferScript != deferred {
				continue
			}
			// A script error stays isolated - the rest of the document still runs
			// and still renders, which is what a browser does with one bad ad tag -
			// but it is no longer discarded.
			var err error
			if it.deferScript {
				err = s.jsRT.RunParserDeferScript(it.text, it.name)
			} else {
				err = s.jsRT.RunParserScript(it.text, it.name)
			}
			if err != nil {
				s.recordScriptError(err)
			}
		}
	}
	s.runModuleScripts(s.jsRT, root, s.scriptBase, extLeft, deadline)
	s.runUserScripts(userscript.RunAtDocumentEnd)
}

// decodeDataScriptURL decodes a bounded data: URL for a classic external
// script. The URL is size-checked before percent or base64 decoding, then the
// decoded source is checked against the normal per-script admission limit.
func decodeDataScriptURL(raw string) (string, bool) {
	if len(raw) > js.MaxScriptBytes*3+8192 {
		return "", false
	}
	scheme, _, ok := strings.Cut(raw, ":")
	if !ok || !strings.EqualFold(scheme, "data") {
		return "", false
	}
	metadata, payload, ok := strings.Cut(raw[len(scheme)+1:], ",")
	if !ok || len(metadata) > 1024 {
		return "", false
	}
	base64Encoded := strings.HasSuffix(strings.ToLower(metadata), ";base64")
	if base64Encoded {
		metadata = metadata[:len(metadata)-len(";base64")]
	}
	mediaType := "text/plain"
	if metadata != "" {
		parsedType, _, err := mime.ParseMediaType(metadata)
		if err != nil {
			return "", false
		}
		mediaType = strings.ToLower(parsedType)
	}
	switch mediaType {
	case "application/ecmascript", "application/javascript", "application/x-ecmascript", "application/x-javascript",
		"text/ecmascript", "text/javascript", "text/javascript1.0", "text/javascript1.1", "text/javascript1.2", "text/javascript1.3", "text/javascript1.4", "text/javascript1.5", "text/jscript", "text/livescript":
	default:
		return "", false
	}
	decodedPayload, err := url.PathUnescape(payload)
	if err != nil {
		return "", false
	}
	source := []byte(decodedPayload)
	if base64Encoded {
		source, err = base64.StdEncoding.DecodeString(decodedPayload)
		if err != nil {
			return "", false
		}
	}
	if len(source) > js.MaxScriptBytes {
		return "", false
	}
	return string(source), true
}

// runUserScripts runs any configured user scripts matching the target lifecycle stage.
func (s *Session) runUserScripts(stage userscript.RunAt) {
	if s.jsRT == nil || len(s.userScripts) == 0 {
		return
	}
	for _, us := range s.userScripts {
		if !us.Enabled {
			continue
		}
		if us.RunAt != stage {
			if !(stage == userscript.RunAtDocumentEnd && us.RunAt == userscript.RunAtDocumentIdle) {
				continue
			}
		}
		if us.Code != "" {
			name := "userscript:" + us.Name
			if err := s.jsRT.Run(us.Code, name); err != nil {
				s.recordScriptError(err)
			}
		}
	}
}

// scriptSrc returns the src attribute of a script tag, or "" when inline.
func scriptSrc(n *dom.Node) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Name, "src") {
			return strings.TrimSpace(a.Value)
		}
	}
	if n.Namespace == dom.NSSVG {
		if href := strings.TrimSpace(n.GetAttribute("href")); href != "" {
			return href
		}
		return strings.TrimSpace(n.GetAttribute("xlink:href"))
	}
	return ""
}

// isClassicScript reports whether a script tag holds executable classic
// JavaScript: no type (or a JavaScript MIME type) runs, while modules and
// non-JS payloads (JSON-LD, templates) never do. nomodule scripts run:
// without module support this engine is exactly the fallback audience.
func isClassicScript(n *dom.Node) bool {
	for _, a := range n.Attr {
		if !strings.EqualFold(a.Name, "type") {
			continue
		}
		t := strings.ToLower(strings.TrimSpace(a.Value))
		switch t {
		case "", "text/javascript", "application/javascript",
			"application/ecmascript", "application/x-ecmascript",
			"application/x-javascript", "text/ecmascript",
			"text/jscript", "text/livescript":
			return true
		default:
			return false
		}
	}
	return true
}

// Close releases resources held by the session. If a JS runtime was supplied,
// it is closed so the realm is torn down; any media elements stop and release
// their downloaded files, because a document that is going away must not leave
// audio playing behind it.
func (s *Session) Close() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	for n := s.active; n != nil && n.Element(); n = n.Parent {
		css.SetActive(n, false)
	}
	for n := s.hovered; n != nil && n.Element(); n = n.Parent {
		css.SetHover(n, false)
	}
	if s.focus != nil {
		css.SetFocus(s.focus, false)
		css.SetFocusVisible(s.focus, false)
	}
	s.active = nil
	s.hovered = nil
	s.focus = nil
	s.focusState.Store(nil)
	if s.media != nil {
		s.media.closeAll()
	}
	if s.jsRT != nil {
		_ = s.jsRT.Close()
	}
}

// PumpMedia advances every media controller one step and returns the
// document-space rectangles whose painting changed. It applies whatever the
// download workers produced, polls the backends for new frames and positions,
// attaches the results to the laid-out video boxes, closes the elements that left
// the document, and hands any transitions to the JavaScript runtime.
//
// It never reflows. A decoded frame is not a style change, and paying a layout
// pass per video frame would put the document's cost on the display's clock
// instead of on the page's own edits. Callers repaint the returned rectangles and
// nothing else; an empty result means the tick was worth nothing.
func (s *Session) PumpMedia() []frame.RectF {
	if s.media == nil {
		return nil
	}
	s.opMu.Lock()
	if s.Doc == nil {
		s.opMu.Unlock()
		return nil
	}
	dmg := s.media.pump(&s.Doc.Node, s.Arena)
	// Drained under the lock, dispatched outside it: running a page's listener is
	// user code, and user code must not run while the pipeline state is held.
	var dispatch []mediaDispatch
	if s.jsRT != nil {
		dispatch = s.media.drainEvents()
	}
	rt := s.jsRT
	s.opMu.Unlock()

	for _, d := range dispatch {
		rt.DispatchMediaEvents(d.node, d.events)
	}
	return dmg
}

// JSTitle returns the document title as seen by the JS runtime after script
// execution. If no runtime was supplied, it returns the empty string.
func (s *Session) JSTitle() string {
	if s.jsRT == nil {
		return ""
	}
	return s.jsRT.Title()
}

// PumpJS runs one event-loop turn for the document: timer, fetch, worker and
// RAF callbacks fire, then the pipeline reflows if the turn changed the DOM.
// It reports whether the caller must repaint for a DOM or canvas change.
//
// State-only callbacks do not repaint. While deferred font/image loads are in
// flight the DOM reflow is skipped (their tables attach outside opMu by
// design) and retried next tick; timers still fire, so work is delayed.
func (s *Session) PumpJS(viewportW float32) bool {
	if s.jsRT == nil {
		return false
	}
	canvasBefore := s.jsRT.CanvasVisualStamp()
	s.jsRT.Tick()
	if s.jsRT.HasPendingRAF() {
		s.jsRT.TickRAF(time.Now())
	}
	// Attribute changes made by timer or message callbacks can queue new
	// iframe navigations after the document's initial load. Process those
	// navigations as part of the event-loop pump so their child scripts and
	// load events can run instead of waiting forever for startup lifecycle.
	s.jsRT.FlushIFrameLoads()
	s.fixupDisabledFocus()
	canvasChanged := canvasBefore != s.jsRT.CanvasVisualStamp()
	// Script image results recorded by the pixel sink during Tick are
	// reconciled after every callback of the turn has run, so the last src
	// write wins. A turn that still owes a layout from an earlier postponed
	// turn reports layoutPending even when nothing changed this turn.
	layoutPending := s.drainScriptImages()
	// Mutation state is captured before Reset: it decides between a full
	// re-resolve and skipping straight to repaint below.
	mutated := s.focusStylePending
	if s.inv != nil {
		mutated = mutated || !s.inv.Empty()
		s.inv.Reset()
	}
	if !mutated && !layoutPending {
		// Timers/RAF can contain state-only work and even a continuously
		// rescheduled no-op animation loop. Every DOM write notifies the sink;
		// canvas operations are the other observable output in this runtime.
		// Avoid full frame commits when neither changed.
		return canvasChanged
	}
	if atomic.LoadInt32(&s.deferredActive) > 0 {
		s.focusStylePending = true
		// The layout-pending bit stays set: the next tick must still realize
		// the attached pixels once the deferred tables have settled.
		return false
	}
	s.focusStylePending = false
	// Re-resolve before reflowing: scripts may have added nodes or classes
	// the load-time cascade never saw, and unstyled nodes are skipped by
	// layout. Stored sheets (linked and imported, in order) resolve exactly
	// as at load. Reflow takes opMu itself; PumpJS composes locked
	// primitives and never holds it, so reflow cannot deadlock against a
	// concurrent Close.
	layouts := s.workCounts.layoutBuilds.Load()
	if err := s.RestyleAndReflow(viewportW); err != nil {
		return false
	}
	if layoutPending && s.workCounts.layoutBuilds.Load() == layouts {
		// RestyleAndReflow can refresh computed styles without rebuilding the
		// arena when the cascade is unchanged. That is not enough here: the
		// attached pixels changed s.natural, which only a real layout reads.
		// Build once because the restyle path did not.
		if err := s.Reflow(viewportW); err != nil {
			return false
		}
	}
	if layoutPending {
		s.clearScriptImgsLayoutPending()
	}
	return true
}

// RestyleAndReflow resolves the document's current CSS state and rebuilds its
// layout when a layout input changed. For geometry-neutral style changes it
// refreshes arena style pointers and leaves the existing layout in place. It is
// used when an interaction changes selector state such as :checked, :focus, or
// :placeholder-shown without a script DOM mutation.
func (s *Session) RestyleAndReflow(viewportW float32) error {
	if s == nil {
		return nil
	}
	s.opMu.Lock()
	s.clearDetachedFocusLocked()
	s.workCounts.styleResolves.Add(1)
	if s.styleResolver == nil {
		s.styleResolver = style.NewResolver(s.sheets)
	}
	if incremental, ok := s.styleResolver.ResolveInteractiveStates(s.Doc, s.styleViewport(), s.customFonts, s.Styles, s.PseudoStyles); ok {
		s.workCounts.styleNodeVisits.Add(uint64(incremental.Visited))
		layoutChanged := interactiveLayoutChanged(incremental, s.Styles, s.PseudoStyles)
		if !layoutChanged && s.Arena.RefreshComputedStylesForNodes(s.Styles, incremental.Nodes) {
			s.opMu.Unlock()
			return nil
		}
		s.opMu.Unlock()
		return s.Reflow(viewportW)
	}
	styles := s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
	pseudoStyles := s.styleResolver.ResolvePseudoElements(s.Doc, styles, s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(styles)))
	if sameLayoutStyles(s.Styles, styles) && sameLayoutPseudoStyles(s.PseudoStyles, pseudoStyles) && s.Arena.RefreshComputedStyles(styles) {
		s.Styles = styles
		s.PseudoStyles = pseudoStyles
		s.opMu.Unlock()
		return nil
	}
	s.Styles = styles
	s.PseudoStyles = pseudoStyles
	s.opMu.Unlock()
	return s.Reflow(viewportW)
}

// HasPseudoClassRule reports whether an author stylesheet contains a selector
// that depends on pseudo. Interaction code uses it to avoid a full document
// style and layout pass when changing a state that no page rule observes.
func (s *Session) HasPseudoClassRule(pseudo string) bool {
	if s == nil || pseudo == "" {
		return false
	}
	needle := ":" + strings.ToLower(strings.TrimSpace(pseudo))
	s.opMu.Lock()
	defer s.opMu.Unlock()
	for _, sheet := range s.sheets {
		if sheet == nil {
			continue
		}
		for _, rule := range sheet.Rules {
			for _, selector := range rule.SelectorStrs {
				if strings.Contains(strings.ToLower(selector), needle) {
					return true
				}
			}
		}
	}
	return false
}

// JSMutationSink returns a function suitable for js.Options.OnMutation that
// records DOM mutations in the session's invalidation batch. The caller passes
// it to the JS runtime at construction time; when a script mutates the DOM
// (setAttribute, appendChild, innerHTML, etc.), the runtime fires the callback
// and the engine learns it must re-style and re-layout.
//
// If no invalidation was supplied via WithInvalidation, the returned function
// is a no-op so a session without invalidation tracking still works.
func (s *Session) JSMutationSink() func() {
	return func() {
		if s.inv != nil {
			s.inv.Add(DOMMutation, frame.RectF{}, 0)
		}
	}
}

// JSLiveNodeCount reports how many nodes the script admission budget counts as
// live in this document. It exposes the budget's counter for tests and
// diagnostics so they can assert a hostile script never pushed the tree past
// MaxDocumentNodes. It returns 0 when no JS runtime was supplied, since the
// budget is only built for a scripted session.
func (s *Session) JSLiveNodeCount() int {
	if s == nil || s.jsNodeBudget == nil {
		return 0
	}
	return s.jsNodeBudget.liveCount()
}

// SubresourceBytesUsed reports the raw subresource bytes this document has
// charged to its shared budget across linked sheets, @imports, fonts, images
// and JS fetch/XHR responses. It exposes the pool the per-family caps all draw
// on, so a test can assert a fetch was accounted rather than slipping past the
// budget. It returns 0 when the session has no budget.
func (s *Session) SubresourceBytesUsed() int64 {
	if s == nil || s.sub == nil {
		return 0
	}
	s.sub.mu.Lock()
	defer s.sub.mu.Unlock()
	return s.sub.used
}

// HasPendingDOMMutation reports whether a synchronous script callback has
// changed the document since the last PumpJS. Input handlers run inside Edit;
// when one mutates the DOM, the pump will restyle and reflow the resulting
// state. The caller can avoid doing a native reflow immediately before that
// full mutation pass. Invalidation is single-owner and this query must be made
// on the same thread that dispatches document events and calls PumpJS.
func (s *Session) HasPendingDOMMutation() bool {
	return s != nil && s.inv != nil && s.inv.domMutationPending
}

// CanDeferControlEditReflow reports whether the next PumpJS turn can perform
// the style and layout work for a synchronous input-handler mutation. While
// deferred resources are active PumpJS intentionally postpones that work, so
// the caller must keep its immediate reflow instead of leaving stale geometry
// on screen.
func (s *Session) CanDeferControlEditReflow() bool {
	return s.HasPendingDOMMutation() && atomic.LoadInt32(&s.deferredActive) == 0
}

// FocusedInputValueDoesNotAffectLayout reports whether the focused control is
// a single-line text input whose intrinsic box size is independent of its
// current value. Paint reads that value directly from the DOM node, so a
// repaint is sufficient unless an input handler queued a DOM mutation (which
// CanDeferControlEditReflow handles separately). Textareas and button-like
// inputs are deliberately excluded: their content can participate in layout.
func (s *Session) FocusedInputValueDoesNotAffectLayout() bool {
	if s == nil {
		return false
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.focus == nil || s.focus.DataAtom != dom.AtomInput {
		return false
	}
	switch strings.ToLower(s.focus.GetAttribute("type")) {
	case "", "text", "password", "email", "tel", "url", "search", "number":
		return true
	default:
		return false
	}
}

// ActiveElement returns the focused node without taking the pipeline mutex so
// scripts can read document.activeElement from inside focus/blur handlers.
func (s *Session) ActiveElement() *dom.Node {
	if s == nil {
		return nil
	}
	if n := s.focusState.Load(); n != nil {
		return n
	}
	if s.Doc == nil {
		return nil
	}
	if s.Doc.Body != nil {
		return s.Doc.Body
	}
	return s.Doc.HTML
}

// TickAnimations advances all active animations to the given time.
// Returns true if any animation is still active (needs another frame).
func (s *Session) TickAnimations(now time.Time) bool {
	if s.Animations == nil {
		return false
	}
	active := s.Animations.Update(now)
	if !active {
		s.Animations.RemoveCompleted()
	}
	return active
}

// AnimationsActive reports whether any @keyframes animation is in progress.
// This is the method the surface loop checks to decide whether to keep
// producing frames.
func (s *Session) AnimationsActive() bool {
	if s.Animations == nil {
		return false
	}
	return s.Animations.HasActive()
}

// startAnimations scans computed styles for animation declarations and
// registers them with the AnimationController. Keyframes are looked up from
// the session's stylesheets.
func (s *Session) startAnimations() {
	if s.Styles == nil {
		return
	}
	// Collect keyframes from all stylesheets.
	kfMap := make(map[string][]css.KeyframeStop)
	for _, sheet := range s.sheets {
		for _, kf := range sheet.Keyframes {
			kfMap[kf.Name] = kf.Stops
		}
	}
	if len(kfMap) == 0 {
		return
	}
	// Check if any style has an animation-name that matches a keyframes rule.
	hasAnimations := false
	for _, cs := range s.Styles {
		if cs.AnimationName != "" {
			if _, ok := kfMap[cs.AnimationName]; ok {
				hasAnimations = true
				break
			}
		}
	}
	if !hasAnimations {
		return
	}
	if s.Animations == nil {
		s.Animations = &AnimationController{}
	}
	now := time.Now()
	for id, cs := range s.Styles {
		if cs.AnimationName == "" {
			continue
		}
		stops, ok := kfMap[cs.AnimationName]
		if !ok {
			continue
		}
		s.Animations.Add(
			id,
			cs.AnimationName,
			stops,
			cs.AnimationDuration,
			cs.AnimationDelay,
			cs.AnimationTiming,
			cs.AnimationIterCount,
			css.AnimationDirection(cs.AnimationDirection),
			css.AnimationFillMode(cs.AnimationFillMode),
			now,
		)
	}
}

// Stylesheets returns a slice of parsed stylesheets applied to this session.
func (s *Session) Stylesheets() []*css.Stylesheet {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	out := make([]*css.Stylesheet, len(s.sheets))
	copy(out, s.sheets)
	return out
}

// DOMStats computes the total node count and maximum depth of the session's DOM tree.
func (s *Session) DOMStats() (nodeCount int, maxDepth int) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Doc == nil {
		return 0, 0
	}
	var walk func(n *dom.Node, depth int)
	walk = func(n *dom.Node, depth int) {
		if n == nil {
			return
		}
		nodeCount++
		if depth > maxDepth {
			maxDepth = depth
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, depth+1)
		}
	}
	walk(&s.Doc.Node, 1)
	return nodeCount, maxDepth
}

// EvaluateJS runs a JavaScript expression against the document's JS runtime
// and returns its string result.
func (s *Session) EvaluateJS(expr string) (string, error) {
	s.opMu.Lock()
	rt := s.jsRT
	s.opMu.Unlock()
	if rt == nil {
		return "", errors.New("engine: JS runtime not active")
	}
	return rt.Evaluate(expr)
}

// DarkMode reports whether the session is operating under dark color scheme.
func (s *Session) DarkMode() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.darkMode
}

// HighContrast reports whether the session is operating under high contrast mode.
func (s *Session) HighContrast() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.highContrast
}

// SetDarkMode updates the dark color scheme mode for the session.
func (s *Session) SetDarkMode(enabled bool) {
	s.opMu.Lock()
	s.darkMode = enabled
	s.opMu.Unlock()
}

// SetHighContrast updates the high contrast mode for the session.
func (s *Session) SetHighContrast(enabled bool) {
	s.opMu.Lock()
	s.highContrast = enabled
	s.opMu.Unlock()
}

func (s *Session) mediaOptions() css.MediaOptions {
	return css.MediaOptions{
		ViewportWidth: s.viewportW,
		DarkMode:      s.darkMode,
		HighContrast:  s.highContrast,
	}
}

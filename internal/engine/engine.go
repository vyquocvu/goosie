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
	"fmt"
	stdimage "image"
	"strings"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	imgdec "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
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

	// sheets are the parsed stylesheets kept for RefreshAfterFonts: when
	// deferred fonts land, styles must be re-resolved against the now-populated
	// customFonts table so text picks the web faces instead of system fallbacks.
	sheets []*css.Stylesheet

	imgBase  string
	imgFetch ImageFetcher
	natural  map[dom.NodeID]layout.NaturalSize
	images   map[dom.NodeID]stdimage.Image
	bgImages map[dom.NodeID]stdimage.Image
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
	// navigation starts unfocused. marked is the IME composition preview at
	// the caret: painted but not yet the value. focusValue records the
	// control's value when it gained focus so blur can fire a change event
	// if the value was modified during the focus session.
	focus      *dom.Node
	hovered    *dom.Node
	caret      int
	marked     string
	focusValue string

	// Text selection state. selActive and the two ends index the current
	// arena's word list, so Reflow clears them the same rebuild that moves
	// every word; a new session starts with nothing selected.
	selActive bool
	selAnchor selPos
	selHead   selPos

	// JS runtime for this document's scripts. nil when the caller did not
	// supply WithJS, so a session without scripting works exactly as before.
	jsRT *js.Runtime

	// Transitions tracks active CSS transitions for animated property changes.
	// nil when no transitions have been started.
	Transitions *TransitionTracker

	// Animations tracks active @keyframes animations. nil when no animations
	// have been started.
	Animations *AnimationController

	// inv is the caller's invalidation batch. When non-nil, JSMutationSink
	// records DOM mutations into it so the pipeline knows to re-style and
	// re-layout. Set via WithInvalidation.
	inv *Invalidation
}

// CSSLinker fetches one linked style sheet. base is the document URL the href
// resolves against; the returned string is the sheet's CSS text. A linker that
// fails simply contributes no rules for that href; document rendering does not
// stop for one missing sheet. Linked sheets are fetched concurrently, so a
// linker may be called from several goroutines at once.
type CSSLinker func(base, href string) (string, error)

// ImageFetcher fetches one image subresource's raw bytes. base is the document
// URL the src resolves against; the returned bytes are the encoded image. A
// fetcher that fails leaves that image undecoded, and the box keeps the layout
// space its attributes reserve, so a missing image does not stop rendering.
type ImageFetcher func(base, url string) ([]byte, error)

// FontFetcher fetches one @font-face resource's raw bytes. Same shape as
// ImageFetcher: the engine resolves the src URL against the stylesheet base
// before calling, so the fetcher only sees absolute URLs. Fonts are fetched
// concurrently, so a fetcher may be called from several goroutines at once.
type FontFetcher func(base, url string) ([]byte, error)

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

// WithJS supplies a JavaScript runtime for executing the document's classic
// scripts. The engine walks <script> elements in document order after the DOM
// is built and runs each one through the runtime. Script errors are recorded
// but do not stop the pipeline: a page with a broken ad script still renders.
// console receives the runtime's console output; pass nil to discard it.
func WithJS(rt *js.Runtime) Option {
	return func(s *Session) { s.jsRT = rt }
}

// WithInvalidation supplies the invalidation batch that JSMutationSink records
// DOM mutations into. Without it, JSMutationSink is a no-op and the engine
// does not learn about script-driven DOM changes.
func WithInvalidation(inv *Invalidation) Option {
	return func(s *Session) { s.inv = inv }
}

// fontJob is one @font-face descriptor set queued for fetching.
type fontJob struct {
	family, srcURL, weightStr, styleStr string
}

// NewSession parses HTML and builds the pipeline state. The caller provides the
// raw HTML and any author style sheets; <style> blocks inside the document are
// collected automatically, and the UA stylesheet is added internally.
// The returned session has a fully laid-out arena ready for paint.
func NewSession(html string, authorCSS []string, viewportW float32, opts ...Option) (*Session, error) {
	s := &Session{}
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
	doc, err := dom.ParseBounded(html, dom.ParseLimits{
		Nodes: MaxDocumentNodes, Depth: MaxDocumentDepth,
		Attributes: MaxAttributes, AttributeBytes: MaxAttributeBytes,
	})
	if err != nil {
		return nil, err
	}
	s.Doc = doc
	// Wire the parsed document into the JS runtime so getElementById,
	// querySelector, and event listeners operate on the same nodes the
	// engine's arena references.
	if s.jsRT != nil {
		s.jsRT.SetDOM(doc)
	}
	s.runScripts()
	sheets, err := checkedSheets(doc, authorCSS, s.linkBase, s.linker, viewportW)
	if err != nil {
		return nil, err
	}
	s.loadFonts(sheets)
	s.sheets = sheets
	s.Styles = style.ResolveViewport(doc, sheets, s.styleViewport(), s.customFonts)
	s.PseudoStyles = style.ResolvePseudoElements(doc, sheets, s.Styles, s.customFonts)
	s.startAnimations()
	s.loadImages()
	if err := s.Reflow(viewportW); err != nil {
		return nil, err
	}
	return s, nil
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
	s.applyFontJobs(jobs)
	if onReady != nil {
		onReady(1)
	}
}

// RefreshAfterFonts re-resolves styles against the now-populated customFonts
// table. Call it after LoadDeferredFonts completes so text picks up the web
// faces instead of the system fallbacks the first paint used.
func (s *Session) RefreshAfterFonts() {
	if len(s.sheets) == 0 {
		return
	}
	s.Styles = style.ResolveViewport(s.Doc, s.sheets, s.styleViewport(), s.customFonts)
	s.PseudoStyles = style.ResolvePseudoElements(s.Doc, s.sheets, s.Styles, s.customFonts)
}

// applyFontJobs fetches font bytes through a bounded worker pool and registers
// each successful result with the font registry, populating s.customFonts.
func (s *Session) applyFontJobs(jobs []fontJob) {
	data := make([][]byte, len(jobs))
	deadline := time.Now().Add(fontFetchBudget)
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
			data[i] = fetched
		}(i)
	}
	wg.Wait()

	fonts := &style.CustomFonts{}
	var fetchedBytes int64
	for i, j := range jobs {
		if len(data[i]) == 0 {
			continue
		}
		fetchedBytes += int64(len(data[i]))
		if fetchedBytes > maxFontBytes {
			return
		}
		idx, err := s.fontReg.Register(data[i])
		if err != nil {
			continue
		}
		bold := j.weightStr == "bold" || j.weightStr == "700" || j.weightStr == "800" || j.weightStr == "900"
		light := j.weightStr == "300" || j.weightStr == "200" || j.weightStr == "100"
		italic := j.styleStr == "italic" || j.styleStr == "oblique"
		fonts.Register(j.family, idx, bold, italic, light)
	}
	s.customFonts = fonts
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
			abs, ok := resolveSheetURL(fontBase, srcURL)
			if !ok {
				continue
			}
			jobs = append(jobs, fontJob{family: family, srcURL: abs, weightStr: weightStr, styleStr: styleStr})
		}
	}
	return jobs
}

// extractFontURL pulls the first url(...) reference out of a @font-face src
// value. The src property can carry multiple fallback formats; the engine only
// serves TrueType and OpenType, so the first url() is the one to try.
func extractFontURL(v string) string {
	idx := strings.Index(v, "url(")
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(v[idx+4:])
	if len(rest) > 0 && (rest[0] == '\'' || rest[0] == '"') {
		quote := rest[0]
		end := strings.IndexByte(rest[1:], quote)
		if end < 0 {
			return ""
		}
		return rest[1 : end+1]
	}
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// maxFontRequests caps how many @font-face sources one document may fetch so a
// stylesheet that references dozens of weights cannot stall first paint.
const maxFontRequests = 16

// maxFontBytes caps the total font payload one document may download. A single
// woff2 is typically 20–100 KiB; 10 MiB covers generous real-world pages while
// blocking a hostile stylesheet that references gigabytes of font data.
const maxFontBytes = 10 << 20

// fontFetchBudget is the wall-clock deadline for all font fetching. After this
// elapses, remaining @font-face rules are skipped and the document falls back
// to system fonts.
const fontFetchBudget = 6 * time.Second

// maxLoadedImages bounds how many image subresources one document may fetch,
// so a page that references hundreds cannot stall the first frame.
const maxLoadedImages = 256

// imageFetchBudget stops subresource fetching past a wall-clock deadline so a
// slow or unreachable host cannot push the first render past its time limit;
// images left unfetched simply reserve their box and paint nothing.
const imageFetchBudget = 6 * time.Second

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

func (s *Session) initImageMaps() {
	s.natural = make(map[dom.NodeID]layout.NaturalSize)
	s.images = make(map[dom.NodeID]stdimage.Image)
	s.bgImages = make(map[dom.NodeID]stdimage.Image)
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
			if src := strings.TrimSpace(n.GetAttribute("src")); src != "" {
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
				if abs, ok := resolveSheetURL(s.imgBase, src); ok {
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
				return nil
			}
			if abs, ok := resolveSheetURL(s.imgBase, bg); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, bg: true})
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
	deadline := time.Now().Add(imageFetchBudget)
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
		if r.bg {
			if _, ok := s.bgVectorSrc[r.abs]; ok {
				s.bgVectorRef[r.id] = r.abs
				continue
			}
		}
		img, ok := byURL[r.abs]
		if !ok {
			continue
		}
		if r.bg {
			s.bgImages[r.id] = img
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
	return style.Viewport{W: s.viewportW, H: s.viewportH}
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

// Reflow runs layout only, retaining the document, computed style identities,
// and font metrics. The old arena is untouched unless the candidate validates.
// Like Paint, this method must be called by the session's single owner.
func (s *Session) Reflow(viewportW float32) error {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
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
	candidate := layout.Build(s.Doc, s.Styles, s.PseudoStyles)
	candidate.Metrics = s.metrics
	candidate.NaturalSizes = s.natural
	layout.Block(candidate, layout.ObjectID(1), viewportW, s.viewportH)
	if err := validateArena(candidate); err != nil {
		return err
	}
	layout.Inline(candidate, layout.ObjectID(1))
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
				// Vector backgrounds without intrinsics rasterize here, at
				// the laid-out tile size, because only now is the tile
				// known. Skipped boxes (zero area, over-limit tiles, decode
				// failures) keep no image and paint nothing, like any
				// missing subresource.
				if abs, ok := s.bgVectorRef[n.ID]; ok {
					s.attachVectorBackground(candidate, i, abs)
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
					}
				}
			}
		}
	}
	// Update intersection observer geometry and check intersections.
	if s.jsRT != nil {
		s.jsRT.SetViewportRect(0, 0, s.viewportW, s.viewportH)
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil {
				s.jsRT.SetElementRect(int(n.ID), candidate.Objects[i].X, candidate.Objects[i].Y, candidate.Objects[i].W, candidate.Objects[i].H)
			}
		}
		s.jsRT.CheckIntersections()
	}
	s.Arena = candidate
	// Selection ends index the old arena's word list; the rebuild just moved
	// every word, so the highlight would paint on the wrong boxes.
	s.selActive = false
	return nil
}

// PaintChecked validates geometry and scale before any device-coordinate
// conversion, then checks tile metadata bounds before handing off the list.
func (s *Session) PaintChecked(scale float32) (*paint.List, error) {
	if err := validateScale(float64(scale)); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("missing session")
	}
	if err := validateArena(s.Arena); err != nil {
		return nil, err
	}
	list := s.Paint(scale)
	if err := ValidateExtent(list.Bounds()); err != nil {
		return nil, err
	}
	return list, nil
}

// Paint builds a display list from a trusted session's layout arena. Binaries
// processing untrusted input must use PaintChecked instead.
//
// The list is mutable; the caller freezes it with Publish when the frame is
// ready. scale is the device-pixel ratio: a 2x Retina display passes 2.0, and
// the builder multiplies every CSS-pixel coordinate by it to produce device
// pixels.
func (s *Session) Paint(scale float32) *paint.List {
	list := &paint.List{}
	b := paint.NewBuilder(list, s.Arena, scale, s.metrics)
	if s.focus != nil {
		if obj := s.objectFor(s.focus); obj != nil {
			b.SetFocus(obj, s.caret, s.marked)
		}
	}
	if spans := s.selectionSpans(); len(spans) > 0 {
		b.SetSelection(spans)
	}
	b.Build(layout.ObjectID(1))
	return list
}

// BackgroundColor returns the effective canvas background color.
// If html has a non-transparent background, that is used. Otherwise if body
// has a non-transparent background, that is used. If both are transparent,
// white RGB(255, 255, 255) is returned.
//
// A display:none root contributes nothing: with no box the background has
// nothing to propagate from, so html (or body) with display:none reads as
// absent here.
func (s *Session) BackgroundColor() frame.Color {
	if s.Doc != nil {
		if s.Doc.HTML != nil {
			if st, ok := s.Styles[s.Doc.HTML.ID]; ok && st.BackgroundColor.A > 0 && st.Display != style.DisplayNone {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
		if s.Doc.Body != nil {
			if st, ok := s.Styles[s.Doc.Body.ID]; ok && st.BackgroundColor.A > 0 && st.Display != style.DisplayNone {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
	}
	return frame.RGB(255, 255, 255)
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
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if cur.Node != nil && cur.Node.DataAtom == dom.AtomA {
				if href := cur.Node.GetAttribute("href"); href != "" {
					return href
				}
			}
		}
	}
	return ""
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
	n := s.activatableAt(x, y)
	if n == nil {
		return false
	}
	ev := dom.NewEvent("click", true, true)
	if !dom.DispatchEvent(n, ev) {
		return true // preventDefault was called
	}
	s.runClickDefault(n)
	return true
}

// runClickDefault performs the default action for an activated control:
// toggling checkbox/radio, or submitting/resetting the owning form for submit
// and reset controls. Shared by pointer ClickAt and keyboard Enter activation.
func (s *Session) runClickDefault(n *dom.Node) {
	typ := strings.ToLower(n.GetAttribute("type"))
	switch n.Data {
	case "input":
		switch typ {
		case "checkbox", "radio":
			s.ToggleControl(n)
		case "submit":
			if form := FindForm(n); form != nil {
				s.SubmitForm(form)
			}
		case "reset":
			if form := FindForm(n); form != nil {
				s.ResetForm(form)
			}
		}
	case "button":
		btnType := strings.ToLower(n.GetAttribute("type"))
		switch btnType {
		case "submit", "":
			if form := FindForm(n); form != nil {
				s.SubmitForm(form)
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
		x0, y0, x1, y1 := obj.BorderRect()
		if x < x0 || x >= x1 || y < y0 || y >= y1 {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if IsActivatable(cur.Node) {
				return cur.Node
			}
		}
	}
	return nil
}

// SubmitForm fires a cancelable "submit" event on the form, then collects form
// data and encodes it. The actual navigation is the caller's responsibility;
// the method stores the submission details for the host to retrieve.
func (s *Session) SubmitForm(form *dom.Node) {
	ev := dom.NewEvent("submit", true, true)
	if !dom.DispatchEvent(form, ev) {
		return // preventDefault was called
	}
	_ = CollectFormData(form)
	_ = FormMethod(form)
	_ = FormAction(form)
	// Navigation is the caller's responsibility.
}

// ResetForm resets the form controls to their default values.
// Currently a placeholder for future implementation.
func (s *Session) ResetForm(form *dom.Node) {
	// Placeholder: reset logic to be implemented.
	_ = form
}

// Match is one find result: the document-space rect of an occurrence of the
// query, in CSS pixels like every other engine coordinate.
type Match struct {
	X0, Y0, X1, Y1 float32
}

// maxFindMatches bounds the work a hostile page can make Find do.
const maxFindMatches = 1000

// Find returns the rects of every visible occurrence of query, in tree order,
// case-insensitively. Consecutive word boxes of a run are joined with single
// spaces, so a query may span word boundaries and line breaks but not element
// boundaries.
func (s *Session) Find(query string) []Match {
	if s.Arena == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	q := strings.ToLower(query)

	objs := s.Arena.Objects
	var matches []Match
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
			matches = append(matches, matchRect(words, off, start, start+len(q)))
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
	return matches
}

// matchRect maps a match spanning joined-string offsets [start, end) back onto
// the word boxes it covers. Edges inside a box are interpolated proportionally
// to character position, which is exact for full-word matches.
func matchRect(words []*layout.Object, off []int, start, end int) Match {
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
			return Match{X0: sx, Y0: y0, X1: ex, Y1: y1}
		}
		return Match{X0: x0, Y0: y0, X1: x1, Y1: y1}
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
	return Match{x0, top, x1, bot}
}

func parentObj(objs []layout.Object, o *layout.Object) *layout.Object {
	if o.Parent == 0 || int(o.Parent) >= len(objs) {
		return nil
	}
	return &objs[o.Parent]
}

// runScripts walks the DOM for <script> elements in document order and
// executes each one through the JS runtime. Script errors are silently
// ignored: a page with a broken script still renders, matching browser
// behavior where one bad ad tag does not take down the page.
func (s *Session) runScripts() {
	if s.jsRT == nil || s.Doc == nil {
		return
	}
	root := s.Doc.HTML
	if root == nil {
		root = &s.Doc.Node
	}
	walkScripts(root, s.jsRT)
}

func walkScripts(n *dom.Node, rt *js.Runtime) {
	if n == nil {
		return
	}
	if n.Element() && n.Data == "script" {
		src := n.TextContent()
		if src != "" {
			_ = rt.Run(src, "inline")
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkScripts(c, rt)
	}
}

// Close releases resources held by the session. If a JS runtime was supplied,
// it is closed so the realm is torn down.
func (s *Session) Close() {
	if s.jsRT != nil {
		_ = s.jsRT.Close()
	}
}

// JSTitle returns the document title as seen by the JS runtime after script
// execution. If no runtime was supplied, it returns the empty string.
func (s *Session) JSTitle() string {
	if s.jsRT == nil {
		return ""
	}
	return s.jsRT.Title()
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

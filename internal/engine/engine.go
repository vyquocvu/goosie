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
	Doc         *dom.Document
	Styles      map[dom.NodeID]*style.ComputedStyle
	PseudoStyles map[style.PseudoKey]*style.ComputedStyle
	Arena        *layout.Arena

	metrics   layout.Metrics
	viewportW float32
	viewportH float32
	linkBase  string
	linker    CSSLinker

	imgBase   string
	imgFetch  ImageFetcher
	natural   map[dom.NodeID]layout.NaturalSize
	images    map[dom.NodeID]stdimage.Image
	bgImages  map[dom.NodeID]stdimage.Image

	fontBase  string
	fontFetch FontFetcher
	fontReg   FontRegistry

	// Focus state for form editing. focus survives Reflow because reflow
	// rebuilds only the layout arena, not the DOM; a new session per
	// navigation starts unfocused.
	focus *dom.Node
	caret int
}

// CSSLinker fetches one linked style sheet. base is the document URL the href
// resolves against; the returned string is the sheet's CSS text. A linker that
// fails simply contributes no rules for that href; document rendering does not
// stop for one missing sheet.
type CSSLinker func(base, href string) (string, error)

// ImageFetcher fetches one image subresource's raw bytes. base is the document
// URL the src resolves against; the returned bytes are the encoded image. A
// fetcher that fails leaves that image undecoded, and the box keeps the layout
// space its attributes reserve, so a missing image does not stop rendering.
type ImageFetcher func(base, url string) ([]byte, error)

// FontFetcher fetches one @font-face resource's raw bytes. Same shape as
// ImageFetcher: the engine resolves the src URL against the stylesheet base
// before calling, so the fetcher only sees absolute URLs.
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
	css.SetMediaViewportWidth(viewportW)
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
	sheets, err := checkedSheets(doc, authorCSS, s.linkBase, s.linker)
	if err != nil {
		return nil, err
	}
	s.loadFonts(sheets)
	s.Doc = doc
	s.Styles = style.ResolveViewport(doc, sheets, s.styleViewport())
	s.PseudoStyles = style.ResolvePseudoElements(doc, sheets, s.Styles)
	s.loadImages()
	if err := s.Reflow(viewportW); err != nil {
		return nil, err
	}
	return s, nil
}

// loadFonts extracts @font-face rules from every stylesheet, fetches the font
// files, registers them with the font registry, and publishes the family→index
// map so style resolution can match custom family names.
func (s *Session) loadFonts(sheets []*css.Stylesheet) {
	if s.fontFetch == nil || s.fontReg == nil {
		return
	}
	familyMap := make(map[string]uint16)
	registry := make(map[style.CustomFontKey]uint16)
	deadline := time.Now().Add(fontFetchBudget)
	var fetched int
	var fetchedBytes int64
	for _, sheet := range sheets {
		for _, ff := range sheet.FontFaces {
			if fetched >= maxFontRequests {
				return
			}
			if time.Now().After(deadline) {
				return
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
			abs, ok := resolveSheetURL(s.fontBase, srcURL)
			if !ok {
				continue
			}
			data, err := s.fontFetch(s.fontBase, abs)
			if err != nil {
				continue
			}
			fetchedBytes += int64(len(data))
			fetched++
			if fetchedBytes > maxFontBytes {
				return
			}
			idx, err := s.fontReg.Register(data)
			if err != nil {
				continue
			}
			familyMap[strings.ToLower(family)] = idx
			
			// Build the weight/slant key for the registry.
			bold := weightStr == "bold" || weightStr == "700" || weightStr == "800" || weightStr == "900"
			light := weightStr == "300" || weightStr == "200" || weightStr == "100"
			italic := styleStr == "italic" || styleStr == "oblique"
			key := style.CustomFontKey{
				Family: strings.ToLower(family),
				Bold:   bold,
				Italic: italic,
				Light:  light,
			}
			registry[key] = idx
		}
	}
	if len(familyMap) > 0 {
		style.SetCustomFonts(familyMap)
	}
	if len(registry) > 0 {
		style.SetCustomFontRegistry(registry)
	}
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

// loadImages walks the styled document for <img> srcs and background-image
// URLs, fetches and decodes each unique target, and records the intrinsic size
// (for <img> layout) and the decoded pixels (for paint). It is a no-op without
// a fetcher.
//
// Fetching runs through a bounded worker pool rather than inline in the walk. A
// page can carry dozens of images; a sequential walk spends the whole
// wall-clock budget on the first handful (at ~0.8s each, a 6s budget covers
// only ~7), leaving the rest to reserve their box and paint nothing. Fetching
// concurrently lets the same deadline cover the page, which is what a real
// browser does and what the reference renders assume.
func (s *Session) loadImages() {
	if s.imgFetch == nil || s.Doc == nil {
		return
	}
	s.natural = make(map[dom.NodeID]layout.NaturalSize)
	s.images = make(map[dom.NodeID]stdimage.Image)
	s.bgImages = make(map[dom.NodeID]stdimage.Image)

	// Collect every reference in document order. A URL can back several nodes
	// (an <img> and a background), so fetch each unique target once and fan the
	// decoded pixels back out to all of them.
	type ref struct {
		id  dom.NodeID
		abs string
		bg  bool
	}
	var refs []ref
	var uniq []string
	seen := make(map[string]bool)
	_, _ = walkDocument(s.Doc, func(n *dom.Node) error {
		if !n.Element() {
			return nil
		}
		if n.Data == "img" {
			if src := strings.TrimSpace(n.GetAttribute("src")); src != "" {
				if abs, ok := resolveSheetURL(s.imgBase, src); ok {
					refs = append(refs, ref{id: n.ID, abs: abs})
					if !seen[abs] {
						seen[abs] = true
						uniq = append(uniq, abs)
					}
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.BackgroundImage != "" {
			if abs, ok := resolveSheetURL(s.imgBase, st.BackgroundImage); ok {
				refs = append(refs, ref{id: n.ID, abs: abs, bg: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		return nil
	})

	byURL := make(map[string]stdimage.Image, len(uniq))
	var mu sync.Mutex
	loaded := 0
	deadline := time.Now().Add(imageFetchBudget)
	const imageWorkers = 12
	sem := make(chan struct{}, imageWorkers)
	var wg sync.WaitGroup
	reservation := newImageReservation(MaxDocumentDecodedPixels)
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
			data, err := s.imgFetch(s.imgBase, u)
			if err != nil || len(data) == 0 {
				return
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

	for _, r := range refs {
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
	if s.images != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil {
				if img, ok := s.images[n.ID]; ok {
					candidate.Objects[i].Image = img
				}
				if img, ok := s.bgImages[n.ID]; ok {
					candidate.Objects[i].BgImage = img
				}
			}
		}
	}
	s.Arena = candidate
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
			b.SetFocus(obj, s.caret)
		}
	}
	b.Build(layout.ObjectID(1))
	return list
}

// BackgroundColor returns the effective canvas background color.
// If html has a non-transparent background, that is used. Otherwise if body
// has a non-transparent background, that is used. If both are transparent,
// white RGB(255, 255, 255) is returned.
func (s *Session) BackgroundColor() frame.Color {
	if s.Doc != nil {
		if s.Doc.HTML != nil {
			if st, ok := s.Styles[s.Doc.HTML.ID]; ok && st.BackgroundColor.A > 0 {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
		if s.Doc.Body != nil {
			if st, ok := s.Styles[s.Doc.Body.ID]; ok && st.BackgroundColor.A > 0 {
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

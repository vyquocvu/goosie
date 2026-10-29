package engine

import (
	"errors"
	stdimage "image"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	imgdec "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
)

// Script-created image loading.
//
// A page can allocate an HTMLImageElement with `new Image()` and point it at a
// source without ever putting it in the document. That path must not become a
// way around the caps a parsed <img> passes: internal/js owns the element's
// load state and events but may not import internal/image (the archtest import
// table), so it injects this loader and the engine performs the fetch, the
// Probe admission and the decode.
//
// The three caps applied here mirror fetchImages exactly:
//
//   - maxLoadedImages attempts per document (not successes: a failing URL does
//     not replenish the allowance),
//   - ImageFetchBudget wall-clock from the first attempt, and
//   - subBudget bytes plus the shared imgReservation decoded-pixel budget
//     (imgdec.Probe's own MaxEncodedImageBytes/MaxImageDimension/
//     MaxDecodedImagePixels checks run first).
//
// A source data: URI decodes inline on the calling goroutine; the js side
// settles the element in a microtask, so the result is observable at the end
// of the script turn. Network sources fetch on their own goroutine and settle
// whenever the host fetcher returns.

var (
	errScriptImageAttempts = errors.New("engine: script image attempt limit reached")
	errScriptImageBudget   = errors.New("engine: script image fetch budget exhausted")
	errScriptImageFetch    = errors.New("engine: script image could not be fetched")
	errScriptImageBytes    = errors.New("engine: script image exceeds the subresource byte budget")
	errScriptImagePixels   = errors.New("engine: script image exceeds the document decoded-pixel budget")
	errScriptImageDecode   = errors.New("engine: script image could not be decoded")
)

// LoadScriptImage implements js.ImageLoader. It never blocks the JS goroutine
// on network I/O and never reports its own panics back to the page: every
// failure becomes an error result, which the binding turns into an error
// event and a rejected decode().
func (s *Session) LoadScriptImage(src string, done func(js.LoadedImage)) {
	if s == nil || done == nil {
		return
	}
	if !s.beginScriptImage() {
		done(js.LoadedImage{Src: src, Err: errScriptImageAttempts})
		return
	}
	if strings.HasPrefix(src, "data:") {
		s.decodeScriptImage(src, done)
		return
	}
	go s.decodeScriptImage(src, done)
}

// beginScriptImage charges one attempt and reports whether the document may
// make it. The budget window opens at the first attempt, so a page that
// creates images later still gets the same ImageFetchBudget the parse-time
// pool gets rather than a fresh window per element.
func (s *Session) beginScriptImage() bool {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	if s.scriptImgAttempts >= maxLoadedImages {
		return false
	}
	if s.scriptImgDeadline.IsZero() {
		s.scriptImgDeadline = time.Now().Add(ImageFetchBudget)
	}
	if time.Now().After(s.scriptImgDeadline) {
		return false
	}
	s.scriptImgAttempts++
	return true
}

// decodeScriptImage fetches, admits, decodes and reports one script image.
// The decoded pixels are handed back through LoadedImage.Pixels so a script
// assignment that lands on an in-document <img> can paint; whether they are
// kept is decided on the pump goroutine by drainScriptImages, which knows
// whether the node is still connected and still names this source. A detached
// HTMLImageElement (new Image()) exposes naturalWidth/naturalHeight but has no
// box to paint, so its pixels are dropped at that point. The pixel
// reservation is kept either way: it bounds cumulative decode work per
// document, and nothing here refunds it (see drainScriptImages).
func (s *Session) decodeScriptImage(src string, done func(js.LoadedImage)) {
	fail := func(err error) {
		done(js.LoadedImage{Src: src, Err: err})
	}
	s.imgMu.Lock()
	deadline := s.scriptImgDeadline
	reservation := s.imgReservation
	if reservation == nil {
		reservation = newImageReservation(MaxDocumentDecodedPixels)
		s.imgReservation = reservation
	}
	s.imgMu.Unlock()
	if !deadline.IsZero() && time.Now().After(deadline) {
		fail(errScriptImageBudget)
		return
	}
	data, err := s.fetchImageBytes(src)
	if err != nil || len(data) == 0 {
		fail(errScriptImageFetch)
		return
	}
	// Raw bytes join the shared subresource total before anything decodes
	// them, exactly as in fetchImages.
	if !s.sub.take(len(data)) {
		fail(errScriptImageBytes)
		return
	}
	cfg, err := imgdec.Probe(data)
	if err != nil {
		fail(err)
		return
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if !reservation.reserve(pixels) {
		fail(errScriptImagePixels)
		return
	}
	img, err := imgdec.Decode(data)
	if err != nil {
		reservation.release(pixels)
		fail(err)
		return
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		reservation.release(pixels)
		fail(errScriptImageDecode)
		return
	}
	w, h := b.Dx(), b.Dy()
	// For a standalone SVG with a viewBox ratio but no absolute dimensions,
	// HTML's default object sizing algorithm completes the ratio against the
	// default 300x150 object size. The SVG raster viewport can instead reflect
	// the viewBox units (for example 600x200), which are not intrinsic pixels.
	if hasDims, hasRatio := imgdec.SVGIntrinsicKind(data); !hasDims && hasRatio {
		if rw, rh, ok := imgdec.SVGRatio(data); ok && rw > 0 && rh > 0 {
			scale := min(300/rw, 150/rh)
			w, h = int(rw*scale+0.5), int(rh*scale+0.5)
		}
	}
	done(js.LoadedImage{Src: src, Width: w, Height: h, Pixels: img})
}

// scriptImagePixels is one successful script-image result together with the
// node and raw source it belongs to. node is kept so reconciliation can ask
// the live DOM (connected? still an <img>? still the same src text?) instead
// of trusting a result that may be several turns old.
type scriptImagePixels struct {
	node   *dom.Node
	nodeID dom.NodeID
	rawSrc string
	image  stdimage.Image
	w, h   int
}

// retainScriptImage implements js.Options.ImagePixelSink. It runs inside
// PumpJS -> Tick -> microtask drain (and, for an inline data: source assigned
// during the load-time script turn, inside NewSession's script execution). It
// must not reflow there: the pump is in the middle of a turn and a later
// timer or RAF callback in that same turn may still change the src again. So
// it only records the result; drainScriptImages turns it into paint state
// after the turn's callbacks have all run.
//
// A result that names a detached node is recorded here too and dropped at
// drain time; the runtime still settles the element's state and fires its
// load event, because naturalWidth/naturalHeight must work on a detached
// Image.
func (s *Session) retainScriptImage(node *dom.Node, rawSrc string, pixels any) {
	if s == nil || node == nil || pixels == nil {
		return
	}
	img, ok := pixels.(stdimage.Image)
	if !ok {
		return
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return
	}
	s.imgMu.Lock()
	if s.pendingScriptImgs == nil {
		s.pendingScriptImgs = make(map[dom.NodeID]scriptImagePixels)
	}
	s.pendingScriptImgs[node.ID] = scriptImagePixels{
		node:   node,
		nodeID: node.ID,
		rawSrc: rawSrc,
		image:  img,
		w:      b.Dx(),
		h:      b.Dy(),
	}
	// The pixels need a layout pass to reach a box even when the src write
	// itself already marked the document mutated: RestyleAndReflow can
	// early-return without rebuilding the arena. The bit survives turns when
	// the deferred-load skip postpones the reflow.
	s.scriptImgsLayoutPending = true
	s.imgMu.Unlock()
}

// scriptImageCurrent reports whether a recorded result still describes the
// node's live state. Only a connected <img> whose src attribute is still the
// exact raw text that produced the pixels may paint them: re-resolving a URL
// here would compare against a different base than the binding used.
func scriptImageCurrent(node *dom.Node, rawSrc string) bool {
	return node != nil && node.IsConnected() && node.Data == "img" && node.GetAttribute("src") == rawSrc
}

// drainScriptImages attaches this turn's successful script pixels to the
// session's image tables and reconciles previously retained entries with the
// DOM. It runs on the pump goroutine after Tick and TickRAF, so the last src
// write of the turn owns the result.
//
// Reinsertion rule: a node that is dropped (disconnected or src changed) and
// later reconnected still holding a valid src does not get its old pixels
// back; the drop forgets them, and only a new src assignment — which starts a
// new load — can attach pixels again. Nothing here refunds the decoded-pixel
// reservation for a dropped entry: the charge bounds cumulative decode work
// per document, and refunding it would let a src-cycling page decode
// unbounded bytes.
//
// The returned layoutPending reports whether a layout is owed from this or an
// earlier postponed turn. The caller performs exactly one layout and then
// clears the bit.
func (s *Session) drainScriptImages() (layoutPending bool) {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	pending := s.pendingScriptImgs
	s.pendingScriptImgs = nil
	changed := false
	for id, e := range pending {
		if !scriptImageCurrent(e.node, e.rawSrc) {
			continue
		}
		if s.images == nil {
			s.images = make(map[dom.NodeID]stdimage.Image)
		}
		if s.natural == nil {
			s.natural = make(map[dom.NodeID]layout.NaturalSize)
		}
		if s.scriptImgs == nil {
			s.scriptImgs = make(map[dom.NodeID]scriptImagePixels)
		}
		s.images[id] = e.image
		s.natural[id] = layout.NaturalSize{W: float32(e.w), H: float32(e.h)}
		s.scriptImgs[id] = e
		changed = true
	}
	for id, e := range s.scriptImgs {
		if scriptImageCurrent(e.node, e.rawSrc) {
			continue
		}
		delete(s.scriptImgs, id)
		delete(s.images, id)
		delete(s.natural, id)
		changed = true
	}
	if changed {
		s.scriptImgsLayoutPending = true
	}
	return s.scriptImgsLayoutPending
}

// clearScriptImgsLayoutPending clears the layout-pending bit after a layout
// rebuilt the arena with the attached intrinsic sizes. A result that arrived
// between the drain and the reflow leaves the bit set so the next turn still
// lays it out.
func (s *Session) clearScriptImgsLayoutPending() {
	s.imgMu.Lock()
	if len(s.pendingScriptImgs) == 0 {
		s.scriptImgsLayoutPending = false
	}
	s.imgMu.Unlock()
}

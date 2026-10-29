package engine

import (
	"errors"
	"strings"
	"time"

	imgdec "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/js"
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
// The decoded pixels are not retained: a detached HTMLImageElement exposes
// naturalWidth/naturalHeight, and paint cannot reach it. The pixel
// reservation is kept anyway, so an unbounded chain of script images stays
// inside MaxDocumentDecodedPixels rather than spending decode work the
// document budget never saw.
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
	done(js.LoadedImage{Src: src, Width: w, Height: h})
}

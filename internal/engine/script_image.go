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

var (
	errScriptImageAttempts = errors.New("engine: script image attempt limit reached")
	errScriptImageBudget   = errors.New("engine: script image fetch budget exhausted")
	errScriptImageFetch    = errors.New("engine: script image could not be fetched")
	errScriptImageBytes    = errors.New("engine: script image exceeds the subresource byte budget")
	errScriptImagePixels   = errors.New("engine: script image exceeds the document decoded-pixel budget")
	errScriptImageDecode   = errors.New("engine: script image could not be decoded")
)

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
	if hasDims, hasRatio := imgdec.SVGIntrinsicKind(data); !hasDims && hasRatio {
		if rw, rh, ok := imgdec.SVGRatio(data); ok && rw > 0 && rh > 0 {
			scale := min(300/rw, 150/rh)
			w, h = int(rw*scale+0.5), int(rh*scale+0.5)
		}
	}
	done(js.LoadedImage{Src: src, Width: w, Height: h, Pixels: img})
}

type scriptImagePixels struct {
	node   *dom.Node
	nodeID dom.NodeID
	rawSrc string
	image  stdimage.Image
	w, h   int
}

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
	s.scriptImgsLayoutPending = true
	s.imgMu.Unlock()
}

func scriptImageCurrent(node *dom.Node, rawSrc string) bool {
	return node != nil && node.IsConnected() && node.Data == "img" && node.GetAttribute("src") == rawSrc
}

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

func (s *Session) clearScriptImgsLayoutPending() {
	s.imgMu.Lock()
	if len(s.pendingScriptImgs) == 0 {
		s.scriptImgsLayoutPending = false
	}
	s.imgMu.Unlock()
}

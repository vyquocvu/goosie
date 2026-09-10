package image

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"strings"
	"sync"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// IsSVGContent performs a quick sanity check that the data contains an SVG root element.
func IsSVGContent(data []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if se, ok := tok.(xml.StartElement); ok {
			return strings.EqualFold(se.Name.Local, "svg")
		}
	}
}

// rasterizeIcon draws an oksvg icon onto a new RGBA canvas at (w, h) pixels.
func rasterizeIcon(icon *oksvg.SvgIcon, w, h int) *image.RGBA {
	icon.SetTarget(0, 0, float64(w), float64(h))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)

	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	return rgba
}

// RasterizeSVG decodes SVG bytes and rasterizes to an RGBA image at (w, h) pixels.
// If w or h is 0, uses the SVG's intrinsic ViewBox size (defaulting to 100 if unset).
//
// Identical (bytes, size) rasters are served from a small bounded process-wide
// LRU: render-tree rebuilds re-rasterize every inline <svg> on each full
// reparse (including every structural JS mutation batch), so without this
// cache a single page logo costs a full software raster per mutation. The
// returned image is shared and must be treated as read-only.
func RasterizeSVG(data []byte, w, h int) (*image.RGBA, error) {
	if !IsSVGContent(data) {
		return nil, fmt.Errorf("svg parse: data does not contain an SVG root element")
	}

	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("svg parse: %w", err)
	}

	intrinsicW := int(icon.ViewBox.W)
	intrinsicH := int(icon.ViewBox.H)
	if intrinsicW <= 0 {
		intrinsicW = 100
	}
	if intrinsicH <= 0 {
		intrinsicH = 100
	}
	if w <= 0 {
		w = intrinsicW
	}
	if h <= 0 {
		h = intrinsicH
	}

	if hit := svgRasterCacheLookup(data, w, h); hit != nil {
		return hit, nil
	}
	rgba := rasterizeIcon(icon, w, h)
	svgRasterCacheStore(data, w, h, rgba)
	return rgba, nil
}

const (
	// svgRasterCacheMaxEntries caps the number of retained SVG rasters.
	svgRasterCacheMaxEntries = 64
	// svgRasterCacheMaxBytes caps total retained raster bytes (w*h*4 each).
	svgRasterCacheMaxBytes = int64(16 * 1024 * 1024)
	// svgRasterCacheSingleMaxBytes caps a single cacheable raster; larger
	// one-off rasters are returned uncached rather than evicting the cache.
	svgRasterCacheSingleMaxBytes = int64(8 * 1024 * 1024)
)

type svgRasterKey struct {
	sum [32]byte
	w   int
	h   int
}

type svgRasterEntry struct {
	key   svgRasterKey
	rgba  *image.RGBA
	bytes int64
}

var svgRasterCache = struct {
	sync.Mutex
	items map[svgRasterKey]*list.Element
	lru   *list.List
	bytes int64
}{items: make(map[svgRasterKey]*list.Element), lru: list.New()}

func svgRasterCacheLookup(data []byte, w, h int) *image.RGBA {
	key := svgRasterKey{sum: sha256.Sum256(data), w: w, h: h}
	svgRasterCache.Lock()
	defer svgRasterCache.Unlock()
	if elem, ok := svgRasterCache.items[key]; ok {
		svgRasterCache.lru.MoveToFront(elem)
		return elem.Value.(*svgRasterEntry).rgba
	}
	return nil
}

func svgRasterCacheStore(data []byte, w, h int, rgba *image.RGBA) {
	if rgba == nil {
		return
	}
	size := int64(rgba.Bounds().Dx()) * int64(rgba.Bounds().Dy()) * 4
	if size <= 0 || size > svgRasterCacheSingleMaxBytes {
		return
	}
	key := svgRasterKey{sum: sha256.Sum256(data), w: w, h: h}
	svgRasterCache.Lock()
	defer svgRasterCache.Unlock()
	if elem, ok := svgRasterCache.items[key]; ok {
		svgRasterCache.lru.MoveToFront(elem)
		return
	}
	elem := svgRasterCache.lru.PushFront(&svgRasterEntry{key: key, rgba: rgba, bytes: size})
	svgRasterCache.items[key] = elem
	svgRasterCache.bytes += size
	for (len(svgRasterCache.items) > svgRasterCacheMaxEntries || svgRasterCache.bytes > svgRasterCacheMaxBytes) && svgRasterCache.lru.Len() > 0 {
		back := svgRasterCache.lru.Back()
		if back == nil {
			break
		}
		svgRasterCache.lru.Remove(back)
		entry := back.Value.(*svgRasterEntry)
		delete(svgRasterCache.items, entry.key)
		svgRasterCache.bytes -= entry.bytes
		if svgRasterCache.bytes < 0 {
			svgRasterCache.bytes = 0
		}
	}
}

// ClearSVGRasterCache drops all retained SVG rasters. Test hook for isolation.
func ClearSVGRasterCache() {
	svgRasterCache.Lock()
	defer svgRasterCache.Unlock()
	svgRasterCache.items = make(map[svgRasterKey]*list.Element)
	svgRasterCache.lru = list.New()
	svgRasterCache.bytes = 0
}

// SVGRasterCacheStats reports entry count and retained bytes. Test hook.
func SVGRasterCacheStats() (entries int, bytes int64) {
	svgRasterCache.Lock()
	defer svgRasterCache.Unlock()
	return len(svgRasterCache.items), svgRasterCache.bytes
}

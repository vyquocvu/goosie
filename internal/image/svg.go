package image

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"strings"

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

	return rasterizeIcon(icon, w, h), nil
}

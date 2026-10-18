package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// ExportSessionToPDF renders the document session into a standard PDF 1.4 document at outPath.
func ExportSessionToPDF(sess *engine.Session, fonts *raster.Fonts, scale float32, outPath string) error {
	if sess == nil {
		return fmt.Errorf("pdf export: nil session")
	}
	if scale < 1 {
		scale = 1
	}

	// 1. Paint document to display list
	list, err := sess.PaintChecked(scale)
	if err != nil {
		return fmt.Errorf("pdf export: paint document: %w", err)
	}
	dl := list.Build(1)
	extent := dl.Extent()

	// 2. Geometry: standard A4 page dimensions (595.28 x 841.89 points)
	const (
		pdfPageW = 595
		pdfPageH = 842
	)

	sessW, _ := sess.Viewport()
	if sessW <= 0 {
		sessW = 800
	}
	bmpW := int32(float32(sessW) * scale)
	if bmpW <= 0 {
		bmpW = 800
	}

	// Content height per page
	ptPerPx := float64(pdfPageW) / float64(bmpW)
	pageContentH := int32(float64(pdfPageH) / ptPerPx)
	if pageContentH <= 0 {
		pageContentH = bmpW * 14 / 10
	}

	docH := extent.H()
	if docH < pageContentH {
		docH = pageContentH
	}
	numPages := int((docH + pageContentH - 1) / pageContentH)
	if numPages <= 0 {
		numPages = 1
	}
	if numPages > 100 {
		numPages = 100
	}

	bg := sess.BackgroundColor()
	if !bg.Opaque() {
		bg = frame.RGB(255, 255, 255)
	}

	// 3. Rasterize each page and compress image stream
	type pageImage struct {
		compressed []byte
		w, h       int32
	}
	pages := make([]pageImage, numPages)

	for p := 0; p < numPages; p++ {
		bY0 := int32(p) * pageContentH
		bY1 := bY0 + pageContentH
		bounds := frame.Rect4(0, bY0, bmpW, bY1)

		pageBmp := frame.NewBitmap(int(bmpW), int(pageContentH))
		if err := raster.RasterizeTile(dl, bounds, pageBmp, fonts, nil, bg); err != nil {
			return fmt.Errorf("pdf export: rasterize page %d: %w", p+1, err)
		}

		// Convert RGBA to RGB
		rgb := make([]byte, pageBmp.W*pageBmp.H*3)
		j := 0
		for y := 0; y < pageBmp.H; y++ {
			row := pageBmp.RGBA[y*pageBmp.Stride : y*pageBmp.Stride+pageBmp.W*4]
			for x := 0; x < pageBmp.W; x++ {
				rgb[j] = row[x*4]
				rgb[j+1] = row[x*4+1]
				rgb[j+2] = row[x*4+2]
				j += 3
			}
		}

		var compBuf bytes.Buffer
		zw := zlib.NewWriter(&compBuf)
		if _, err := zw.Write(rgb); err != nil {
			return fmt.Errorf("pdf export: compress page %d: %w", p+1, err)
		}
		if err := zw.Close(); err != nil {
			return fmt.Errorf("pdf export: close zlib page %d: %w", p+1, err)
		}

		pages[p] = pageImage{
			compressed: compBuf.Bytes(),
			w:          bmpW,
			h:          pageContentH,
		}
	}

	// 4. Assemble PDF 1.4 document
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	totalObjs := 2 + numPages*3
	offsets := make([]int, totalObjs+1)

	// Object 1: Catalog
	offsets[1] = pdf.Len()
	pdf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Object 2: Pages
	offsets[2] = pdf.Len()
	pdf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for p := 0; p < numPages; p++ {
		pageObjID := 3 + p*3
		if p > 0 {
			pdf.WriteString(" ")
		}
		pdf.WriteString(fmt.Sprintf("%d 0 R", pageObjID))
	}
	pdf.WriteString(fmt.Sprintf("] /Count %d >>\nendobj\n", numPages))

	// Objects for each page: Page, Image XObject, Contents Stream
	for p := 0; p < numPages; p++ {
		pageObjID := 3 + p*3
		imgObjID := pageObjID + 1
		contentObjID := pageObjID + 2
		pi := pages[p]

		// Page object
		offsets[pageObjID] = pdf.Len()
		pdf.WriteString(fmt.Sprintf("%d 0 obj\n<<\n  /Type /Page\n  /Parent 2 0 R\n  /MediaBox [0 0 %d %d]\n  /Resources <<\n    /XObject << /Im1 %d 0 R >>\n  >>\n  /Contents %d 0 R\n>>\nendobj\n",
			pageObjID, pdfPageW, pdfPageH, imgObjID, contentObjID))

		// Image XObject
		offsets[imgObjID] = pdf.Len()
		pdf.WriteString(fmt.Sprintf("%d 0 obj\n<<\n  /Type /XObject\n  /Subtype /Image\n  /Width %d\n  /Height %d\n  /ColorSpace /DeviceRGB\n  /BitsPerComponent 8\n  /Filter /FlateDecode\n  /Length %d\n>>\nstream\n",
			imgObjID, pi.w, pi.h, len(pi.compressed)))
		pdf.Write(pi.compressed)
		pdf.WriteString("\nendstream\nendobj\n")

		// Content stream (drawing /Im1 onto page)
		streamContent := fmt.Sprintf("q\n%d 0 0 %d 0 0 cm\n/Im1 Do\nQ\n", pdfPageW, pdfPageH)
		offsets[contentObjID] = pdf.Len()
		pdf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n",
			contentObjID, len(streamContent), streamContent))
	}

	// Cross-reference table
	startxref := pdf.Len()
	pdf.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", totalObjs+1))
	for i := 1; i <= totalObjs; i++ {
		pdf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}

	// Trailer
	pdf.WriteString(fmt.Sprintf("trailer\n<<\n  /Size %d\n  /Root 1 0 R\n>>\nstartxref\n%d\n%%%%EOF\n",
		totalObjs+1, startxref))

	return os.WriteFile(outPath, pdf.Bytes(), 0o644)
}

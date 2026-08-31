package image_test

import (
	"testing"

	img "github.com/vyquocvu/goosie/internal/image"
	"github.com/stretchr/testify/assert"
)

func TestRasterizeSVG(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10">
		<rect x="0" y="0" width="10" height="10" fill="#ff0000"/>
	</svg>`)
	result, err := img.RasterizeSVG(svgData, 10, 10)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 10, result.Bounds().Dx())
	assert.Equal(t, 10, result.Bounds().Dy())
}

func TestRasterizeSVGDefaultSize(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
		<circle cx="10" cy="10" r="8" fill="blue"/>
	</svg>`)
	result, err := img.RasterizeSVG(svgData, 0, 0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 20, result.Bounds().Dx())
}

func TestRasterizeSVGMalformed(t *testing.T) {
	_, err := img.RasterizeSVG([]byte(`not svg`), 10, 10)
	assert.Error(t, err)
}

func TestParseSVGIntrinsicSize(t *testing.T) {
	cases := []struct {
		name string
		svg  string
		w    int
		h    int
		ok   bool
	}{
		{"px attrs", `<svg width="234px" height="72px" viewBox="0 0 468 144">`, 234, 72, true},
		{"unitless attrs", `<svg xmlns="x" width="100" height="50" viewBox="0 0 100 50">`, 100, 50, true},
		{"no attrs", `<svg viewBox="0 0 100 50">`, 0, 0, false},
		{"single quote", `<svg width='32px' height='32px' viewBox="0 0 32 32">`, 32, 32, true},
		{"pt units", `<svg width="12pt" height="6pt">`, 16, 8, true},
		{"pc units", `<svg width="1pc" height="0.5pc">`, 16, 8, true},
		{"percent ignored", `<svg width="50%" height="auto">`, 0, 0, false},
		{"case insensitive", `<svg WIDTH="10" HEIGHT="20">`, 10, 20, true},
		{"whitespace", `<svg width = "40" height = "30" >`, 40, 30, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h, ok := img.ParseSVGIntrinsicSize([]byte(tc.svg))
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v", ok, tc.ok)
			}
			if ok && (w != tc.w || h != tc.h) {
				t.Fatalf("got %dx%d want %dx%d", w, h, tc.w, tc.h)
			}
		})
	}
}

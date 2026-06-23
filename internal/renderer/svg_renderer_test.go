package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSVGRasterize(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10">
		<rect x="0" y="0" width="10" height="10" fill="#ff0000"/>
	</svg>`)
	img, err := RasterizeSVG(svgData, 10, 10)
	assert.NoError(t, err)
	assert.NotNil(t, img)
	assert.Equal(t, 10, img.Bounds().Dx())
	assert.Equal(t, 10, img.Bounds().Dy())
}

func TestSVGRasterizeDefaultSize(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
		<circle cx="10" cy="10" r="8" fill="blue"/>
	</svg>`)
	// w=0, h=0 → uses intrinsic 20x20
	img, err := RasterizeSVG(svgData, 0, 0)
	assert.NoError(t, err)
	assert.NotNil(t, img)
	assert.Equal(t, 20, img.Bounds().Dx())
}

func TestSVGRasterizeMalformed(t *testing.T) {
	// Should not panic, just return error
	_, err := RasterizeSVG([]byte(`not svg`), 10, 10)
	assert.Error(t, err)
}

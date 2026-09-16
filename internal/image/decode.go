package image

import (
	"bytes"
	"fmt"
	gimage "image"
	"image/jpeg"
	"image/png"
	"io"

	_ "image/gif"
)

// Decode decodes an image from raw bytes. The output is premultiplied RGBA at
// the original resolution; the caller is responsible for any downscaling.
func Decode(data []byte) (gimage.Image, error) {
	r := bytes.NewReader(data)
	img, _, err := gimage.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("image: decode: %w", err)
	}
	return img, nil
}

// DecodePNG decodes a PNG image.
func DecodePNG(r io.Reader) (gimage.Image, error) {
	return png.Decode(r)
}

// DecodeJPEG decodes a JPEG image.
func DecodeJPEG(r io.Reader) (gimage.Image, error) {
	return jpeg.Decode(r)
}

// EncodePNG encodes an image as PNG.
func EncodePNG(w io.Writer, img gimage.Image) error {
	return png.Encode(w, img)
}

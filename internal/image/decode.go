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

const (
	MaxEncodedImageBytes     = 8 << 20
	MaxImageDimension        = 8192
	MaxDecodedImagePixels    = 16 * 1024 * 1024
)

// Probe checks encoded length and metadata before full decode. It returns the
// image config if the data passes admission limits.
func Probe(data []byte) (gimage.Config, error) {
	if len(data) > MaxEncodedImageBytes {
		return gimage.Config{}, fmt.Errorf("image: encoded size %d exceeds limit %d", len(data), MaxEncodedImageBytes)
	}
	cfg, _, err := gimage.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return gimage.Config{}, fmt.Errorf("image: decode config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageDimension || cfg.Height > MaxImageDimension {
		return gimage.Config{}, fmt.Errorf("image: dimensions %dx%d exceed admission limit %d", cfg.Width, cfg.Height, MaxImageDimension)
	}
	if int64(cfg.Width) > MaxDecodedImagePixels/int64(cfg.Height) {
		return gimage.Config{}, fmt.Errorf("image: pixel count %dx%d exceeds limit %d", cfg.Width, cfg.Height, MaxDecodedImagePixels)
	}
	return cfg, nil
}

// Decode decodes an image from raw bytes. The output is premultiplied RGBA at
// the original resolution; the caller is responsible for any downscaling.
func Decode(data []byte) (gimage.Image, error) {
	if _, err := Probe(data); err != nil {
		return nil, err
	}
	r := bytes.NewReader(data)
	img, _, err := gimage.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("image: decode: %w", err)
	}
	b := img.Bounds()
	if b.Dx() > MaxImageDimension || b.Dy() > MaxImageDimension {
		return nil, fmt.Errorf("image: decoded dimensions %dx%d exceed limit %d", b.Dx(), b.Dy(), MaxImageDimension)
	}
	if int64(b.Dx()) > MaxDecodedImagePixels/int64(b.Dy()) {
		return nil, fmt.Errorf("image: decoded pixel count %dx%d exceeds limit %d", b.Dx(), b.Dy(), MaxDecodedImagePixels)
	}
	return img, nil
}

// DecodePNG decodes a PNG image with metadata admission.
func DecodePNG(r io.Reader) (gimage.Image, error) {
	limited := io.LimitReader(r, MaxEncodedImageBytes+1)
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, limited); err != nil {
		return nil, fmt.Errorf("image: read png: %w", err)
	}
	if buf.Len() > MaxEncodedImageBytes {
		return nil, fmt.Errorf("image: encoded size exceeds limit %d", MaxEncodedImageBytes)
	}
	data := buf.Bytes()
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: png config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageDimension || cfg.Height > MaxImageDimension {
		return nil, fmt.Errorf("image: png dimensions %dx%d exceed limit %d", cfg.Width, cfg.Height, MaxImageDimension)
	}
	if int64(cfg.Width) > MaxDecodedImagePixels/int64(cfg.Height) {
		return nil, fmt.Errorf("image: png pixel count %dx%d exceeds limit %d", cfg.Width, cfg.Height, MaxDecodedImagePixels)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: png decode: %w", err)
	}
	return img, nil
}

// DecodeJPEG decodes a JPEG image with metadata admission.
func DecodeJPEG(r io.Reader) (gimage.Image, error) {
	limited := io.LimitReader(r, MaxEncodedImageBytes+1)
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, limited); err != nil {
		return nil, fmt.Errorf("image: read jpeg: %w", err)
	}
	if buf.Len() > MaxEncodedImageBytes {
		return nil, fmt.Errorf("image: encoded size exceeds limit %d", MaxEncodedImageBytes)
	}
	data := buf.Bytes()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: jpeg config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageDimension || cfg.Height > MaxImageDimension {
		return nil, fmt.Errorf("image: jpeg dimensions %dx%d exceed limit %d", cfg.Width, cfg.Height, MaxImageDimension)
	}
	if int64(cfg.Width) > MaxDecodedImagePixels/int64(cfg.Height) {
		return nil, fmt.Errorf("image: jpeg pixel count %dx%d exceeds limit %d", cfg.Width, cfg.Height, MaxDecodedImagePixels)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image: jpeg decode: %w", err)
	}
	return img, nil
}

// EncodePNG encodes an image as PNG.
func EncodePNG(w io.Writer, img gimage.Image) error {
	return png.Encode(w, img)
}

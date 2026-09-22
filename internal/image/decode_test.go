package image

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func smallPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func smallJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func smallGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeSmallValid(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"png", smallPNG(t)},
		{"jpeg", smallJPEG(t)},
		{"gif", smallGIF(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img, err := Decode(tc.data)
			if err != nil {
				t.Fatalf("Decode(%s) = %v", tc.name, err)
			}
			if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 4 {
				t.Errorf("Decode(%s) bounds = %v, want 4x4", tc.name, img.Bounds())
			}
		})
	}
}

func TestProbeRejectsOversizedDimensions(t *testing.T) {
	cfg := image.Config{Width: 8193, Height: 1, ColorModel: nil}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))); err != nil {
		t.Fatal(err)
	}
	_, err := Probe(buf.Bytes())
	if err == nil {
		t.Fatal("Probe accepted 8193x1 image")
	}
}

func TestProbeAcceptsAtLimit(t *testing.T) {
	cfg := image.Config{Width: 8192, Height: 2048}
	if int64(cfg.Width)*int64(cfg.Height) > MaxDecodedImagePixels {
		t.Fatal("test setup: 8192x2048 exceeds pixel limit")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))); err != nil {
		t.Fatal(err)
	}
	got, err := Probe(buf.Bytes())
	if err != nil {
		t.Fatalf("Probe rejected 8192x2048: %v", err)
	}
	if got.Width != cfg.Width || got.Height != cfg.Height {
		t.Errorf("Probe config = %dx%d, want %dx%d", got.Width, got.Height, cfg.Width, cfg.Height)
	}
}

func TestProbeRejectsJustOverPixelLimit(t *testing.T) {
	cfg := image.Config{Width: 8192, Height: 2049}
	if int64(cfg.Width)*int64(cfg.Height) <= MaxDecodedImagePixels {
		t.Fatal("test setup: 8192x2049 should exceed pixel limit")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))); err != nil {
		t.Fatal(err)
	}
	_, err := Probe(buf.Bytes())
	if err == nil {
		t.Fatal("Probe accepted 8192x2049 image")
	}
}

func TestProbeRejectsEncodedByteLimit(t *testing.T) {
	data := make([]byte, MaxEncodedImageBytes+1)
	copy(data, []byte{0x89, 'P', 'N', 'G'})
	_, err := Probe(data)
	if err == nil {
		t.Fatal("Probe accepted oversized encoded data")
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	_, err := Decode([]byte("not an image"))
	if err == nil {
		t.Fatal("Decode accepted malformed data")
	}
}

func TestDecodeRejectsTruncated(t *testing.T) {
	data := smallPNG(t)
	_, err := Decode(data[:len(data)/2])
	if err == nil {
		t.Fatal("Decode accepted truncated PNG")
	}
}

func TestDecodePNGRejectsOversized(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8193, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	_, err := DecodePNG(&buf)
	if err == nil {
		t.Fatal("DecodePNG accepted 8193x1 image")
	}
}

func TestDecodeJPEGRejectsOversized(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 8193))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	_, err := DecodeJPEG(&buf)
	if err == nil {
		t.Fatal("DecodeJPEG accepted 1x8193 image")
	}
}

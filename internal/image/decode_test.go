package image

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
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

func smallWebP(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("UklGRjwAAABXRUJQVlA4IDAAAADQAQCdASoEAAQAAUAmJaACdLoB+AADsAD+8ut//NgVzXPv9//S4P0uD9Lg/9KQAAA=")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func oversizedWebP(t *testing.T) []byte {
	t.Helper()
	return []byte{
		'R', 'I', 'F', 'F', 22, 0, 0, 0, 'W', 'E', 'B', 'P',
		'V', 'P', '8', 'X', 10, 0, 0, 0,
		0, 0, 0, 0, // flags and reserved bytes
		0, 32, 0, // canvas width minus one: 8192
		0, 0, 0, // canvas height minus one: 0
	}
}

func TestDecodeSmallValid(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"png", smallPNG(t)},
		{"jpeg", smallJPEG(t)},
		{"gif", smallGIF(t)},
		{"webp", smallWebP(t)},
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

// qc:BR-166
func TestProbeRejectsOversizedWebPDimensions(t *testing.T) {
	if _, err := Probe(oversizedWebP(t)); err == nil || !strings.Contains(err.Error(), "dimensions 8193x1") {
		t.Fatalf("Probe(8193x1 WebP) error = %v, want dimension admission error", err)
	}
}

// qc:BR-166
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

// qc:BR-165 BR-166
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

// qc:BR-166
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

// qc:BR-165
func TestProbeRejectsEncodedByteLimit(t *testing.T) {
	// A valid 4x4 PNG padded past the cap: the config probe would accept it, so
	// only a length check that runs before metadata decoding rejects it.
	data := append(smallPNG(t), make([]byte, MaxEncodedImageBytes)...)
	_, err := Probe(data)
	if err == nil || !strings.Contains(err.Error(), "encoded size") {
		t.Fatalf("Probe(%d bytes) = %v, want the encoded-size cap", len(data), err)
	}
}

// qc:BR-166
func TestProbeRejectsPixelBombFromHeaderOnlyPNG(t *testing.T) {
	// 4096x8192 RGBA would need 128 MiB decoded, but the file is only an IHDR.
	data := headerOnlyPNG(t, 4096, 8192)
	if len(data) > 128 {
		t.Fatalf("header-only fixture = %d bytes, want a small file", len(data))
	}
	_, err := Probe(data)
	if err == nil || !strings.Contains(err.Error(), "pixel count") {
		t.Fatalf("Probe(%d bytes declaring 4096x8192) = %v, want pixel-count admission", len(data), err)
	}
}

func headerOnlyPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(height))
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // color type RGBA
	writePNGChunk(&buf, "IHDR", ihdr)
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	buf.Write(length[:])
	buf.WriteString(typ)
	buf.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())
	buf.Write(sum[:])
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

// qc:BR-166
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

// qc:BR-166
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

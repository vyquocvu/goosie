package engine_test

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestPictureElementSourceResolution(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()

	var requestedURL string
	fetcher := func(base, url string) ([]byte, error) {
		requestedURL = url
		return pngBytes, nil
	}

	html := `<html><body style="margin: 0;">
<picture>
  <source media="(max-width: 500px)" srcset="/mobile.png">
  <source srcset="/desktop.png">
  <img id="my-pic" src="/fallback.png" style="width: 100px; height: 100px;">
</picture>
</body></html>`

	// 1. Mobile viewport (400px wide) -> should choose /mobile.png
	requestedURL = ""
	sMobile, err := engine.NewSession(html, nil, 400,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	_ = sMobile
	if !strings.HasSuffix(requestedURL, "/mobile.png") {
		t.Errorf("at viewport 400px, expected /mobile.png, got %q", requestedURL)
	}

	// 2. Desktop viewport (800px wide) -> should choose /desktop.png
	requestedURL = ""
	sDesktop, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	_ = sDesktop
	if !strings.HasSuffix(requestedURL, "/desktop.png") {
		t.Errorf("at viewport 800px, expected /desktop.png, got %q", requestedURL)
	}
}

func TestPictureTypeFallback(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()

	var requestedURL string
	fetcher := func(base, url string) ([]byte, error) {
		requestedURL = url
		return pngBytes, nil
	}

	html := `<html><body style="margin: 0;">
<picture>
  <source type="image/unsupported-fmt" srcset="/bad.fmt">
  <source type="image/png" srcset="/good.png">
  <img id="my-pic2" src="/fallback.png" style="width: 100px; height: 100px;">
</picture>
</body></html>`

	s, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	_ = s
	if !strings.HasSuffix(requestedURL, "/good.png") {
		t.Errorf("expected supported type image /good.png, got %q", requestedURL)
	}
}

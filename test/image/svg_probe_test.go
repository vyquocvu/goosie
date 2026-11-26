package image_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/image"
)

// qc:BR-330
func TestUnsizedSVGProbeUsesDefaultWidthAndViewBoxRatio(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 140 22"><rect width="140" height="22"/></svg>`)
	cfg, err := image.Probe(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 300 || cfg.Height != 47 {
		t.Errorf("probe = %dx%d, want 300x47 (default width at the 140:22 viewBox ratio)", cfg.Width, cfg.Height)
	}
}

// qc:BR-330
func TestExplicitSVGSizeStillWinsOverViewBox(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 140 22"></svg>`)
	cfg, err := image.Probe(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 24 || cfg.Height != 24 {
		t.Errorf("probe = %dx%d, want 24x24", cfg.Width, cfg.Height)
	}
}

package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestHugeNonGeometryNumbersAdmitted guards the admission gate's scope: huge
// plain numbers (counters, z-index, order) are legal CSS and must not refuse
// the document. Only dimensions and percentages feed geometry, so only they
// trip the magnitude cap. A 2-billion counter reset used to blank the page.
func TestHugeNonGeometryNumbersAdmitted(t *testing.T) {
	html := `<html><head><style>.x{counter-reset: c 2000000000; z-index: 999999999;}</style></head><body><div class="x">hi</div></body></html>`
	if _, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7})); err != nil {
		t.Fatalf("NewSession with huge counter/z-index = %v, want admitted", err)
	}
}

// TestHugeGeometryStillRefused pins the other half: enormous lengths keep
// refusing the document, since they would explode layout arithmetic.
func TestHugeGeometryStillRefused(t *testing.T) {
	html := `<html><head><style>.x{width: 999999999999px;}</style></head><body><div class="x">hi</div></body></html>`
	if _, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7})); err == nil {
		t.Fatal("NewSession with gigantic width admitted, want refused")
	}
}

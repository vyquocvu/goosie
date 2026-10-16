package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
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

func TestUnicodeRangeIsNotGeometry(t *testing.T) {
	html := `<style>@font-face{font-family:test;src:url(test.woff2);unicode-range:U+1E80-1EFF}.x{width:100px}</style><div class="x">word</div>`
	if _, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7})); err != nil {
		t.Fatalf("NewSession with hexadecimal unicode-range = %v, want admitted", err)
	}
}

func TestOversizedBorderRadiusIsClampedDuringPaint(t *testing.T) {
	html := `<style>.pill{width:100px;height:40px;background:red;border-radius:3.40282e38px}</style><div class="pill"></div>`
	s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession with large pill radius = %v, want admitted", err)
	}
	list, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked with large pill radius = %v", err)
	}
	for _, cmd := range list.All() {
		if cmd.Kind == paint.CmdFill && cmd.Radius.TL.X > 0 {
			if cmd.Radius.TL.X > 20.01 || cmd.Radius.TL.Y > 20.01 {
				t.Fatalf("rounded radius = %+v, want normalized within the 100x40 box", cmd.Radius.TL)
			}
			return
		}
	}
	t.Fatal("no rounded fill command emitted")
}

package renderer

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestCalcEval(t *testing.T) {
	tests := []struct {
		expr     string
		pct      float32
		fontSize float32
		vw, vh   float32
		want     float32
	}{
		{"calc(100% - 160px)", 800, 16, 1024, 768, 640},
		{"calc(50% + 20px)", 400, 16, 1024, 768, 220},
		{"min(200px, 50%)", 300, 16, 1024, 768, 150},
		{"max(100px, 50%)", 300, 16, 1024, 768, 150},
		{"clamp(100px, 50%, 500px)", 300, 16, 1024, 768, 150},
		{"clamp(100px, 10%, 500px)", 300, 16, 1024, 768, 100},
	}
	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			got := evalCalcExpr(tc.expr, tc.fontSize, tc.vw, tc.vh, tc.pct)
			assert.InDelta(t, tc.want, got, 1.0)
		})
	}
}

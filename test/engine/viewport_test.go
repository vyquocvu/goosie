package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestValidateViewport(t *testing.T) {
	for _, c := range []struct {
		w, h  int
		dpr   float64
		valid bool
	}{
		{800, 600, 1, true}, {4096, 4096, 1, true}, {16384, 1024, 1, true}, {2048, 128, 8, true},
		{0, 600, 1, false}, {800, -1, 1, false}, {800, 600, 0, false}, {800, 600, 9, false},
		{800, 600, math.NaN(), false}, {800, 600, math.Inf(1), false},
		{math.MaxInt, 600, 1, false}, {16385, 1, 1, false}, {4097, 4096, 1, false},
		{1, 1, 0.01, false}, {1 << 21, 1, 0.001, false},
		{16384, 1024, 1.000001, false},
	} {
		err := engine.ValidateViewport(c.w, c.h, c.dpr)
		if (err == nil) != c.valid {
			t.Errorf("ValidateViewport(%d,%d,%v)=%v, valid=%v", c.w, c.h, c.dpr, err, c.valid)
		}
	}
}

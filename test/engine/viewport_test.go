package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
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

func TestValidateExtentCountsSignedTileSpans(t *testing.T) {
	cases := []struct {
		name  string
		rect  frame.Rect
		valid bool
	}{
		{"aligned limit", frame.Rect4(0, 0, 65536, 65536), true},
		{"one extra column", frame.Rect4(0, 0, 65537, 65536), false},
		{"negative partial tiles", frame.Rect4(-1, -1, 65535, 65535), false},
		{"one negative tile", frame.Rect4(-256, -256, 0, 0), true},
		{"empty", frame.Rect4(0, 0, 0, 0), true},
		{"inverted", frame.Rect4(1, 0, 0, 1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := engine.ValidateExtent(tc.rect); (err == nil) != tc.valid {
				t.Fatalf("ValidateExtent(%v) = %v; valid=%v", tc.rect, err, tc.valid)
			}
		})
	}
}

func TestTileCacheBudget(t *testing.T) {
	cases := []struct {
		name      string
		rect      frame.Rect
		wantTiles int
		wantErr   bool
	}{
		{"small retains headroom", frame.Rect4(0, 0, 256, 256), 9, false},
		{"empty gets positive cap", frame.Rect4(0, 0, 0, 0), 8, false},
		{"inverted errors", frame.Rect4(1, 0, 0, 1), 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tiles, bytes, err := engine.TileCacheBudget(tc.rect)
			if (err != nil) != tc.wantErr {
				t.Fatalf("TileCacheBudget(%v) err=%v, wantErr=%v", tc.rect, err, tc.wantErr)
			}
			if tc.wantErr {
				if tiles != 0 || bytes != 0 {
					t.Fatalf("TileCacheBudget(%v) = (%d, %d); want (0, 0) on error", tc.rect, tiles, bytes)
				}
				return
			}
			if tiles != tc.wantTiles {
				t.Errorf("TileCacheBudget(%v) tiles=%d, want %d", tc.rect, tiles, tc.wantTiles)
			}
			if bytes != int64(tiles)*frame.TileSizeBytes() {
				t.Errorf("TileCacheBudget(%v) bytes=%d, want %d", tc.rect, bytes, int64(tiles)*frame.TileSizeBytes())
			}
		})
	}
}

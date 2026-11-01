package main

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
)

func TestFrameOutcome(t *testing.T) {
	tests := []struct {
		name    string
		stats   surface.LoopStats
		done    bool
		wantErr string
	}{
		{
			name:  "before the first present",
			stats: surface.LoopStats{},
		},
		{
			name:  "settled",
			stats: surface.LoopStats{Presents: 1},
			done:  true,
		},
		{
			name:  "still needs tiles",
			stats: surface.LoopStats{Presents: 1, Last: surface.FrameWork{Needed: 2}},
		},
		{
			name:  "buffers still on order",
			stats: surface.LoopStats{Presents: 1, Last: surface.FrameWork{PaintingBytes: 256}},
		},
		{
			name:  "a refused tile means the frame is not settled",
			stats: surface.LoopStats{Presents: 1, Last: surface.FrameWork{Refused: 1}},
		},
		{
			name:    "a visible failed tile is terminal",
			stats:   surface.LoopStats{Presents: 1, Last: surface.FrameWork{Failed: 2}},
			wantErr: "2 visible tile(s) failed rasterization",
		},
		{
			name:    "failure outranks a settled counter set",
			stats:   surface.LoopStats{Presents: 1, Last: surface.FrameWork{Failed: 1}},
			wantErr: "1 visible tile(s) failed rasterization",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done, err := frameOutcome(tt.stats)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("frameOutcome() error = %v, want nil", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("frameOutcome() error = %v, want containing %q", err, tt.wantErr)
			}
			if done != tt.done {
				t.Fatalf("frameOutcome() done = %v, want %v", done, tt.done)
			}
		})
	}
}

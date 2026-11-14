package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
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

const revealPage = `<!DOCTYPE html><html><head><style>
html,body{margin:0;padding:0}
#box{position:absolute;left:0;top:0;width:100px;height:100px;background:#0000ff}
</style></head><body><div id="box"></div>
<script>
setTimeout(function(){ document.getElementById("box").style.backgroundColor = "#ff0000"; }, 120);
</script></body></html>`

func renderPNG(t *testing.T, html string, extra ...string) image.Image {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "page.html")
	out := filepath.Join(dir, "page.png")
	if err := os.WriteFile(in, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-in", in, "-out", out, "-width", "120", "-height", "80", "-dpr", "1"}
	args = append(args, extra...)
	if err := run(args); err != nil {
		t.Fatalf("run(%v): %v", args, err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	return img
}

func rgb(t *testing.T, img image.Image, x, y int) (uint32, uint32, uint32) {
	t.Helper()
	r, g, b, _ := img.At(x, y).RGBA()
	return r >> 8, g >> 8, b >> 8
}

// qc:BR-261
func TestPumpRunsTimersBeforePainting(t *testing.T) {
	img := renderPNG(t, revealPage, "-pump", "400ms")
	r, g, b := rgb(t, img, 10, 10)
	if r != 0xff || g != 0 || b != 0 {
		t.Fatalf("background after -pump = #%02x%02x%02x, want #ff0000 (timer did not paint)", r, g, b)
	}
}

// qc:BR-261
func TestWithoutPumpTimersDoNotRun(t *testing.T) {
	img := renderPNG(t, revealPage)
	r, g, b := rgb(t, img, 10, 10)
	if r != 0 || g != 0 || b != 0xff {
		t.Fatalf("background without -pump = #%02x%02x%02x, want the initial #0000ff", r, g, b)
	}
}

// qc:BR-261
func TestPumpRejectsOutOfRangeBudget(t *testing.T) {
	for _, budget := range []string{"-1ms", "11s"} {
		err := run([]string{"-in", "page.html", "-out", "page.png", "-pump", budget})
		if err == nil || !strings.Contains(err.Error(), "-pump") {
			t.Fatalf("-pump %s error = %v, want a -pump usage error", budget, err)
		}
	}
}

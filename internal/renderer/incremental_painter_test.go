package renderer

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

func makeTestCommands() []raster.DisplayCmd {
	return []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 200, H: 200},
			Color: frame.White,
		},
		{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 10, Y: 10, W: 50, H: 50},
			Color: frame.Black,
		},
	}
}

func TestIncrementalPainterPaintFull(t *testing.T) {
	p, err := NewIncrementalPainter(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	img, err := p.PaintFull(makeTestCommands())
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestIncrementalPainterPaintDirtyLimitsDirtyRects(t *testing.T) {
	p, err := NewIncrementalPainter(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dirty := []frame.Rect{{X: 10, Y: 10, W: 50, H: 50}}
	img, err := p.PaintDirty(nil, makeTestCommands(), dirty)
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("expected non-nil image from dirty paint")
	}
}

func BenchmarkIncrementalPainterPaintFull(b *testing.B) {
	p, err := NewIncrementalPainter(800, 600)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()
	cmds := makeTestCommands()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.PaintFull(cmds); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIncrementalPainterPaintDirty(b *testing.B) {
	p, err := NewIncrementalPainter(800, 600)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()
	cmds := makeTestCommands()
	dirty := []frame.Rect{{X: 10, Y: 10, W: 50, H: 50}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.PaintDirty(nil, cmds, dirty); err != nil {
			b.Fatal(err)
		}
	}
}

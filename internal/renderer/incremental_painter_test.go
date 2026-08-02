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

func TestIncrementalPainterPresentPushesFrame(t *testing.T) {
	p, err := NewIncrementalPainter(64, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	adapter := NewFyneAdapter()
	if _, err := p.PaintFull(makeTestCommands()); err != nil {
		t.Fatal(err)
	}
	p.Present(adapter)
	if adapter.CurrentFrame() == nil {
		t.Fatal("expected frame buffer to be presented")
	}
}

func TestIncrementalPainterPaintDirtyLimitsDirtyRects(t *testing.T) {
	p, err := NewIncrementalPainter(200, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	chunks := []PaintChunk{
		{Owner: LayoutID(1), Start: 0, End: 1, Bounds: RectF{X: 10, Y: 10, W: 50, H: 50}, dirty: true},
	}
	img, err := p.PaintDirty(chunks, makeTestCommands())
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("expected non-nil image from dirty paint")
	}
}

func TestDirtyRectUnionsChunkBounds(t *testing.T) {
	chunks := []PaintChunk{
		{Owner: LayoutID(1), Start: 0, End: 1, Bounds: RectF{X: 0, Y: 0, W: 50, H: 50}, dirty: true},
		{Owner: LayoutID(2), Start: 1, End: 2, Bounds: RectF{X: 60, Y: 70, W: 30, H: 30}},
		{Owner: LayoutID(3), Start: 2, End: 3, Bounds: RectF{X: 80, Y: 90, W: 40, H: 40}, dirty: true},
	}
	dirty := DirtyRect(chunks)
	if dirty.W != 120 || dirty.H != 130 {
		t.Fatalf("expected union bounds, got %+v", dirty)
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
	chunks := []PaintChunk{
		{Owner: LayoutID(1), Start: 0, End: 1, Bounds: RectF{X: 10, Y: 10, W: 50, H: 50}, dirty: true},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.PaintDirty(chunks, cmds); err != nil {
			b.Fatal(err)
		}
	}
}

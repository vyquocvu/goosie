package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

func TestTiledRasterizer_SingleTile(t *testing.T) {
	// A small frame that fits in a single tile should still work
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(1024))
	vp := frame.NewViewport(100, 50, frame.PixelScaleDefault)

	cmds := []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 100, H: 50},
			Color: frame.NewColor(255, 0, 0, 255),
		},
	}

	img, err := tr.RasterizeParallel(100, 50, cmds, nil, vp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 50 {
		t.Errorf("expected 100x50, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestTiledRasterizer_MultipleTiles(t *testing.T) {
	// A tall frame that spans multiple tiles
	tileH := 100
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(tileH), raster.WithMaxWorkers(2))
	vp := frame.NewViewport(50, 350, frame.PixelScaleDefault)

	cmds := []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 50, H: 350},
			Color: frame.NewColor(0, 0, 255, 255),
		},
	}

	img, err := tr.RasterizeParallel(50, 350, cmds, nil, vp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	if img.Bounds().Dx() != 50 || img.Bounds().Dy() != 350 {
		t.Errorf("expected 50x350, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Verify some pixels are painted (not all transparent)
	hasColor := false
	for y := 0; y < 350; y += 50 {
		for x := 0; x < 50; x += 10 {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && (r > 0 || g > 0 || b > 0) {
				hasColor = true
				break
			}
		}
		if hasColor {
			break
		}
	}
	if !hasColor {
		t.Error("expected some painted pixels, got all transparent")
	}
}

func TestTiledRasterizer_Fallback_SingleThread(t *testing.T) {
	// With maxWorkers=1, should fall back to single-threaded
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(100), raster.WithMaxWorkers(1))
	vp := frame.NewViewport(50, 300, frame.PixelScaleDefault)

	cmds := []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: 0, W: 50, H: 300},
			Color: frame.NewColor(0, 255, 0, 255),
		},
	}

	img, err := tr.RasterizeParallel(50, 300, cmds, nil, vp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestTiledRasterizer_EmptyCommands(t *testing.T) {
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(100))
	vp := frame.NewViewport(50, 200, frame.PixelScaleDefault)

	img, err := tr.RasterizeParallel(50, 200, nil, nil, vp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image even with no commands")
	}
}

func TestTiledRasterizer_ZeroDimensions(t *testing.T) {
	tr := raster.NewTiledRasterizer()
	vp := frame.NewViewport(1, 1, frame.PixelScaleDefault)

	// Zero width/height should be normalized to 1
	img, err := tr.RasterizeParallel(0, 0, nil, nil, vp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if img == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestTiledRasterizer_ReturnsRGBA(t *testing.T) {
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(50))
	vp := frame.NewViewport(10, 10, frame.PixelScaleDefault)

	img, err := tr.RasterizeParallel(10, 10, nil, nil, vp)
	if err != nil {
		t.Fatal(err)
	}
	if img == nil {
		t.Fatal("expected non-nil *image.RGBA")
	}
	// RasterizeParallel returns *image.RGBA directly
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Errorf("expected 10x10, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func BenchmarkTiledRasterize_SingleCore(b *testing.B) {
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(256), raster.WithMaxWorkers(1))
	vp := frame.NewViewport(800, 2000, frame.PixelScaleDefault)

	cmds := make([]raster.DisplayCmd, 100)
	for i := range cmds {
		cmds[i] = raster.DisplayCmd{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: float32(i * 20), W: 800, H: 20},
			Color: frame.NewColor(uint8(i%256), uint8(i%128), uint8(i%64), 255),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		img, err := tr.RasterizeParallel(800, 2000, cmds, nil, vp)
		if err != nil {
			b.Fatal(err)
		}
		raster.PutBuffer(img)
	}
}

func BenchmarkTiledRasterize_MultiCore(b *testing.B) {
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(256), raster.WithMaxWorkers(4))
	vp := frame.NewViewport(800, 2000, frame.PixelScaleDefault)

	cmds := make([]raster.DisplayCmd, 100)
	for i := range cmds {
		cmds[i] = raster.DisplayCmd{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: float32(i * 20), W: 800, H: 20},
			Color: frame.NewColor(uint8(i%256), uint8(i%128), uint8(i%64), 255),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		img, err := tr.RasterizeParallel(800, 2000, cmds, nil, vp)
		if err != nil {
			b.Fatal(err)
		}
		raster.PutBuffer(img)
	}
}

func BenchmarkTiledRasterize_LargePage(b *testing.B) {
	tr := raster.NewTiledRasterizer(raster.WithTileHeight(512))
	vp := frame.NewViewport(1024, 5000, frame.PixelScaleDefault)

	cmds := make([]raster.DisplayCmd, 500)
	for i := range cmds {
		cmds[i] = raster.DisplayCmd{
			Kind:  raster.CmdFill,
			Rect:  frame.Rect{X: 0, Y: float32(i * 10), W: 1024, H: 10},
			Color: frame.NewColor(uint8(i%256), 128, 64, 255),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		img, err := tr.RasterizeParallel(1024, 5000, cmds, nil, vp)
		if err != nil {
			b.Fatal(err)
		}
		raster.PutBuffer(img)
	}
}

// Command goosie-headless renders an HTML file to a PNG image.
//
// This is the M2 front-end pipeline's file-to-pixels path: read HTML, parse DOM,
// resolve styles, lay out boxes, paint a display list, rasterize tiles, and write
// the result as a PNG. It exists so automated tests can compare rendered output
// against baselines and against Chromium.
//
// Usage:
//
//	goosie-headless -in page.html -out page.png
//	goosie-headless -in page.html -out page.png -width 1440 -height 900 -dpr 2
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

const (
	// rasterQueue is how many tile jobs may be offered at once. The same depth as
	// the main binary, so a headless render never refuses work an interactive run
	// would have taken.
	rasterQueue = 4096
	// glyphAtlasBudget is per layer and is not one of M2's criteria. A tile is
	// 256px on a side, so 4 MiB holds more glyph masks than one screen of text needs.
	glyphAtlasBudget = 4 << 20
	// renderTimeout is how long to wait for the frame to be presented. A headless
	// render should complete in milliseconds, but a slow machine or large page
	// might need more.
	renderTimeout = 10 * time.Second
)

// config is one parsed invocation.
type config struct {
	in     string
	out    string
	width  int
	height int
	dpr    float64
}

// devSize is the surface in device pixels.
func (c config) devSize() frame.Size {
	return frame.Size{
		W: int32(float64(c.width) * c.dpr + 0.5),
		H: int32(float64(c.height) * c.dpr + 0.5),
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "goosie-headless: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var c config
	fs := flag.NewFlagSet("goosie-headless", flag.ContinueOnError)
	fs.StringVar(&c.in, "in", "", "HTML file to render (required)")
	fs.StringVar(&c.out, "out", "", "PNG file to write (required)")
	fs.IntVar(&c.width, "width", 1440, "viewport width in CSS pixels")
	fs.IntVar(&c.height, "height", 900, "viewport height in CSS pixels")
	fs.Float64Var(&c.dpr, "dpr", 2, "device pixel ratio")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: goosie-headless -in page.html -out page.png [flags]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "flags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if c.in == "" {
		return fmt.Errorf("-in is required")
	}
	if c.out == "" {
		return fmt.Errorf("-out is required")
	}
	if c.width <= 0 || c.height <= 0 {
		return fmt.Errorf("-width and -height must be positive")
	}
	if c.dpr <= 0 || c.dpr > 8 {
		return fmt.Errorf("-dpr must be in (0, 8]")
	}

	html, err := os.ReadFile(c.in)
	if err != nil {
		return fmt.Errorf("read %s: %w", c.in, err)
	}

	doc := dom.Parse(string(html))
	authorCSS := extractStyleSheets(doc)

	dev := c.devSize()
	scale := float32(c.dpr)

	fonts, err := raster.NewFonts()
	if err != nil {
		return fmt.Errorf("init fonts: %w", err)
	}

	sess, err := engine.NewSession(string(html), authorCSS, float32(c.width), engine.WithMetrics(fonts))
	if err != nil {
		return fmt.Errorf("build session: %w", err)
	}

	list := sess.Paint(scale)
	dl := list.Build(1)
	extent := dl.Extent()

	// The grid must be a tile grid: TileSize squares with a pool of matching
	// buffers, not dev-sized tiles. Budget covers the document plus headroom.
	docTiles := int64((extent.W()+frame.TileSize-1)/frame.TileSize) *
		int64((extent.H()+frame.TileSize-1)/frame.TileSize)
	budgetTiles := docTiles + 8
	bitmapPool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(budgetTiles))
	layerBounds := frame.Rect{X0: 0, Y0: 0, X1: extent.X1, Y1: extent.Y1}
	layer := frame.NewLayer(1, layerBounds, budgetTiles*frame.TileSizeBytes(), bitmapPool)
	layer.SetContent(dl)

	wp := raster.New(raster.DefaultWorkers(), rasterQueue, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(glyphAtlasBudget, fonts)))
	wp.Start(context.Background())
	defer wp.Close()

	sched := raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{})
	sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: frame.RGB(248, 248, 248)})

	composer := surface.NewComposer(dev, bitmapPool)
	rec := frame.NewFrameRecorder(2)

	win := headless.New(headless.Config{
		Size:  dev,
		Scale: scale,
	})
	defer win.Close()

	loop := surface.NewLoop(win, sched, composer, rec)

	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(ctx)
	}()

	win.Inject(surface.Event{Kind: surface.EvResize, Size: dev, Scale: scale})

	for {
		select {
		case <-time.After(10 * time.Millisecond):
			if win.Stats().Presents >= 2 {
				cancel()
				<-loopDone
				if err := win.WritePNG(c.out); err != nil {
					return fmt.Errorf("write PNG: %w", err)
				}
				stats := win.Stats()
				fmt.Printf("goosie-headless: rendered %s → %s (%dx%d, presents=%d)\n",
					c.in, c.out, dev.W, dev.H, stats.Presents)
				return nil
			}
		case err := <-loopDone:
			if err != nil && err != context.Canceled {
				return fmt.Errorf("loop: %w", err)
			}
			return fmt.Errorf("loop exited before presenting a frame")
		case <-ctx.Done():
			return fmt.Errorf("no frame presented within %v", renderTimeout)
		}
	}
}

func extractStyleSheets(doc *dom.Document) []string {
	var sheets []string
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Type == 1 && n.Data == "style" && n.FirstChild != nil {
			if n.FirstChild.Type == 2 {
				sheets = append(sheets, n.FirstChild.DataContent)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	return sheets
}

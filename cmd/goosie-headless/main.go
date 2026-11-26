package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

const (
	rasterQueue      = 4096
	glyphAtlasBudget = 4 << 20
	renderTimeout    = 10 * time.Second
	pumpStep         = 10 * time.Millisecond
)

type config struct {
	in     string
	out    string
	width  int
	height int
	dpr    float64
	pump   time.Duration
}

func (c config) devSize() frame.Size {
	return frame.Size{
		W: int32(float64(c.width)*c.dpr + 0.5),
		H: int32(float64(c.height)*c.dpr + 0.5),
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "goosie-headless: %v\n", err)
		os.Exit(1)
	}
}

func frameOutcome(stats surface.LoopStats) (bool, error) {
	if stats.Presents == 0 {
		return false, nil
	}
	if stats.Last.Failed > 0 {
		return false, fmt.Errorf("%d visible tile(s) failed rasterization", stats.Last.Failed)
	}
	return stats.Last.Needed == 0 && stats.Last.PaintingBytes == 0 && stats.Last.Refused == 0, nil
}

func mustJSRuntime(docURL string) *js.Runtime {
	rt, err := js.New(js.Options{URL: docURL})
	if err != nil {
		fmt.Fprintf(os.Stderr, "goosie-headless: js runtime: %v\n", err)
		return nil
	}
	return rt
}

func pumpJSFor(sess *engine.Session, viewportW float32, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	for {
		sess.PumpJS(viewportW)
		if sess.HasScriptTimeout() {
			return fmt.Errorf("script timed out during -pump after %d interrupt(s)", sess.ScriptTimeoutCount())
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil
		}
		wait := pumpStep
		if remaining < wait {
			wait = remaining
		}
		time.Sleep(wait)
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
	fs.DurationVar(&c.pump, "pump", 0, "run the JS event loop for this long before painting (e.g. 500ms); 0 paints once, without timers")
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
	if c.pump < 0 || c.pump > renderTimeout {
		return fmt.Errorf("-pump must be in [0, %v]", renderTimeout)
	}
	if err := engine.ValidateViewport(c.width, c.height, c.dpr); err != nil {
		return fmt.Errorf("viewport: %w", err)
	}

	f, err := os.Open(c.in)
	if err != nil {
		return fmt.Errorf("open %s: %w", c.in, err)
	}
	defer f.Close()
	limited := io.LimitReader(f, engine.MaxDocumentBytes+1)
	html, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read %s: %w", c.in, err)
	}
	if len(html) > engine.MaxDocumentBytes {
		return fmt.Errorf("document exceeds %d bytes", engine.MaxDocumentBytes)
	}

	dev := c.devSize()
	scale := float32(c.dpr)

	fonts, err := raster.NewFonts()
	if err != nil {
		return fmt.Errorf("init fonts: %w", err)
	}

	documentText, documentEncoding, _ := net.DecodeHTML(html, "")
	sess, err := engine.NewSession(documentText, nil, float32(c.width), engine.WithMetrics(fonts), engine.WithViewportH(float32(c.height)), engine.WithDocumentEncoding(documentEncoding), engine.WithJS(mustJSRuntime(c.in)), engine.WithInvalidation(&engine.Invalidation{}))
	if err != nil {
		return fmt.Errorf("build session: %w", err)
	}
	defer sess.Close()

	if c.pump > 0 {
		if err := pumpJSFor(sess, float32(c.width), c.pump); err != nil {
			return err
		}
	}

	list, err := sess.PaintChecked(scale)
	if err != nil {
		return fmt.Errorf("paint document: %w", err)
	}
	dl := list.Build(1)
	extent, err := sess.DocumentLayerBounds(dl.Extent(), scale, dev)
	if err != nil {
		return fmt.Errorf("document bounds: %w", err)
	}

	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return fmt.Errorf("size document cache: %w", err)
	}
	bitmapPool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, bitmapPool)
	layer.SetContent(dl)

	wp := raster.New(raster.DefaultWorkers(), rasterQueue, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(glyphAtlasBudget, fonts)))
	wp.Start(context.Background())
	defer wp.Close()

	sched := raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{})
	sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: sess.BackgroundColor()})

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
			done, err := frameOutcome(loop.Stats())
			if err != nil {
				return err
			}
			if done {
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

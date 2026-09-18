// Command goosie is v2's browser binary: the M1 frame path behind a window.
//
// M1 has no document loader, so what it draws is one of the synthetic scenes in
// paint: a checkerboard page with positioned text over it. That is not a placeholder
// in the binary's own terms - the frame path's criteria are all about tiles, budgets,
// and pacing, and a page whose contents are known analytically is what makes them
// measurable. The flags are the ones a real browser will have: the scene replaces the
// document, nothing else in the shape of the command changes.
//
// Two modes share one code path. Interactive mode opens a window and draws frames as
// the display asks for them. Gate mode (-gate) drives a fixed number of scroll frames
// through the same surface.Loop and reports the timings, which is how the nightly
// macOS job measures real vsync pacing without a finger on the glass.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

const (
	// gateScrollStep is -gate's per-frame scroll delta, in device pixels. It is the
	// same 100px the warm criterion in test/gate walks with, so the two runs
	// measure the same travel.
	gateScrollStep = 100
	// defaultFrames is -frames' default: the gate's 600-frame sweep.
	defaultFrames = 600
	// displayPeriod stands in for a 60Hz panel when the backend is paced by a clock
	// rather than by a display. A headless gate run measures the frame path against
	// the same interval the macOS shim will be held to.
	displayPeriod = 16 * time.Millisecond
	// benchPeriod is -bench's clock. It is not zero because the headless window paces
	// itself with a timer, and a period of zero means the default rather than as fast
	// as possible.
	benchPeriod = time.Microsecond
	// rasterQueue is how many tile jobs may be offered at once. The gate suite uses
	// the same depth, so a CLI run never refuses work a test run would have taken.
	rasterQueue = 4096
	// interactiveDocHeight is the document an interactive run scrolls, in device
	// pixels: the 20,000 px procedural page M1's first criterion names.
	interactiveDocHeight = 20_000
	// glyphAtlasBudget is per layer and is not one of M1's criteria. A tile is 256px
	// on a side, so 4 MiB holds more glyph masks than one screen of text needs.
	glyphAtlasBudget = 4 << 20
	// ringFrames is how many frame marks an interactive run keeps for its final
	// report: two seconds at 60Hz, which is the window the report is read against.
	ringFrames = 120
)

// errUsage marks a bad invocation, which the shell should see as exit status 2 rather
// than as a failure deep inside a run.
var errUsage = errors.New("usage")

// config is one parsed invocation.
type config struct {
	url     string
	gate    bool
	bench   bool
	backend string
	frames  int
	scene   string
	out     string
	width   int
	height  int
	dpr     float64
}

// devSize is the surface in device pixels: the units the frame path, the scheduler,
// and the shim all measure a viewport in.
func (c config) devSize() frame.Size {
	return frame.Size{
		W: int32(float64(c.width)*c.dpr + 0.5),
		H: int32(float64(c.height)*c.dpr + 0.5),
	}
}

// vsyncPeriod is the interval a clock-paced backend ticks at. A native backend is
// paced by its display and never reads this.
func (c config) vsyncPeriod() time.Duration {
	if c.bench {
		return benchPeriod
	}
	return displayPeriod
}

// paced reports whether this run is driven by a fixed frame count rather than by a
// person closing the window.
func (c config) paced() bool { return c.gate || c.bench }

// parse turns os.Args into a config, and rejects the combinations that would draw
// something other than what was asked for.
func parse(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet("goosie", flag.ContinueOnError)
	fs.StringVar(&c.url, "url", "", "document to load")
	fs.BoolVar(&c.gate, "gate", false, "drive -frames scripted scroll frames at real pacing, then report")
	fs.BoolVar(&c.bench, "bench", false, "run frames as fast as the clock allows, headless, and report the rate")
	fs.StringVar(&c.backend, "backend", "", "window backend: headless or native (empty means whatever this machine has)")
	fs.IntVar(&c.frames, "frames", defaultFrames, "frames to draw with -gate and -bench")
	fs.StringVar(&c.scene, "scene", "checkerboard", "synthetic document: checkerboard or plain")
	fs.StringVar(&c.out, "out", "", "write the frame report as JSON here, or - for stdout")
	fs.IntVar(&c.width, "width", 1440, "viewport width in CSS pixels")
	fs.IntVar(&c.height, "height", 900, "viewport height in CSS pixels")
	fs.Float64Var(&c.dpr, "dpr", 2, "device pixel ratio")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: goosie [-scene checkerboard] [-width 1440] [-height 900] [-dpr 2]")
		fmt.Fprintln(fs.Output(), "       goosie -gate -frames 600 -out gate.json")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, fmt.Errorf("%w: help requested", errUsage)
		}
		return c, fmt.Errorf("%w: %v", errUsage, err)
	}
	switch {
	case c.width <= 0 || c.height <= 0:
		return c, fmt.Errorf("%w: -width and -height must be positive", errUsage)
	case c.dpr <= 0 || c.dpr > 8:
		return c, fmt.Errorf("%w: -dpr must be in (0, 8]", errUsage)
	case c.frames <= 0:
		return c, fmt.Errorf("%w: -frames must be positive", errUsage)
	}
	if _, err := sceneSpec(c.scene); err != nil {
		return c, err
	}
	if c.bench {
		// A rate run that depended on a panel's refresh would report the panel, and
		// the nightly job wants the machine. An explicit -backend still wins, so a
		// caller who means to measure a real display can say so with -gate.
		if fs.Lookup("backend").Value.String() == "" {
			c.backend = platform.Headless
		}
	}
	return c, nil
}

// sceneSpec maps -scene to a document. Two scenes is the whole list because two is
// what the criteria distinguish: a page of flat colour exercises the grid and the
// budget, and only a page with positioned glyphs exercises the atlas.
func sceneSpec(name string) (paint.SceneSpec, error) {
	switch name {
	case "checkerboard":
		return paint.SceneSpec{Checkerboard: true, TextRuns: paint.DefaultTextRuns}, nil
	case "plain":
		return paint.SceneSpec{Checkerboard: false, TextRuns: paint.DefaultTextRuns}, nil
	}
	return paint.SceneSpec{}, fmt.Errorf("%w: unknown -scene %q (want checkerboard or plain)", errUsage, name)
}

// framePath is the assembled pipeline: everything between a vsync and a Present, with
// the pieces a report has to name kept reachable.
type framePath struct {
	window   surface.Window
	loop     *surface.Loop
	sched    *raster.Scheduler
	pool     *raster.Pool
	composer *surface.Composer
	layer    *frame.Layer
	rec      *frame.FrameRecorder
	spec     paint.SceneSpec
	config   config
	toolbar  *toolbar.State
	client   net.HTTP
	fonts    *raster.Fonts
	navSerial uint64
	// started is when frames began, so the reported rate excludes window creation and
	// the scene build: a run's frames-per-second is a claim about the frame path.
	started time.Time
}

// build wires a scene, a grid, a pool, a composer, a scheduler, and a window into a
// frame path sized for c.
func build(c config) (*framePath, error) {
	dev := c.devSize()
	scale := float32(c.dpr)

	fonts, err := raster.NewFonts()
	if err != nil {
		return nil, fmt.Errorf("goosie: %w", err)
	}

	client := net.DefaultClient()

	var layer *frame.Layer
	var spec paint.SceneSpec

	if c.url != "" {
		// Defer URL fetch until after the window opens. Start with a placeholder
		// layer so the user sees the toolbar immediately, not a blank wait.
		spec = paint.SceneSpec{DocHeight: dev.H}
		_, layer = paint.BuildLayer(spec)
	} else {
		spec, err = sceneSpec(c.scene)
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		cols := spec.Cols
		if cols <= 0 {
			cols = paint.DefaultCols
		}
		if c.paced() {
			if spec.DocHeight <= 0 {
				spec.DocHeight = paint.DefaultDocHeight
			}
			spec.BudgetTiles = int64((spec.DocHeight+frame.TileSize-1)/frame.TileSize)*int64(cols) + 8
		} else {
			spec.DocHeight = interactiveDocHeight
		}
		_, layer = paint.BuildLayer(spec)
	}

	wp := raster.New(raster.DefaultWorkers(), rasterQueue, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(glyphAtlasBudget, fonts)))
	wp.Start(context.Background())

	capacity := ringFrames
	if c.paced() {
		capacity = c.frames
	}
	f := &framePath{
		sched:    raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
		pool:     wp,
		composer: surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
		layer:    layer,
		rec:      frame.NewFrameRecorder(capacity),
		spec:     spec,
		config:   c,
		client:   client,
		fonts:    fonts,
	}
	if !c.paced() {
		f.toolbar = toolbar.NewState(dev.W, fonts)
		f.toolbar.OnNavigate = func(rawURL string) {
			f.navigate(rawURL)
		}
		if c.url != "" {
			f.toolbar.URL = normalizeURL(c.url)
			f.toolbar.Input = normalizeURL(c.url)
		}
		if cb := platform.NewClipboard(); cb != nil {
			f.toolbar.Clipboard = cb
		}
	}
	f.sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: frame.RGB(248, 248, 248)})
	return f, nil
}

// loadURL fetches a URL, builds a session and display list, and returns the layer,
// scene spec, and document background color for it. It is used both at startup
// for -url and by navigate for address bar submissions.
func loadURL(client net.HTTP, fonts *raster.Fonts, rawURL string, viewportW int, scale float32) (*frame.Layer, paint.SceneSpec, frame.Color, error) {
	u := normalizeURL(rawURL)
	resp, err := client.Get(context.Background(), u)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), fmt.Errorf("goosie: fetch %s: %w", u, err)
	}
	sess, err := engine.NewSession(string(resp.Body), nil, float32(viewportW), engine.WithMetrics(fonts))
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), fmt.Errorf("goosie: build session: %w", err)
	}
	list := sess.Paint(scale)
	dl := list.Build(1)
	extent := dl.Extent()
	docTiles := int64((extent.W()+frame.TileSize-1)/frame.TileSize) *
		int64((extent.H()+frame.TileSize-1)/frame.TileSize)
	budgetTiles := docTiles + 8
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(budgetTiles))
	layer := frame.NewLayer(frame.LayerID(budgetTiles), extent, budgetTiles*frame.TileSizeBytes(), pool)
	layer.SetContent(dl)
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), nil
}

// navigate loads a new URL on a goroutine and publishes a new plan when it
// arrives. It is called from the toolbar event pump, which runs on a different
// goroutine from the UI thread; SetPlan is the one scheduler method that is
// safe to call cross-thread, and the layer swap it triggers is applied in
// BeginFrame on the UI thread.
func (f *framePath) navigate(rawURL string) {
	go func() {
		layer, _, bgColor, err := loadURL(f.client, f.fonts, rawURL, f.config.width, float32(f.config.dpr))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		f.navSerial++
		f.toolbar.Navigate(normalizeURL(rawURL))
		f.sched.SetPlan(frame.FramePlan{
			Serial:     f.navSerial,
			Layers:     []*frame.Layer{layer},
			Background: bgColor,
		})
	}()
}


// normalizeURL adds https:// to bare domains so the fetcher has a scheme to dial.
func normalizeURL(raw string) string {
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "file://") {
		return raw
	}
	return "https://" + raw
}

// openWindow picks the window last, because a backend that cannot open one is a
// property of the machine the report has to state rather than a construction error.
func (f *framePath) openWindow() error {
	w, err := platform.Select(platform.Options{
		Backend:     f.config.backend,
		Size:        f.config.devSize(),
		Scale:       float32(f.config.dpr),
		VsyncPeriod: f.config.vsyncPeriod(),
		Title:       "Goosie",
	})
	if err != nil {
		return fmt.Errorf("goosie: %w", err)
	}
	if f.toolbar != nil {
		f.window = newToolbarWindow(w, f.toolbar)
	} else {
		f.window = w
	}
	f.loop = surface.NewLoop(f.window, f.sched, f.composer, f.rec)
	return nil
}

// main pins this goroutine to the process's main thread before anything can open a
// window, because AppKit requires the main thread and says so by aborting the process
// rather than by returning an error.
func main() {
	runtime.LockOSThread()
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "goosie:", err)
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	c, err := parse(args)
	if err != nil {
		return err
	}
	f, err := build(c)
	if err != nil {
		return err
	}
	defer func() { _ = f.pool.Close() }()
	defer func() { _ = f.client.Close() }()

	if err := f.openWindow(); err != nil {
		return err
	}
	defer func() { _ = f.window.Close() }()

	// If a URL was provided, start loading it now that the window is visible.
	// The user sees the toolbar immediately while the page fetches in the background.
	if c.url != "" && !c.paced() {
		f.navigate(c.url)
	}


	// A paced run cannot wait for a finger, so the driver stamps the scroll onto the
	// ticks the display already produces. Everything downstream of it - the loop, the
	// scheduler, the composer, the present - is the code an interactive run executes.
	var d *driver
	if c.paced() {
		d = newDriver(f, c.frames)
		f.loop = surface.NewLoop(d, f.sched, f.composer, f.rec)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopErr := make(chan error, 1)
	stopped := make(chan struct{})
	f.started = time.Now()
	go func() {
		err := f.loop.Run(ctx)
		loopErr <- err
		close(stopped)
	}()

	// Whoever ends the run says so here, and it cannot be the main thread's own
	// goroutine: on a platform with a run loop the main thread is busy turning it, and
	// closing the window is what stops that loop. An interactive run has no frame
	// count to finish on, so its finish channel stays nil and never readies.
	var finish chan struct{}
	if d != nil {
		finish = d.done
	}
	go func() {
		select {
		case <-stopped: // the window's events ran out, which is an orderly shutdown
		case <-sig:
		case <-finish: // the paced run reached its frame count
		}
		// Cancel, then wait for the loop to leave the frame it is drawing, then close.
		// Closing first would put a Present against a closed window in front of the
		// loop's next look at the context, and a run that ended because it finished its
		// frames would exit non-zero for a present nobody asked for.
		cancel()
		<-stopped
		_ = f.window.Close()
	}()

	if r, ok := f.window.(platform.Runner); ok {
		// A backend that owns a run loop gets this thread, which is the one pinned to
		// the process's main thread. Returning from Run means the window is gone.
		r.Run()
	}
	<-stopped
	if err := <-loopErr; err != nil {
		return fmt.Errorf("goosie: %w", err)
	}
	return f.report(d)
}

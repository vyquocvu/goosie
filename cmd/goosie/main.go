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
	"math"
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
	"github.com/vyquocvu/goosie/internal/tabs"
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

// navResult is the outcome of a per-tab navigation, delivered on navResults.
type navResult struct {
	tabID   uint64
	serial  uint64
	layer   *frame.Layer
	bgColor frame.Color
	session *engine.Session
	err     error
	url     string
}

// errUsage marks a bad invocation, which the shell should see as exit status 2 rather
// than as a failure deep inside a run.
var errUsage = errors.New("usage")

// config is one parsed invocation.
type config struct {
	url        string
	gate       bool
	bench      bool
	screenshot bool
	backend    string
	frames     int
	scene      string
	out        string
	width      int
	height     int
	dpr        float64
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
func (c config) paced() bool { return c.gate || c.bench || c.screenshot }

// parse turns os.Args into a config, and rejects the combinations that would draw
// something other than what was asked for.
func parse(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet("goosie", flag.ContinueOnError)
	fs.StringVar(&c.url, "url", "", "document to load")
	fs.BoolVar(&c.gate, "gate", false, "drive -frames scripted scroll frames at real pacing, then report")
	fs.BoolVar(&c.bench, "bench", false, "run frames as fast as the clock allows, headless, and report the rate")
	fs.BoolVar(&c.screenshot, "screenshot", false, "render -url to a PNG at -out and exit")
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
	case math.IsNaN(c.dpr) || c.dpr <= 0 || c.dpr > 8:
		return c, fmt.Errorf("%w: -dpr must be in (0, 8]", errUsage)
	case c.frames <= 0:
		return c, fmt.Errorf("%w: -frames must be positive", errUsage)
	}
	if _, err := sceneSpec(c.scene); err != nil {
		return c, err
	}
	if c.bench || c.screenshot {
		// A rate run that depended on a panel's refresh would report the panel, and
		// the nightly job wants the machine. An explicit -backend still wins, so a
		// caller who means to measure a real display can say so with -gate.
		if fs.Lookup("backend").Value.String() == "" {
			c.backend = platform.Headless
		}
	}
	if c.screenshot {
		if c.out == "" {
			return c, fmt.Errorf("%w: -screenshot requires -out <path.png>", errUsage)
		}
		if c.url == "" {
			return c, fmt.Errorf("%w: -screenshot requires -url", errUsage)
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
	tabMgr   *tabs.TabManager
	client   net.HTTP
	fonts    *raster.Fonts
	started    time.Time
	navResults chan navResult
	zoom     float64
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
		sched:      raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
		pool:       wp,
		composer:   surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
		layer:      layer,
		rec:        frame.NewFrameRecorder(capacity),
		spec:       spec,
		config:     c,
		client:     client,
		fonts:      fonts,
		navResults: make(chan navResult, 16),
		zoom:       1.0,
	}
	if !c.paced() {
		go f.drainNavResults()
	}
	if !c.paced() {
		f.toolbar = toolbar.NewState(dev.W, fonts)
		f.tabMgr = tabs.NewManager(func() {
			if f.window != nil {
				_ = f.window.Close()
			}
		})
		f.tabMgr.OnChange = func() {
			f.syncTabToToolbar()
		}
		firstTab := f.tabMgr.NewTab()
		if c.url != "" {
			firstTab.URL = normalizeURL(c.url)
			firstTab.Title = firstTab.URL
		}
		f.syncTabToToolbar()

		f.toolbar.OnNavigate = func(rawURL string) {
			f.navigateTab(rawURL)
		}
		f.toolbar.OnTraverse = func(delta int) {
			f.traverseTab(delta)
		}
		f.toolbar.OnReload = func() {
			f.reloadTab()
		}
		if cb := platform.NewClipboard(); cb != nil {
			f.toolbar.Clipboard = cb
		}
	}
	f.sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: frame.RGB(248, 248, 248)})
	return f, nil
}

// syncTabToToolbar copies the active tab's state into the toolbar for display.
func (f *framePath) syncTabToToolbar() {
	if f.toolbar == nil || f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.toolbar.URL = tab.URL
	f.toolbar.Input = tab.URL
	f.toolbar.Cursor = len([]rune(tab.URL))
	f.toolbar.SelStart = f.toolbar.Cursor
	f.toolbar.SelEnd = f.toolbar.Cursor
	f.toolbar.History = tab.History
	f.toolbar.SetLoading(tab.Nav.Loading)
	f.toolbar.Error = tab.Error
}

// navigateTab loads a URL on the active tab.
func (f *framePath) navigateTab(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	u := normalizeURL(rawURL)

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	tab.Nav.Cancel = cancel
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.Loading = true
	tab.Error = ""
	f.toolbar.SetLoading(true)
	f.toolbar.Error = ""

	go func() {
		layer, _, bgColor, sess, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr))
		f.navResults <- navResult{
			tabID:   tab.ID,
			serial:  serial,
			layer:   layer,
			bgColor: bgColor,
			session: sess,
			err:     err,
			url:     u,
		}
	}()
}

// drainNavResults reads navigation results from the channel and applies them.
func (f *framePath) drainNavResults() {
	for result := range f.navResults {
		f.applyNavResult(result)
	}
}

// applyNavResult applies a completed navigation result to its tab.
func (f *framePath) applyNavResult(result navResult) {
	tab := f.tabMgr.TabByID(result.tabID)
	if tab == nil {
		return
	}

	tab.Nav.Mu.Lock()
	if tab.Nav.Serial != result.serial {
		tab.Nav.Mu.Unlock()
		return
	}
	tab.Nav.Loading = false
	tab.Nav.Mu.Unlock()

	tab.Loading = false
	if result.err != nil {
		tab.Error = result.err.Error()
		if f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
			f.toolbar.Error = result.err.Error()
		}
		fmt.Fprintln(os.Stderr, result.err)
		return
	}

	tab.Layer = result.layer
	tab.Session = result.session
	tab.BGColor = result.bgColor
	tab.URL = result.url
	tab.Title = result.url
	tab.History.Push(result.url)

	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.sched.SetPlan(frame.FramePlan{
			Serial:     result.serial,
			Layers:     []*frame.Layer{result.layer},
			Background: result.bgColor,
		})
	}
}

// traverseTab handles back/forward on the active tab.
func (f *framePath) traverseTab(delta int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	var url string
	var ok bool
	if delta < 0 {
		url, ok = tab.History.Back()
	} else if delta > 0 {
		url, ok = tab.History.Forward()
	}
	if !ok || url == "" {
		return
	}
	f.navigateTabNoHistory(url)
}

// navigateTabNoHistory loads a URL without pushing to history.
func (f *framePath) navigateTabNoHistory(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	u := normalizeURL(rawURL)

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	tab.Nav.Cancel = cancel
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.Loading = true
	tab.Error = ""
	f.toolbar.SetLoading(true)

	go func() {
		layer, _, bgColor, sess, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr))
		f.navResults <- navResult{
			tabID:   tab.ID,
			serial:  serial,
			layer:   layer,
			bgColor: bgColor,
			session: sess,
			err:     err,
			url:     u,
		}
	}()
}

// reloadTab re-fetches the active tab's URL.
func (f *framePath) reloadTab() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL == "" {
		return
	}
	f.navigateTabNoHistory(tab.URL)
}

// handleResize reflows and repaints the active tab after the window is resized.
func (f *framePath) handleResize(contentW, contentH int) {
	f.config.width = contentW
	f.config.height = contentH

	tab := f.tabMgr.Active()
	if tab == nil || tab.Session == nil {
		return
	}

	if err := tab.Session.Reflow(float32(contentW)); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		return
	}

	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	list, err := tab.Session.PaintChecked(effectiveDPR)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: paint after reflow:", err)
		return
	}

	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: budget after reflow:", err)
		return
	}

	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)

	tab.Layer = layer
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor,
	})
}

func (f *framePath) handleZoom(delta float64) {
	if delta == 0 {
		f.zoom = 1.0
	} else {
		f.zoom += delta
		if f.zoom < 0.25 {
			f.zoom = 0.25
		} else if f.zoom > 4.0 {
			f.zoom = 4.0
		}
	}
	f.handleResize(f.config.width, f.config.height)
}

// switchTab saves the current tab's scroll and switches to the given tab.
func (f *framePath) switchTab(id uint64) {
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.ScrollY = vp.Offset.Y
	}
	f.tabMgr.SwitchTo(id)
	newTab := f.tabMgr.Active()
	if newTab == nil {
		return
	}
	f.syncTabToToolbar()
	if newTab.Layer != nil {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     newTab.Nav.Serial,
			Layers:     []*frame.Layer{newTab.Layer},
			Background: newTab.BGColor,
		})
	}
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: newTab.ScrollY},
		Size:   f.config.devSize(),
	})
}

// newTab creates a new tab and switches to it.
func (f *framePath) newTab() {
	if f.tabMgr == nil {
		return
	}
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.ScrollY = vp.Offset.Y
	}
	tab := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(tab.ID)
	f.syncTabToToolbar()
	spec := paint.SceneSpec{DocHeight: f.config.devSize().H}
	_, layer := paint.BuildLayer(spec)
	tab.Layer = layer
	tab.BGColor = frame.RGB(255, 255, 255)
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor,
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: 0},
		Size:   f.config.devSize(),
	})
}

// closeTab closes a tab by ID and switches to the remaining active tab.
func (f *framePath) closeTab(id uint64) {
	if f.tabMgr == nil {
		return
	}
	if tab := f.tabMgr.TabByID(id); tab != nil {
		tab.Nav.Mu.Lock()
		if tab.Nav.Cancel != nil {
			tab.Nav.Cancel()
			tab.Nav.Cancel = nil
		}
		tab.Nav.Mu.Unlock()
	}
	f.tabMgr.CloseTab(id)
	if f.tabMgr.Count() > 0 {
		tab := f.tabMgr.Active()
		f.syncTabToToolbar()
		if tab.Layer != nil {
			f.sched.SetPlan(frame.FramePlan{
				Serial:     tab.Nav.Serial,
				Layers:     []*frame.Layer{tab.Layer},
				Background: tab.BGColor,
			})
		}
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{Y: tab.ScrollY},
			Size:   f.config.devSize(),
		})
	}
}

// loadURLCtx is like loadURL but respects context cancellation.
func loadURLCtx(ctx context.Context, client net.HTTP, fonts *raster.Fonts, rawURL string, viewportW, viewportH int, scale float32) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, error) {
	if err := engine.ValidateViewport(viewportW, viewportH, float64(scale)); err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: viewport: %w", err)
	}
	// Check context before starting the fetch.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, ctx.Err()
	}
	resp, err := client.Get(ctx, rawURL)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: fetch %s: %w", rawURL, err)
	}
	// Linked style sheets are fetched with the same client and the same
	// cancellation: the linker sees absolute http(s) URLs only, because the
	// engine resolves hrefs against the final document URL before calling.
	linker := func(base, href string) (string, error) {
		resp, err := client.Get(ctx, href)
		if err != nil {
			return "", err
		}
		if refused := resp.Headers["Content-Type"]; net.StyleSheetRefused(refused) {
			return "", fmt.Errorf("goosie: %s is served as %q, not as a style sheet", href, refused)
		}
		return resp.Text(), nil
	}
	// Image subresources use the same client and cancellation. The engine hands
	// absolute http(s) URLs only, so the fetcher just reads the response bytes.
	imageFetcher := func(base, url string) ([]byte, error) {
		resp, err := client.Get(ctx, url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// @font-face resources use the same client. The fetcher is the same shape
	// as the image fetcher; the engine resolves src URLs before calling.
	fontFetcher := func(base, url string) ([]byte, error) {
		resp, err := client.Get(ctx, url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// Check context after the fetch in case it was cancelled during the network call.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, ctx.Err()
	}
	sess, err := engine.NewSession(resp.Text(), nil, float32(viewportW),
		engine.WithMetrics(fonts),
		engine.WithViewportH(float32(viewportH)),
		engine.WithLinkedCSS(resp.URL, linker),
		engine.WithImages(resp.URL, imageFetcher),
		engine.WithCustomFontLoading(resp.URL, fontFetcher, fonts))
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: build session: %w", err)
	}
	list, err := sess.PaintChecked(scale)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: paint document: %w", err)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, fmt.Errorf("goosie: size document cache: %w", err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, nil
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
	if f.toolbar != nil && f.tabMgr != nil {
		f.window = newChromeWindow(w, f.toolbar, f.tabMgr,
			func(id uint64) { f.switchTab(id) },
			func() { f.newTab() },
			func(id uint64) { f.closeTab(id) },
			f.handleResize,
			f.handleZoom,
			f.fonts,
		)
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
	// A screenshot run has nobody to watch the background load: the fixed frame
	// count would present synthetic tiles before the document lands, so it loads
	// synchronously and the run captures the page it was asked for.
	if c.url != "" && !c.gate && !c.bench {
		if c.screenshot {
			layer, _, bgColor, _, err := loadURLCtx(context.Background(), f.client, f.fonts, normalizeURL(c.url), c.width, c.height, float32(c.dpr))
			if err != nil {
				return err
			}
			f.sched.SetPlan(frame.FramePlan{
				Serial:     1,
				Layers:     []*frame.Layer{layer},
				Background: bgColor,
			})
		} else {
			f.navigateTab(c.url)
		}
	}

	// A paced run cannot wait for a finger, so the driver stamps the scroll onto the
	// ticks the display already produces. Everything downstream of it - the loop, the
	// scheduler, the composer, the present - is the code an interactive run executes.
	var d *driver
	var finish chan struct{}
	if c.gate || c.bench {
		d = newDriver(f, c.frames)
		f.loop = surface.NewLoop(d, f.sched, f.composer, f.rec)
		finish = d.done
	} else if c.screenshot {
		sd := newSteadyDriver(f, c.frames)
		f.loop = surface.NewLoop(sd, f.sched, f.composer, f.rec)
		sd.watchIdle(func() bool {
			// Idle is not "nothing needed": a frame can want no tiles while the
			// tiles it is waiting on are still on order at a worker. Both halves
			// have to be clear before the pixels in the PNG are all there.
			st := f.loop.Stats()
			return st.Last.Needed == 0 && st.Last.PaintingBytes == 0
		})
		finish = sd.done
		defer func() {
			// After the loop exits, write the PNG from the headless window.
			if hw, ok := f.window.(interface{ WritePNG(string) error }); ok {
				if err := hw.WritePNG(c.out); err != nil {
					fmt.Fprintf(os.Stderr, "goosie: write screenshot: %v\n", err)
				}
			}
		}()
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
	if c.screenshot {
		return nil
	}
	return f.report(d)
}

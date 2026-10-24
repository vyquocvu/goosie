package main

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// TestInputInteractionPipelineWork exercises the browser's actual focus and
// keyboard callbacks while measuring the session pipeline. It is headless and
// asserts work counts, so it catches duplicate layout or paint work without a
// display or machine-dependent timing threshold.
func TestInputInteractionPipelineWork(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><body>
		<input id="q" value="a">
		<p>content below the input</p>
	</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)

	var x, y int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "q" {
			x0, y0, x1, y1 := obj.BorderRect()
			x, y = int32((x0+x1)/2), int32((y0+y1)/2)
			break
		}
	}
	if x == 0 && y == 0 {
		t.Fatal("input was not laid out")
	}

	f.repaintTab(tab, frame.Rect{})
	base := sess.WorkCounts()
	if !f.focusClick(x, y) {
		t.Fatal("focusClick did not focus the input")
	}
	focus := sess.WorkCounts()
	assertWorkDelta(t, "focus", focus, base, engine.WorkCounts{
		DisplayListBuilds: 1,
	})

	if !f.contentKey('b', surface.KeyMod(0)) {
		t.Fatal("contentKey did not handle text input")
	}
	typed := sess.WorkCounts()
	assertWorkDelta(t, "one typed character", typed, focus, engine.WorkCounts{
		DisplayListBuilds: 1,
	})
	if got := sess.FocusedText(); got != "ab" {
		t.Fatalf("input value = %q, want %q", got, "ab")
	}
}

func TestTextareaValueEditStillReflows(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><body><textarea id="memo">a line</textarea><p>following content</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)

	var x, y int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "memo" {
			x0, y0, x1, y1 := obj.BorderRect()
			x, y = int32((x0+x1)/2), int32((y0+y1)/2)
			break
		}
	}
	if x == 0 && y == 0 {
		t.Fatal("textarea was not laid out")
	}
	if !f.focusClick(x, y) {
		t.Fatal("focusClick did not focus the textarea")
	}
	before := sess.WorkCounts()
	if !f.contentKey('b', surface.KeyMod(0)) {
		t.Fatal("contentKey did not handle textarea input")
	}
	assertWorkDelta(t, "textarea edit", sess.WorkCounts(), before, engine.WorkCounts{
		LayoutBuilds:      1,
		DisplayListBuilds: 1,
	})
	if got := sess.FocusedText(); got != "a lineb" {
		t.Fatalf("textarea value = %q, want %q", got, "a lineb")
	}
}

func TestCheckedPseudoClassUpdatesAfterClick(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		input { background-color: blue }
		input:checked { background-color: red }
	</style></head><body><input id="check" type="checkbox"></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	node, x, y := inputNodeAndCenter(t, sess, "check")
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{B: 255, A: 255}) {
		t.Fatalf("unchecked background = %+v, want blue", got)
	}
	before := sess.WorkCounts()
	if !f.focusClick(x, y) {
		t.Fatal("focusClick did not activate checkbox")
	}
	assertWorkDelta(t, "checked state change", sess.WorkCounts(), before, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{R: 255, A: 255}) {
		t.Fatalf("checked background = %+v, want red", got)
	}
}

func TestFocusVisibleRuleTracksKeyboardTab(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		button { background-color: blue; width: 80px; height: 32px; }
		button:focus-visible { background-color: red; }
	</style></head><body>
		<button id="first">First</button>
		<button id="second">Second</button>
	</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	first := sess.Doc.ElementByID("first")
	second := sess.Doc.ElementByID("second")
	if first == nil || second == nil {
		t.Fatal("focus test buttons are missing")
	}
	blue := css.Color{B: 255, A: 255}
	red := css.Color{R: 255, A: 255}
	if got := sess.Styles[first.ID].BackgroundColor; got != blue {
		t.Fatalf("initial first button background = %+v, want blue", got)
	}

	before := sess.WorkCounts()
	if !f.contentKey('\t', surface.KeyMod(0)) {
		t.Fatal("Tab was not handled")
	}
	afterFirstTab := sess.WorkCounts()
	assertWorkDelta(t, "first Tab focus", afterFirstTab, before, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if got := sess.Styles[first.ID].BackgroundColor; got != red {
		t.Fatalf("keyboard-focused first button background = %+v, want red", got)
	}

	if !f.contentKey('\t', surface.KeyMod(0)) {
		t.Fatal("second Tab was not handled")
	}
	if got := sess.Styles[first.ID].BackgroundColor; got != blue {
		t.Fatalf("blurred first button background = %+v, want blue", got)
	}
	if got := sess.Styles[second.ID].BackgroundColor; got != red {
		t.Fatalf("keyboard-focused second button background = %+v, want red", got)
	}
}

func TestActivePseudoClassTracksPointerPressAndRelease(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		#target { display: block; width: 120px; height: 48px; background-color: blue; }
		#target:active { background-color: red; }
	</style></head><body><div id="target">Press</div></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	target := sess.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("active target is missing")
	}
	var x, y int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node == target {
			x0, y0, x1, y1 := obj.BorderRect()
			x, y = int32((x0+x1)/2), int32((y0+y1)/2)
			break
		}
	}
	if x == 0 && y == 0 {
		t.Fatal("active target was not laid out")
	}
	blue := css.Color{B: 255, A: 255}
	red := css.Color{R: 255, A: 255}
	if got := sess.Styles[target.ID].BackgroundColor; got != blue {
		t.Fatalf("initial background = %+v, want blue", got)
	}
	cw := &chromeWindow{
		tabMgr:          f.tabMgr,
		toolbar:         toolbar.NewState(800, nil),
		scale:           1,
		onFocusClick:    f.focusClick,
		onContentActive: f.setContentActive,
		onActiveStyle:   f.refreshActiveContent,
	}
	press := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerPress,
		Button: surface.ButtonLeft,
		Pos:    frame.Point{X: x, Y: y + cw.chromeHeight()},
	}
	beforePress := sess.WorkCounts()
	cw.intercept(press)
	afterPress := sess.WorkCounts()
	assertWorkDelta(t, "active pointer press", afterPress, beforePress, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if got := sess.Styles[target.ID].BackgroundColor; got != red {
		t.Fatalf("pressed background = %+v, want red", got)
	}

	press.Action = surface.PointerRelease
	beforeRelease := sess.WorkCounts()
	cw.intercept(press)
	assertWorkDelta(t, "active pointer release", sess.WorkCounts(), beforeRelease, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if got := sess.Styles[target.ID].BackgroundColor; got != blue {
		t.Fatalf("released background = %+v, want blue", got)
	}
}

func TestKeyboardSpaceActivationUpdatesCheckedStyle(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		input { display: block; width: 120px; height: 48px; background-color: blue; }
		input:checked { background-color: red; }
	</style></head><body><input id="target" type="checkbox"></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	target := sess.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("checkbox target is missing")
	}
	blue := css.Color{B: 255, A: 255}
	red := css.Color{R: 255, A: 255}
	if got := sess.Styles[target.ID].BackgroundColor; got != blue {
		t.Fatalf("initial background = %+v, want blue", got)
	}
	if !f.contentKey('\t', surface.KeyMod(0)) {
		t.Fatal("Tab was not handled")
	}
	before := sess.WorkCounts()
	if !f.contentKey(' ', surface.KeyMod(0)) {
		t.Fatal("Space was not handled")
	}
	assertWorkDelta(t, "keyboard checkbox activation", sess.WorkCounts(), before, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if !engine.IsChecked(target) {
		t.Fatal("Space did not check the focused checkbox")
	}
	if got := sess.Styles[target.ID].BackgroundColor; got != red {
		t.Fatalf("checked background = %+v, want red", got)
	}
}

func TestPlaceholderShownPseudoClassUpdatesAfterEdit(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		input { background-color: blue }
		input:placeholder-shown { background-color: red }
	</style></head><body><input id="query" placeholder="Search"></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	node, x, y := inputNodeAndCenter(t, sess, "query")
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{R: 255, A: 255}) {
		t.Fatalf("empty input background = %+v, want red", got)
	}
	if !f.focusClick(x, y) {
		t.Fatal("focusClick did not focus input")
	}
	before := sess.WorkCounts()
	if !f.contentKey('x', surface.KeyMod(0)) {
		t.Fatal("contentKey did not edit input")
	}
	assertWorkDelta(t, "placeholder state change", sess.WorkCounts(), before, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{B: 255, A: 255}) {
		t.Fatalf("non-empty input background = %+v, want blue", got)
	}
}

func TestHoverPseudoClassUpdatesAfterPointerMove(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		#target { background-color: blue }
		#target:hover { background-color: red }
	</style></head><body><div id="target">hover me</div></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	node, x, y := inputNodeAndCenter(t, sess, "target")
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{B: 255, A: 255}) {
		t.Fatalf("initial background = %+v, want blue", got)
	}
	before := sess.WorkCounts()
	f.hoverContent(x, y)
	afterHover := sess.WorkCounts()
	assertWorkDelta(t, "hover target change", afterHover, before, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if visits := afterHover.StyleNodeVisits - before.StyleNodeVisits; visits == 0 || visits >= uint64(len(sess.Styles))/2 {
		t.Fatalf("pointer hover visited %d style nodes out of %d; want the target subtree only", visits, len(sess.Styles))
	}
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{R: 255, A: 255}) {
		t.Fatalf("hover background = %+v, want red", got)
	}
	before = sess.WorkCounts()
	f.hoverContent(-1, -1)
	afterClear := sess.WorkCounts()
	assertWorkDelta(t, "hover target cleared", afterClear, before, engine.WorkCounts{
		StyleResolves:     1,
		DisplayListBuilds: 1,
	})
	if visits := afterClear.StyleNodeVisits - before.StyleNodeVisits; visits == 0 || visits >= uint64(len(sess.Styles))/2 {
		t.Fatalf("pointer hover clear visited %d style nodes out of %d; want the target subtree only", visits, len(sess.Styles))
	}
	if got := sess.Styles[node.ID].BackgroundColor; got != (css.Color{B: 255, A: 255}) {
		t.Fatalf("cleared hover background = %+v, want blue", got)
	}
}

func inputNodeAndCenter(t *testing.T, sess *engine.Session, id string) (*dom.Node, int32, int32) {
	t.Helper()
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return obj.Node, int32((x0 + x1) / 2), int32((y0 + y1) / 2)
		}
	}
	t.Fatalf("input %q was not laid out", id)
	return nil, 0, 0
}

func assertWorkDelta(t *testing.T, label string, got, before, want engine.WorkCounts) {
	t.Helper()
	delta := engine.WorkCounts{
		StyleResolves:     got.StyleResolves - before.StyleResolves,
		LayoutBuilds:      got.LayoutBuilds - before.LayoutBuilds,
		DisplayListBuilds: got.DisplayListBuilds - before.DisplayListBuilds,
	}
	if delta != want {
		t.Errorf("%s pipeline work = %+v, want %+v", label, delta, want)
	}
}

func TestInputInteractionTileWork(t *testing.T) {
	for _, dpr := range []int{1, 2} {
		t.Run(fmt.Sprintf("dpr%d", dpr), func(t *testing.T) {
			h := newInputTileHarness(t, dpr)
			layer := h.f.tabMgr.Active().Layer()
			coords := layer.VisibleCoords(h.f.sched.Viewport(), nil)
			if len(coords) < 9 {
				t.Fatalf("only %d warm tiles; fixture must span multiple rows and columns", len(coords))
			}
			for _, action := range []string{"focus", "typing"} {
				t.Run(action, func(t *testing.T) {
					pipelineBefore := h.f.tabMgr.Active().Session().WorkCounts()
					before := h.f.sched.Stats().Rasterized
					workerBefore := h.f.pool.Stats().Rasterized
					pixels := make(map[frame.TileCoord]*frame.Bitmap)
					for _, coord := range coords {
						if layer.Needs(coord) {
							t.Fatalf("tile %v was not warmed", coord)
						}
						pixels[coord] = layer.Grid.Pixels(coord)
					}
					prior := append([]byte(nil), h.w.Presented().RGBA...)
					if action == "focus" {
						h.focus(t)
					} else {
						h.input(t, surface.Event{Kind: surface.EvKey, Key: 'b'})
						if got := h.f.tabMgr.Active().Session().FocusedText(); got != "ab" {
							t.Fatalf("typed value = %q, want ab", got)
						}
					}
					assertWorkDelta(t, action+" with JS runtime", h.f.tabMgr.Active().Session().WorkCounts(), pipelineBefore, engine.WorkCounts{
						DisplayListBuilds: 1,
					})
					h.f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
					if h.f.tabMgr.Active().Layer() != layer {
						t.Fatalf("local interaction discarded the warm layer: bounds %v -> %v, budget %d -> %d", layer.Bounds, h.f.tabMgr.Active().Layer().Bounds, layer.Grid.Stats().Budget, h.f.tabMgr.Active().Layer().Grid.Stats().Budget)
					}
					damage := frame.Rect4(16, 16, 200, 100)
					damage = frame.Rect4(damage.X0*int32(dpr), damage.Y0*int32(dpr), damage.X1*int32(dpr), damage.Y1*int32(dpr))
					stale := 0
					for _, coord := range coords {
						if layer.Needs(coord) {
							stale++
							if !coord.Rect(frame.TileSize).Intersects(damage) {
								t.Errorf("%s invalidated tile %v outside the control damage", action, coord)
							}
						}
					}
					budget := int64(dpr)
					if stale == 0 || int64(stale) > budget {
						t.Errorf("%s stale tiles = %d, want 1..%d", action, stale, budget)
					}
					h.settle(t)
					if got := h.f.sched.Stats().Rasterized - before; got == 0 || got > budget {
						t.Errorf("%s rasterized %d tiles, budget 1..%d", action, got, budget)
					} else {
						t.Logf("%s rasterized %d/%d tiles (budget %d)", action, got, len(coords), budget)
					}
					if got := h.f.pool.Stats().Rasterized - workerBefore; got == 0 || got > budget {
						t.Errorf("%s worker rasters = %d, budget 1..%d", action, got, budget)
					}
					for _, coord := range coords {
						if !coord.Rect(frame.TileSize).Intersects(damage) && layer.Grid.Pixels(coord) != pixels[coord] {
							t.Errorf("%s replaced cached pixels outside damage at %v", action, coord)
						}
					}
					if bytes.Equal(prior, h.w.Presented().RGBA) {
						t.Errorf("%s did not change any presented pixels", action)
					}
					h.assertColdPixels(t)
				})
			}
		})
	}
}

func TestInputInteractionTileReflow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		height    int
		newExtent bool
	}{
		{"same_extent", 300, false},
		{"larger_extent", 1000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newInputTileHarness(t, 1)
			h.focus(t)
			h.settle(t)
			tab := h.f.tabMgr.Active()
			layer := tab.Layer()
			oldMarker := h.bounds(t, "marker")
			if err := h.rt.Run(fmt.Sprintf(`document.getElementById("q").addEventListener("input", function () {
				document.getElementById("flow").setAttribute("style", "height: %dpx");
			});`, tc.height), ""); err != nil {
				t.Fatal(err)
			}
			beforeEdit := tab.Session().WorkCounts()
			h.input(t, surface.Event{Kind: surface.EvKey, Key: 'b'})
			afterEdit := tab.Session().WorkCounts()
			assertWorkDelta(t, "input handler mutation before pump", afterEdit, beforeEdit, engine.WorkCounts{
				DisplayListBuilds: 1,
			})
			h.f.pumpJS()
			afterPump := tab.Session().WorkCounts()
			assertWorkDelta(t, "input handler mutation pump", afterPump, afterEdit, engine.WorkCounts{
				StyleResolves:     1,
				LayoutBuilds:      1,
				DisplayListBuilds: 1,
			})
			if got, err := h.rt.Evaluate(`document.getElementById("flow").getAttribute("style")`); err != nil || got != fmt.Sprintf("height: %dpx", tc.height) {
				t.Fatalf("input listener height = %q, err %v", got, err)
			}
			newMarker := h.bounds(t, "marker")
			if newMarker.Y0 <= oldMarker.Y1 {
				t.Fatalf("fixture did not move the following box: %v -> %v", oldMarker, newMarker)
			}
			if got := tab.Layer() != layer; got != tc.newExtent {
				t.Fatalf("layer replaced = %v, want %v (bounds %v -> %v)", got, tc.newExtent, layer.Bounds, tab.Layer().Bounds)
			}
			h.settle(t)
			h.assertColdPixels(t)
		})
	}
}

func TestInputInteractionCoalescedTileUpdates(t *testing.T) {
	h := newInputTileHarness(t, 1)
	h.focus(t)
	h.input(t, surface.Event{Kind: surface.EvKey, Key: 'b'})
	h.input(t, surface.Event{Kind: surface.EvKey, Key: 'c'})
	h.settle(t)
	if got := h.f.tabMgr.Active().Session().FocusedText(); got != "abc" {
		t.Fatalf("coalesced value = %q, want abc", got)
	}
	h.assertColdPixels(t)
}

type inputTileHarness struct {
	f        *framePath
	w        *headless.Window
	cw       *chromeWindow
	composer *surface.Composer
	rt       *js.Runtime
}

func newInputTileHarness(t *testing.T, dpr int) *inputTileHarness {
	t.Helper()
	fonts, err := raster.NewEmbeddedFonts()
	if err != nil {
		t.Fatal(err)
	}
	pool := raster.New(2, 64, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool.Start(ctx)
	t.Cleanup(func() { cancel(); _ = pool.Close() })
	size := frame.Size{W: 768 * int32(dpr), H: 768 * int32(dpr)}
	w := headless.New(headless.Config{Size: size, Scale: float32(dpr), Clock: headless.NewManualClock(time.Time{})})
	t.Cleanup(func() { _ = w.Close() })
	f := &framePath{
		config: config{width: 768, height: 768, dpr: float64(dpr)}, zoom: 1,
		pool: pool, window: w, tabMgr: tabs.NewManager(nil),
		sched: raster.NewScheduler(nil, pool, frame.Viewport{Size: size}, float32(dpr), raster.Pref{PrefetchRows: raster.PrefetchOff}),
	}
	f.tabMgr.NewTab()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	sess, err := engine.NewSession(`<html><head><style>
		body { margin: 0; padding: 32px; width: 704px; height: 704px; background: white; }
		input { display: block; margin: 0; width: 120px; height: 32px; }
		#flow { width: 180px; }
		#marker { width: 160px; height: 40px; background: #208040; }
		#far { position: absolute; left: 520px; top: 600px; width: 180px; height: 48px; background: #e0c080; }
	</style></head><body><input id="q" value="a"><div id="flow">short</div><div id="marker"></div><div id="far">unchanged content</div></body></html>`, nil, 768,
		engine.WithMetrics(fonts), engine.WithViewportH(768), engine.WithJS(rt), engine.WithInvalidation(&engine.Invalidation{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	f.tabMgr.Active().SetSession(sess)
	f.repaintTab(f.tabMgr.Active(), frame.Rect{})
	h := &inputTileHarness{f: f, w: w, rt: rt, composer: surface.NewComposer(size, frame.NewBitmapPool(size, 1))}
	h.cw = &chromeWindow{Window: w, toolbar: toolbar.NewState(size.W, nil), scale: int32(dpr), onFocusClick: f.focusClick, onContentKey: f.contentKey}
	h.settle(t)
	return h
}

func (h *inputTileHarness) bounds(t *testing.T, id string) frame.Rect {
	t.Helper()
	for _, obj := range h.f.tabMgr.Active().Session().Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return frame.RectF4(x0, y0, x1, y1).ToDevice(float32(h.f.config.dpr))
		}
	}
	t.Fatalf("missing box %q", id)
	return frame.Rect{}
}

func (h *inputTileHarness) input(t *testing.T, ev surface.Event) {
	t.Helper()
	if err := h.w.Inject(ev); err != nil {
		t.Fatal(err)
	}
	if !h.cw.intercept(<-h.w.Events()) {
		t.Fatalf("chrome did not consume input %+v", ev)
	}
}

func (h *inputTileHarness) focus(t *testing.T) {
	t.Helper()
	r := h.bounds(t, "q")
	h.input(t, surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft,
		Pos: frame.Point{X: (r.X0 + r.X1) / 2, Y: (r.Y0+r.Y1)/2 + h.cw.chromeHeight()}})
	if !h.w.IMEEnabled() {
		t.Fatal("focus did not enable window IME")
	}
}

func settleInputTiles(t *testing.T, sched *raster.Scheduler, layer *frame.Layer, composer *surface.Composer, window *headless.Window) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	quiet := false
	for time.Now().Before(deadline) {
		plan, needed := sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
		if plan == nil {
			t.Fatal("no frame plan")
		}
		work := sched.Submit(needed)
		damage := sched.ComposeInto(composer.Backing, composer, plan, needed)
		if err := window.Present(composer.Backing, damage); err != nil {
			t.Fatal(err)
		}
		if sched.Stats().Failed != 0 {
			t.Fatalf("raster failures: %+v", sched.Stats())
		}
		idle := work.Needed == 0 && layer.Grid.Stats().PaintingBytes == 0
		// A deferred swap may need the frame after the last worker completes.
		if idle && quiet {
			return
		}
		quiet = idle
		runtime.Gosched()
	}
	t.Fatalf("tiles did not settle: %+v", sched.Stats())
}

func (h *inputTileHarness) settle(t *testing.T) {
	t.Helper()
	settleInputTiles(t, h.f.sched, h.f.tabMgr.Active().Layer(), h.composer, h.w)
}

func (h *inputTileHarness) assertColdPixels(t *testing.T) {
	t.Helper()
	list, err := h.f.tabMgr.Active().Session().PaintChecked(float32(h.f.config.dpr))
	if err != nil {
		t.Fatal(err)
	}
	content := list.Build(1).Publish()
	count, budget, err := engine.TileCacheBudget(content.Extent())
	if err != nil {
		t.Fatal(err)
	}
	cold := frame.NewLayer(1, content.Extent(), budget, frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, count))
	cold.SetContent(content)
	vp := h.f.sched.Viewport()
	sched := raster.NewScheduler(cold, h.f.pool, vp, float32(h.f.config.dpr), raster.Pref{PrefetchRows: raster.PrefetchOff})
	sched.SetPlan(frame.FramePlan{Layers: []*frame.Layer{cold}, Background: h.f.tabMgr.Active().BGColor()})
	window := headless.New(headless.Config{Size: vp.Size, Clock: headless.NewManualClock(time.Time{})})
	defer window.Close()
	settleInputTiles(t, sched, cold, surface.NewComposer(vp.Size, frame.NewBitmapPool(vp.Size, 1)), window)
	if !bytes.Equal(h.w.Presented().RGBA, window.Presented().RGBA) {
		t.Fatal("incremental presented pixels differ from a fresh full render")
	}
}

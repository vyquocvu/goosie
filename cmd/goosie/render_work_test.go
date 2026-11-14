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

func TestRangePointerDragRoutesThroughBrowserInteraction(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>input { display:inline-block; width:280px; height:20px; }</style></head><body><input id="range" type="range" min="0" max="100" step="10" value="20"></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	var left, right, y int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "range" {
			x0, y0, x1, y1 := obj.BorderRect()
			left, right, y = int32(x0+12), int32(x1-12), int32((y0+y1)/2)
			break
		}
	}
	if left == 0 || right <= left {
		t.Fatal("range input was not laid out")
	}
	if !f.focusClick(left, y) {
		t.Fatal("range pointer press was not handled")
	}
	if got := sess.Focused().InputControlValue(); got != "0" {
		t.Fatalf("range value after press = %q, want 0", got)
	}
	if !f.dragSelect(surface.PointerMotion, right, y) {
		t.Fatal("range pointer movement was not handled")
	}
	if got := sess.Focused().InputControlValue(); got != "100" {
		t.Fatalf("range value during drag = %q, want 100", got)
	}
	if !f.dragSelect(surface.PointerRelease, right, y) {
		t.Fatal("range pointer release was not handled")
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
	if !f.releaseContentKey(' ', surface.KeyMod(0)) {
		t.Fatal("Space release was not handled")
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

func TestHoverWithoutPseudoClassDoesNotRepaint(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(`<html><body style="margin:0">
		<p id="a">This domain is for use in documentation examples without needing permission.</p>
		<p id="b">This is not a service; avoid relying on it for testing and monitoring purposes.</p>
		<script>
			window.enters = 0;
			document.querySelectorAll('p').forEach(function (p) {
				p.addEventListener('mouseenter', function () {
					window.enters++;
					document.body.setAttribute('data-enters', String(window.enters));
				});
			});
		</script>
	</body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	f.tabMgr.Active().SetSession(sess)
	_, x1, y1 := inputNodeAndCenter(t, sess, "a")
	_, x2, y2 := inputNodeAndCenter(t, sess, "b")
	before := sess.WorkCounts()
	f.hoverContent(x1, y1)
	f.hoverContent(x2, y2)
	f.hoverContent(x1, y1)
	assertWorkDelta(t, "hover across text without :hover", sess.WorkCounts(), before, engine.WorkCounts{})
	got, err := sess.EvaluateJS(`document.body.getAttribute('data-enters')`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "3" {
		t.Fatalf("mouseenter count = %q, want 3 (hover state never changed?)", got)
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

func TestTextareaPlaceholderShownTracksLiveValueOnEdit(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><head><style>
		textarea { background-color: blue }
		textarea:placeholder-shown { background-color: red }
	</style></head><body><textarea id="memo" placeholder="Notes">saved</textarea></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	node, x, y := inputNodeAndCenter(t, sess, "memo")
	blue := css.Color{B: 255, A: 255}
	red := css.Color{R: 255, A: 255}
	if got := sess.Styles[node.ID].BackgroundColor; got != blue {
		t.Fatalf("nonempty textarea background = %+v, want blue", got)
	}
	if !f.focusClick(x, y) {
		t.Fatal("focusClick did not focus the textarea")
	}
	for range 5 {
		if !f.contentKey('\b', surface.KeyMod(0)) {
			t.Fatal("Backspace did not edit the textarea")
		}
	}
	if got := node.InputControlValue(); got != "" {
		t.Fatalf("textarea live value = %q, want empty", got)
	}
	if got := sess.Styles[node.ID].BackgroundColor; got != red {
		t.Fatalf("empty live textarea background = %+v, want red", got)
	}
	if got := node.TextContent(); got != "saved" {
		t.Fatalf("textarea default text = %q, want saved", got)
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

const inputTileMarkup = `<html><head><style>
	body { margin: 0; padding: 32px; width: 704px; height: 704px; background: white; }
	input { display: block; margin: 0; width: 120px; height: 32px; }
	#flow { width: 180px; }
	#marker { width: 160px; height: 40px; background: #208040; }
	#far { position: absolute; left: 520px; top: 600px; width: 180px; height: 48px; background: #e0c080; }
</style></head><body><input id="q" value="a"><div id="flow">short</div><div id="marker"></div><div id="far">unchanged content</div></body></html>`

func newInputTileHarness(t *testing.T, dpr int) *inputTileHarness {
	t.Helper()
	return newTileHarness(t, dpr, 768, 768, inputTileMarkup)
}

func newTileHarness(t *testing.T, dpr, contentW, contentH int, markup string) *inputTileHarness {
	t.Helper()
	fonts, err := raster.NewEmbeddedFonts()
	if err != nil {
		t.Fatal(err)
	}
	pool := raster.New(2, 64, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool.Start(ctx)
	t.Cleanup(func() { cancel(); _ = pool.Close() })
	size := frame.Size{W: int32(contentW * dpr), H: int32(contentH * dpr)}
	w := headless.New(headless.Config{Size: size, Scale: float32(dpr), Clock: headless.NewManualClock(time.Time{})})
	t.Cleanup(func() { _ = w.Close() })
	f := &framePath{
		config: config{width: contentW, height: contentH, dpr: float64(dpr)}, zoom: 1,
		pool: pool, window: w, tabMgr: tabs.NewManager(nil),
		sched: raster.NewScheduler(nil, pool, frame.Viewport{Size: size}, float32(dpr), raster.Pref{PrefetchRows: raster.PrefetchOff}),
	}
	f.tabMgr.NewTab()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	sess, err := engine.NewSession(markup, nil, float32(contentW),
		engine.WithMetrics(fonts), engine.WithViewportH(float32(contentH)), engine.WithJS(rt), engine.WithInvalidation(&engine.Invalidation{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	f.tabMgr.Active().SetSession(sess)
	f.repaintTab(f.tabMgr.Active(), frame.Rect{})
	h := &inputTileHarness{f: f, w: w, rt: rt, composer: surface.NewComposer(size, frame.NewBitmapPool(size, 1))}
	h.cw = &chromeWindow{Window: w, toolbar: toolbar.NewState(size.W, nil), scale: int32(dpr), onFocusClick: f.focusClick, onContentKey: f.contentKeyAction}
	h.settle(t)
	return h
}

func (h *inputTileHarness) bounds(t *testing.T, id string) frame.Rect {
	t.Helper()
	return h.boundsAtScale(t, id, float32(h.f.config.dpr))
}

func (h *inputTileHarness) boundsAtScale(t *testing.T, id string, scale float32) frame.Rect {
	t.Helper()
	for _, obj := range h.f.tabMgr.Active().Session().Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return frame.RectF4(x0, y0, x1, y1).ToDevice(scale)
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

// qc:BR-173 BR-081
func TestStaticPageIdleFramesDoNoPipelineOrTileWork(t *testing.T) {
	h := newInputTileHarness(t, 1)
	tab := h.f.tabMgr.Active()
	sess := tab.Session()
	layer := tab.Layer()
	if sess == nil || layer == nil {
		t.Fatal("fixture has no settled document to be idle on")
	}

	baseWork := sess.WorkCounts()
	base := h.f.sched.Stats()
	baseVersion := layer.ContentVersion

	const idleFrames = 30
	for i := 0; i < idleFrames; i++ {
		if h.f.pumpJS() {
			t.Fatalf("idle frame %d published a repaint for an unchanged document", i)
		}
		plan, needed := h.f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
		if plan == nil {
			t.Fatalf("idle frame %d produced no plan", i)
		}
		if len(needed) != 0 {
			t.Fatalf("idle frame %d asked for %d tiles, want 0", i, len(needed))
		}
		work := h.f.sched.Submit(needed)
		if work.Submitted != 0 || work.Needed != 0 {
			t.Fatalf("idle frame %d submitted %d jobs for %d needed tiles", i, work.Submitted, work.Needed)
		}
		damage := h.f.sched.ComposeInto(h.composer.Backing, h.composer, plan, needed)
		if err := h.w.Present(h.composer.Backing, damage); err != nil {
			t.Fatalf("idle frame %d present: %v", i, err)
		}
	}

	if got := sess.WorkCounts(); got != baseWork {
		t.Fatalf("idle frames changed pipeline work from %+v to %+v", baseWork, got)
	}
	after := h.f.sched.Stats()
	for _, counter := range []struct {
		name string
		got  int64
		was  int64
	}{
		{"Rasterized", after.Rasterized, base.Rasterized},
		{"Submitted", after.Submitted, base.Submitted},
		{"Refused", after.Refused, base.Refused},
		{"Deferred", after.Deferred, base.Deferred},
		{"Prefetched", after.Prefetched, base.Prefetched},
		{"Failed", after.Failed, base.Failed},
	} {
		if counter.got != counter.was {
			t.Errorf("%s grew from %d to %d across %d idle frames", counter.name, counter.was, counter.got, idleFrames)
		}
	}
	if got := after.Frames - base.Frames; got != idleFrames {
		t.Errorf("scheduler drew %d idle frames, want %d", got, idleFrames)
	}
	if got := layer.ContentVersion; got != baseVersion {
		t.Errorf("idle frames bumped the content version from %d to %d", baseVersion, got)
	}
	if tab.Layer() != layer {
		t.Error("idle frames replaced the settled layer")
	}
}

// qc:BR-082
func TestStateOnlyRAFAndTimerLoopDoesNotAmplifyWork(t *testing.T) {
	h := newTileHarness(t, 1, 512, 512, `<html><head><style>
		html, body { margin: 0; padding: 0; }
		#static { width: 120px; height: 80px; background: #3060a0; }
	</style></head><body><div id="static"></div><script>
		var ticks = 0;
		function frame() { ticks++; requestAnimationFrame(frame); }
		requestAnimationFrame(frame);
		setInterval(function () { ticks++; }, 0);
	</script></body></html>`)
	tab := h.f.tabMgr.Active()
	sess := tab.Session()
	layer := tab.Layer()
	baseWork := sess.WorkCounts()
	base := h.f.sched.Stats()

	const frames = 30
	for i := 0; i < frames; i++ {
		if h.f.pumpJS() {
			t.Fatalf("frame %d repainted for callbacks that only read state", i)
		}
		plan, needed := h.f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
		if plan == nil {
			t.Fatalf("frame %d produced no plan", i)
		}
		if len(needed) != 0 {
			t.Fatalf("frame %d asked for %d tiles", i, len(needed))
		}
		work := h.f.sched.Submit(needed)
		if work.Submitted != 0 {
			t.Fatalf("frame %d submitted %d jobs", i, work.Submitted)
		}
		damage := h.f.sched.ComposeInto(h.composer.Backing, h.composer, plan, needed)
		if len(damage) != 0 {
			t.Fatalf("frame %d composed %d damaged bands", i, len(damage))
		}
	}
	if got, err := h.rt.Evaluate("ticks"); err != nil {
		t.Fatal(err)
	} else if got == "0" {
		t.Fatal("neither the animation frame nor the timer callback ran")
	}
	if got := sess.WorkCounts(); got != baseWork {
		t.Fatalf("state-only callbacks changed pipeline work from %+v to %+v", baseWork, got)
	}
	after := h.f.sched.Stats()
	for _, counter := range []struct {
		name string
		got  int64
		was  int64
	}{
		{"Rasterized", after.Rasterized, base.Rasterized},
		{"Submitted", after.Submitted, base.Submitted},
		{"Deferred", after.Deferred, base.Deferred},
		{"Failed", after.Failed, base.Failed},
	} {
		if counter.got != counter.was {
			t.Errorf("%s grew from %d to %d across %d script frames", counter.name, counter.was, counter.got, frames)
		}
	}
	if got := after.Frames - base.Frames; got != frames {
		t.Errorf("scheduler drew %d frames, want %d", got, frames)
	}
	if tab.Layer() != layer {
		t.Error("state-only callbacks replaced the layer")
	}
}

// qc:BR-080
func TestSmallDOMChangeDamagesOnlyItsOwnTiles(t *testing.T) {
	h := newInputTileHarness(t, 1)
	tab := h.f.tabMgr.Active()
	layer := tab.Layer()
	marker := h.bounds(t, "marker")
	coords := layer.VisibleCoords(h.f.sched.Viewport(), nil)
	pixels := make(map[frame.TileCoord]*frame.Bitmap, len(coords))
	for _, coord := range coords {
		pixels[coord] = layer.Grid.Pixels(coord)
	}
	prior := append([]byte(nil), h.w.Presented().RGBA...)
	beforeRasterized := h.f.sched.Stats().Rasterized

	if err := h.rt.Run(`document.getElementById("marker").style.backgroundColor = "#c02020";`, ""); err != nil {
		t.Fatal(err)
	}
	if !h.f.pumpJS() {
		t.Fatal("a background mutation did not request a repaint")
	}
	affected := map[frame.TileCoord]bool{}
	for _, coord := range coords {
		if coord.Rect(frame.TileSize).Intersects(marker) {
			affected[coord] = true
		}
	}
	if len(affected) == 0 {
		t.Fatal("fixture marker covers no visible tile")
	}

	plan, needed := h.f.sched.BeginFrame(surface.Event{Kind: surface.EvVsync})
	if plan == nil {
		t.Fatal("no plan after the mutation")
	}
	if len(needed) == 0 {
		t.Fatal("the mutated tile was not rasterized")
	}
	for _, coord := range needed {
		if !affected[coord] {
			t.Fatalf("tile %v outside the changed box was invalidated", coord)
		}
	}
	if work := h.f.sched.Submit(needed); work.Submitted == 0 {
		t.Fatal("the damaged tiles were never submitted")
	}
	damage := h.f.sched.ComposeInto(h.composer.Backing, h.composer, plan, needed)
	if err := h.w.Present(h.composer.Backing, damage); err != nil {
		t.Fatal(err)
	}
	h.settle(t)

	if got := h.f.sched.Stats().Rasterized - beforeRasterized; got == 0 || got > int64(len(affected)) {
		t.Fatalf("rasterized %d tiles, want 1..%d covering the changed box", got, len(affected))
	}
	for _, coord := range coords {
		if !affected[coord] && layer.Grid.Pixels(coord) != pixels[coord] {
			t.Errorf("tile %v outside the damage was re-rasterized", coord)
		}
	}
	after := h.w.Presented().RGBA
	w := int(h.composer.Backing.W)
	for y := 0; y < int(h.composer.Backing.H); y++ {
		for x := 0; x < w; x++ {
			inside := int32(x) >= marker.X0 && int32(x) < marker.X1 && int32(y) >= marker.Y0 && int32(y) < marker.Y1
			i := 4 * (y*w + x)
			if !inside && (prior[i] != after[i] || prior[i+1] != after[i+1] || prior[i+2] != after[i+2] || prior[i+3] != after[i+3]) {
				t.Fatalf("pixel (%d,%d) outside the changed box changed after the repaint", x, y)
			}
		}
	}
	changed := false
	for i := 0; i < len(prior); i += 4 {
		if prior[i] != after[i] || prior[i+1] != after[i+1] || prior[i+2] != after[i+2] {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("the repaint never painted the new background")
	}
}

// qc:BR-078
func TestZoomRoundTripScalesPaintKeepsClicksAndClearsOldImage(t *testing.T) {
	h := newInputTileHarness(t, 1)
	tab := h.f.tabMgr.Active()
	sess := tab.Session()
	click := func(r frame.Rect) {
		t.Helper()
		h.input(t, surface.Event{Kind: surface.EvPointer, Action: surface.PointerPress, Button: surface.ButtonLeft,
			Pos: frame.Point{X: (r.X0 + r.X1) / 2, Y: (r.Y0+r.Y1)/2 + h.cw.chromeHeight()}})
	}

	baseLayer := tab.Layer()
	baseBounds := baseLayer.Bounds
	input100 := h.bounds(t, "q")
	marker100 := h.bounds(t, "marker")
	staleX, staleY := int32(220), int32(140)
	if marker100.X0 <= staleX && staleX < marker100.X1 && marker100.Y0 <= staleY && staleY < marker100.Y1 {
		t.Fatalf("fixture point (%d,%d) is already inside the 100%% marker %v", staleX, staleY, marker100)
	}
	baseline := append([]byte(nil), h.w.Presented().RGBA...)

	h.f.handleZoom(0.25)
	h.settle(t)
	if got := h.f.zoom; got < 1.249 || got > 1.251 {
		t.Fatalf("zoom = %v, want 1.25", got)
	}
	zoomedLayer := tab.Layer()
	if zoomedLayer == baseLayer {
		t.Fatal("125% kept the 100% layer; the old image was not cleared")
	}
	if zoomedLayer.Bounds.Y1 <= baseBounds.Y1 {
		t.Fatalf("zoomed extent height = %d, want more than the %d at 100%%", zoomedLayer.Bounds.Y1, baseBounds.Y1)
	}
	marker125 := h.boundsAtScale(t, "marker", 1.25)
	w100, h100 := marker100.X1-marker100.X0, marker100.Y1-marker100.Y0
	if w125, h125 := marker125.X1-marker125.X0, marker125.Y1-marker125.Y0; w125 < w100*5/4-1 || w125 > w100*5/4+1 || h125 < h100*5/4-1 || h125 > h100*5/4+1 {
		t.Fatalf("125%% marker = %v, want (%d, %d) at 5/4 scale", marker125, w100*5/4, h100*5/4)
	}
	if got := h.w.Presented().RGBA[4*(int(staleY)*int(h.composer.Backing.W)+int(staleX))]; got == baseline[4*(int(staleY)*int(h.composer.Backing.W)+int(staleX))] {
		t.Fatalf("pixel (%d,%d) did not scale with the page", staleX, staleY)
	}

	input125 := h.boundsAtScale(t, "q", 1.25)
	click(input125)
	if !sess.FocusedEditable() {
		t.Fatal("click at the 125% input center did not reach the control")
	}
	if err := h.rt.Run(`document.getElementById("q").blur();`, ""); err != nil {
		t.Fatal(err)
	}
	if sess.FocusedEditable() {
		t.Fatal("blur did not clear the control focus before the zoom reset")
	}

	h.f.handleZoom(0)
	h.settle(t)
	if got := h.f.zoom; got != 1 {
		t.Fatalf("reset zoom = %v, want 1", got)
	}
	if tab.Layer() == zoomedLayer || tab.Layer().Bounds != baseBounds {
		t.Fatalf("100%% return layer bounds = %v, want the %v the page started at", tab.Layer().Bounds, baseBounds)
	}
	i := 4 * (int(staleY)*int(h.composer.Backing.W) + int(staleX))
	if got, want := h.w.Presented().RGBA[i], baseline[i]; got != want {
		t.Fatalf("pixel (%d,%d) = %v after returning to 100%%, want the original %v: the 125%% image survived", staleX, staleY, got, want)
	}
	if marker100 != h.bounds(t, "marker") {
		t.Fatalf("marker = %v after reset, want %v", h.bounds(t, "marker"), marker100)
	}
	click(input100)
	if !sess.FocusedEditable() {
		t.Fatal("click at the 100% input center stopped reaching the control after the zoom round trip")
	}
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

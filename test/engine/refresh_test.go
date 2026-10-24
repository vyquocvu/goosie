package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestRefreshScrollOnlyIsNoop(t *testing.T) {
	sess, err := engine.NewSession("<html><body><div>test</div></body></html>", nil, 512)
	if err != nil {
		t.Fatal(err)
	}

	// Scroll-only plan should be a no-op.
	plan := engine.Plan{
		Viewport: frame.Viewport{Offset: frame.Point{Y: 100}, Size: frame.Size{W: 512, H: 512}},
		Reasons:  engine.Scroll,
	}

	// Capture the original arena pointer to verify it's unchanged.
	origArena := sess.Arena

	done := sess.Refresh(plan, nil, 512)
	if done {
		t.Fatal("scroll-only plan should not do work")
	}
	if sess.Arena != origArena {
		t.Fatal("scroll-only plan should not rebuild arena")
	}
}

func TestRestyleAndReflowVisitsOnlyLocalHoverSubtree(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)
	markup := `<html><head><style>#target { background-color: black } #target:hover { background-color: red }</style></head><body><div id="target"></div>` + strings.Repeat(`<div></div>`, 40) + `</body></html>`
	sess, err := engine.NewSession(markup, nil, 512)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	target := sess.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("target is missing")
	}
	originalArena := sess.Arena
	before := sess.WorkCounts()
	for current := target; current != nil && current.Element(); current = current.Parent {
		css.SetHover(current, true)
	}
	if err := sess.RestyleAndReflow(512); err != nil {
		t.Fatal(err)
	}
	after := sess.WorkCounts()
	if after.StyleResolves != before.StyleResolves+1 {
		t.Fatalf("style resolves = %d, want %d", after.StyleResolves, before.StyleResolves+1)
	}
	visited := after.StyleNodeVisits - before.StyleNodeVisits
	if visited == 0 || visited >= uint64(len(sess.Styles))/2 {
		t.Fatalf("style node visits = %d of %d; expected only the hovered subtree", visited, len(sess.Styles))
	}
	if after.LayoutBuilds != before.LayoutBuilds || sess.Arena != originalArena {
		t.Fatalf("paint-only hover rebuilt layout: layout builds %d -> %d", before.LayoutBuilds, after.LayoutBuilds)
	}
	if got := sess.Styles[target.ID].BackgroundColor; got.R != 255 || got.G != 0 || got.B != 0 {
		t.Fatalf("hover background color = %+v, want red", got)
	}
	if visited > 5 {
		t.Fatalf("local hover visited %d nodes, want target subtree only", visited)
	}
}

func TestRestyleAndReflowRebuildsPseudoBoxAfterLocalHoverStyleChange(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)
	sess, err := engine.NewSession(`<html><head><style>
		#target::before { content: "marker"; color: black }
		#target:hover::before { color: red }
	</style></head><body><button id="target"></button></body></html>`, nil, 512)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	target := sess.Doc.ElementByID("target")
	key := style.PseudoKey{NodeID: target.ID, Pseudo: "before"}
	if sess.PseudoStyles[key] == nil {
		t.Fatal("before pseudo style is missing")
	}
	before := sess.WorkCounts()
	for current := target; current != nil && current.Element(); current = current.Parent {
		css.SetHover(current, true)
	}
	if err := sess.RestyleAndReflow(512); err != nil {
		t.Fatal(err)
	}
	if got := sess.PseudoStyles[key].Color; got.R != 255 || got.G != 0 || got.B != 0 {
		t.Fatalf("hover pseudo color = %+v, want red", got)
	}
	if sess.WorkCounts().LayoutBuilds != before.LayoutBuilds+1 {
		t.Fatalf("pseudo style update did not rebuild its arena: work %+v, before %+v", sess.WorkCounts(), before)
	}
}

func TestRefreshFullDocRebuildsPipeline(t *testing.T) {
	sess, err := engine.NewSession("<html><body><div>test</div></body></html>", nil, 512)
	if err != nil {
		t.Fatal(err)
	}

	origArena := sess.Arena

	plan := engine.Plan{
		FullDoc: true,
		Reasons: engine.Resize,
	}

	done := sess.Refresh(plan, nil, 512)
	if !done {
		t.Fatal("full-doc plan should do work")
	}
	if sess.Arena == origArena {
		t.Fatal("full-doc plan should rebuild arena")
	}
}

func TestRefreshPartialRebuildsPipeline(t *testing.T) {
	sess, err := engine.NewSession("<html><body><div>test</div></body></html>", nil, 512)
	if err != nil {
		t.Fatal(err)
	}

	origArena := sess.Arena

	plan := engine.Plan{
		Subtree:       []uint32{1},
		LayoutObjects: 1,
		Reasons:       engine.Hover,
	}

	done := sess.Refresh(plan, nil, 512)
	if !done {
		t.Fatal("partial plan with subtrees should do work")
	}
	if sess.Arena == origArena {
		t.Fatal("partial plan should rebuild arena")
	}
}

func TestRestyleAndReflowReusesArenaForColorOnlyChange(t *testing.T) {
	sess, err := engine.NewSession(`<html><head><style>
		#target { color: black; }
		#target.hot { color: red; }
	</style></head><body><p id="target">text</p></body></html>`, nil, 512)
	if err != nil {
		t.Fatal(err)
	}
	target := sess.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element is missing")
	}
	origArena := sess.Arena
	before := sess.WorkCounts()
	target.SetAttribute("class", "hot")
	if err := sess.RestyleAndReflow(512); err != nil {
		t.Fatal(err)
	}
	after := sess.WorkCounts()
	if after.StyleResolves != before.StyleResolves+1 {
		t.Fatalf("style resolves = %d, want %d", after.StyleResolves, before.StyleResolves+1)
	}
	if after.LayoutBuilds != before.LayoutBuilds {
		t.Fatalf("layout builds = %d, want unchanged %d", after.LayoutBuilds, before.LayoutBuilds)
	}
	if sess.Arena != origArena {
		t.Fatal("paint-only style change replaced the layout arena")
	}
	if got := sess.Styles[target.ID].Color; got.R != 255 || got.G != 0 || got.B != 0 {
		t.Fatalf("computed color = %+v, want red", got)
	}
	objID, ok := sess.Arena.ForNode(target.ID)
	if !ok || sess.Arena.Get(objID).Style.Color.R != 255 {
		t.Fatal("reused arena did not receive the new computed style")
	}
	paintedRed := false
	for _, cmd := range sess.Paint(1).All() {
		if cmd.Kind == paint.CmdText && cmd.Text.Color == frame.RGB(255, 0, 0) {
			paintedRed = true
			break
		}
	}
	if !paintedRed {
		t.Fatal("paint list did not use the refreshed text color")
	}
}

func TestRestyleAndReflowRebuildsArenaForWidthChange(t *testing.T) {
	sess, err := engine.NewSession(`<html><head><style>
		#target { width: 100px; }
		#target.wide { width: 200px; }
	</style></head><body><div id="target">text</div></body></html>`, nil, 512)
	if err != nil {
		t.Fatal(err)
	}
	target := sess.Doc.ElementByID("target")
	origArena := sess.Arena
	before := sess.WorkCounts()
	target.SetAttribute("class", "wide")
	if err := sess.RestyleAndReflow(512); err != nil {
		t.Fatal(err)
	}
	if sess.WorkCounts().LayoutBuilds != before.LayoutBuilds+1 {
		t.Fatalf("layout builds = %d, want %d", sess.WorkCounts().LayoutBuilds, before.LayoutBuilds+1)
	}
	if sess.Arena == origArena {
		t.Fatal("layout-affecting width change reused the old arena")
	}
}

func TestRestyleAndReflowRebuildsArenaWhenPseudoBoxExists(t *testing.T) {
	sess, err := engine.NewSession(`<html><head><style>
		#target::before { content: "prefix"; color: black; }
		#target.hot::before { color: red; }
	</style></head><body><p id="target">text</p></body></html>`, nil, 512)
	if err != nil {
		t.Fatal(err)
	}
	target := sess.Doc.ElementByID("target")
	origArena := sess.Arena
	before := sess.WorkCounts()
	target.SetAttribute("class", "hot")
	if err := sess.RestyleAndReflow(512); err != nil {
		t.Fatal(err)
	}
	if sess.WorkCounts().LayoutBuilds != before.LayoutBuilds+1 {
		t.Fatalf("layout builds = %d, want %d when a pseudo box needs refreshed style", sess.WorkCounts().LayoutBuilds, before.LayoutBuilds+1)
	}
	if sess.Arena == origArena {
		t.Fatal("pseudo style update reused the old anonymous box")
	}
	if got := sess.PseudoStyles[style.PseudoKey{NodeID: target.ID, Pseudo: "before"}].Color; got.R != 255 {
		t.Fatalf("pseudo color = %+v, want red", got)
	}
}

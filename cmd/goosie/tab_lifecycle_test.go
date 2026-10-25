package main

import (
	"context"
	"errors"
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// qc:BR-036
func TestIndependentTabNavigation(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab1 := mgr.NewTab()
	tab2 := mgr.NewTab()

	ctx1, cancel1 := context.WithCancel(context.Background())
	tab1.Nav.Cancel = cancel1
	tab1.Nav.Serial = 1

	ctx2, cancel2 := context.WithCancel(context.Background())
	tab2.Nav.Cancel = cancel2
	tab2.Nav.Serial = 1

	cancel1()

	if ctx1.Err() == nil {
		t.Fatal("tab1 not cancelled")
	}
	if ctx2.Err() != nil {
		t.Fatal("tab2 cancelled when tab1 cancelled")
	}
}

// The serial a tab carries is what stops a navigation the user has since superseded from
// landing on top of the one they are waiting for. That only matters inside
// framePath.applyNavResult, which is where the result reaches the tab, the toolbar, and
// the scheduler, so it is tested there rather than by assigning the field.
// qc:BR-014
func TestStaleNavResultCannotPaintOverTheCurrentOne(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 9
	tab.Nav.Loading = true
	tab.SetLoading(true)
	tab.SetURL("https://current/")

	_, stale := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: tab.ID(), serial: 5, url: "https://stale/", layer: stale})

	if tab.URL() != "https://current/" {
		t.Errorf("tab.URL() = %q after a superseded result; the old navigation took the tab over", tab.URL())
	}
	if tab.Layer() != nil {
		t.Error("tab.Layer() was replaced by a superseded result")
	}
	if got := f.sched.PlanStats().Published; got != 0 {
		t.Errorf("Published = %d, want 0; a superseded result pushed a frame plan to the rasterizer", got)
	}
	if !tab.Nav.Loading {
		t.Error("Nav.Loading was cleared by a superseded result; the spinner stopped while the real load runs")
	}
	if !tab.Loading() {
		t.Error("Loading was cleared by a superseded result")
	}
}

// An error from a navigation the user already replaced is not an error the user should
// see: they moved on, and the load they are waiting on may still succeed.
func TestStaleNavErrorDoesNotSurface(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 4
	tab.Nav.Loading = true
	tab.SetLoading(true)

	f.applyNavResult(navResult{tabID: tab.ID(), serial: 2, url: "https://stale/", err: errors.New("boom")})

	if tab.Error() != "" {
		t.Errorf("tab.Error() = %q, want empty; a superseded failure was reported", tab.Error())
	}
	if !tab.Nav.Loading {
		t.Error("Nav.Loading was cleared by a superseded error result")
	}
	if !tab.Loading() {
		t.Error("Loading was cleared by a superseded error result")
	}
}

// The positive control: the same call with the serial the tab is actually waiting on does
// take the tab over and does publish a plan. Without it the two tests above could pass on
// a framePath that ignores every result.
func TestCurrentNavResultIsApplied(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 5
	tab.Nav.Loading = true
	tab.SetLoading(true)

	_, layer := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: tab.ID(), serial: 5, url: "https://current/", layer: layer})

	if tab.URL() != "https://current/" {
		t.Errorf("tab.URL() = %q, want https://current/", tab.URL())
	}
	if tab.Layer() == nil {
		t.Error("tab.Layer() not set by the result the tab was waiting for")
	}
	if got := f.sched.PlanStats().Published; got != 1 {
		t.Errorf("Published = %d, want 1; the current result should have reached the rasterizer", got)
	}
	if tab.Nav.Loading || tab.Loading() {
		t.Error("the tab is still marked loading after its own result landed")
	}
}

// qc:BR-013
func TestStopTabCancelsOnlyActiveNavigationAndKeepsDisplayedDocument(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	other := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(tab.ID())

	tab.SetURL("https://current/")
	_, currentLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	tab.SetLayer(currentLayer)
	tab.SetLoading(true)
	activeCtx, activeCancel := context.WithCancel(context.Background())
	tab.Nav.Mu.Lock()
	tab.Nav.Cancel = activeCancel
	tab.Nav.Serial = 9
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	otherCtx, otherCancel := context.WithCancel(context.Background())
	t.Cleanup(otherCancel)
	other.Nav.Cancel = otherCancel
	other.Nav.Serial = 3
	other.Nav.Loading = true

	f.toolbar = toolbar.NewState(800, nil)
	f.toolbar.SyncFromTab("https://typed-destination/", true, "", tab.History())
	f.stopTab()

	if activeCtx.Err() != context.Canceled {
		t.Fatal("Stop did not cancel the active tab's navigation")
	}
	if otherCtx.Err() != nil {
		t.Fatal("Stop canceled a background tab's navigation")
	}
	if tab.Nav.Loading || tab.Loading() {
		t.Fatal("active tab still reports loading after Stop")
	}
	if tab.Nav.Cancel != nil {
		t.Fatal("active tab retained its canceled navigation function")
	}
	if tab.URL() != "https://current/" || tab.Layer() != currentLayer {
		t.Fatalf("Stop changed displayed document: URL=%q layer=%p, want current URL and layer %p", tab.URL(), tab.Layer(), currentLayer)
	}
	if tab.Error() != "" {
		t.Fatalf("tab error = %q after Stop, want empty", tab.Error())
	}
	if f.toolbar.Loading || f.toolbar.Error != "" || f.toolbar.URL != tab.URL() || f.toolbar.Input != tab.URL() {
		t.Fatalf("toolbar after Stop = loading:%v error:%q URL:%q Input:%q", f.toolbar.Loading, f.toolbar.Error, f.toolbar.URL, f.toolbar.Input)
	}

	_, staleLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 512})
	f.applyNavResult(navResult{tabID: tab.ID(), serial: 9, url: "https://stopped/", layer: staleLayer, err: context.Canceled})
	if tab.URL() != "https://current/" || tab.Layer() != currentLayer || tab.Error() != "" {
		t.Fatal("late canceled result changed the displayed page or surfaced an error")
	}
}

// qc:BR-027
// A background tab finishing its load updates itself and nothing on screen, which is what
// makes tabs independent. The foreground keeps its document and its published plan.
func TestBackgroundTabResultLeavesTheForegroundAlone(t *testing.T) {
	f := newNavTestPath(t)
	background := f.tabMgr.Active()
	foreground := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(foreground.ID())
	if f.tabMgr.Active() != foreground {
		t.Fatal("SwitchTo did not change the active tab")
	}

	background.Nav.Serial = 1
	foreground.Nav.Serial = 8
	foreground.SetURL("https://foreground/")
	_, foregroundLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: foreground.ID(), serial: 8, url: "https://foreground/", layer: foregroundLayer})
	before := f.sched.PlanStats().Published

	_, backgroundLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 512})
	f.applyNavResult(navResult{tabID: background.ID(), serial: 1, url: "https://background/", layer: backgroundLayer})

	if background.URL() != "https://background/" || background.Layer() == nil {
		t.Errorf("background tab not updated: url=%q layer==nil:%v", background.URL(), background.Layer() == nil)
	}
	if foreground.URL() != "https://foreground/" || foreground.Layer() != foregroundLayer {
		t.Error("a background tab finishing stole the foreground tab's document")
	}
	if got := f.sched.PlanStats().Published; got != before {
		t.Errorf("Published = %d, want %d; a background load repainted the visible tab", got, before)
	}
}

// TestOutOfOrderResultsLandOnTheir OwnTabs pins independent tab navigation
// under completion reorder: the background tab finishing first must not
// disturb the foreground, and the foreground finishing later lands exactly
// once with its own history intact.
// qc:BR-028
func TestOutOfOrderResultsLandOnTheirOwnTabs(t *testing.T) {
	f := newNavTestPath(t)
	foreground := f.tabMgr.Active()
	background := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(foreground.ID())

	foreground.History().Push("https://fg-old/")
	foreground.Nav.Serial = 3
	background.History().Push("https://bg-old/")
	background.Nav.Serial = 5

	_, bgLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 512})
	f.applyNavResult(navResult{tabID: background.ID(), serial: 5, url: "https://bg-new/", layer: bgLayer})

	if background.URL() != "https://bg-new/" || background.Layer() == nil {
		t.Errorf("background tab not updated: url=%q layer==nil:%v", background.URL(), background.Layer() == nil)
	}
	if foreground.URL() != "" || foreground.Layer() != nil {
		t.Errorf("foreground disturbed by background completion: url=%q layer==nil:%v", foreground.URL(), foreground.Layer() == nil)
	}
	if got := f.sched.PlanStats().Published; got != 0 {
		t.Errorf("Published = %d, want 0; no active-tab frame was ready", got)
	}

	_, fgLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: foreground.ID(), serial: 3, url: "https://fg-new/", layer: fgLayer})

	if foreground.URL() != "https://fg-new/" || foreground.Layer() == nil {
		t.Errorf("foreground tab not updated: url=%q layer==nil:%v", foreground.URL(), foreground.Layer() == nil)
	}
	if got := f.sched.PlanStats().Published; got != 1 {
		t.Errorf("Published = %d, want 1; the foreground result must reach the rasterizer once", got)
	}
	fgEntries, _ := foreground.History().Entries()
	if len(fgEntries) != 2 || fgEntries[1] != "https://fg-new/" {
		t.Errorf("foreground history = %v, want [fg-old fg-new]", fgEntries)
	}
	bgEntries, _ := background.History().Entries()
	if len(bgEntries) != 2 || bgEntries[1] != "https://bg-new/" {
		t.Errorf("background history = %v, want [bg-old bg-new]", bgEntries)
	}
}

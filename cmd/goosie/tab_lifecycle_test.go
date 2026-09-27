package main

import (
	"context"
	"errors"
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/tabs"
)

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
func TestStaleNavResultCannotPaintOverTheCurrentOne(t *testing.T) {
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 9
	tab.Nav.Loading = true
	tab.Loading = true
	tab.URL = "https://current/"

	_, stale := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: tab.ID, serial: 5, url: "https://stale/", layer: stale})

	if tab.URL != "https://current/" {
		t.Errorf("tab.URL = %q after a superseded result; the old navigation took the tab over", tab.URL)
	}
	if tab.Layer != nil {
		t.Error("tab.Layer was replaced by a superseded result")
	}
	if got := f.sched.PlanStats().Published; got != 0 {
		t.Errorf("Published = %d, want 0; a superseded result pushed a frame plan to the rasterizer", got)
	}
	if !tab.Nav.Loading {
		t.Error("Nav.Loading was cleared by a superseded result; the spinner stopped while the real load runs")
	}
	if !tab.Loading {
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
	tab.Loading = true

	f.applyNavResult(navResult{tabID: tab.ID, serial: 2, url: "https://stale/", err: errors.New("boom")})

	if tab.Error != "" {
		t.Errorf("tab.Error = %q, want empty; a superseded failure was reported", tab.Error)
	}
	if !tab.Nav.Loading {
		t.Error("Nav.Loading was cleared by a superseded error result")
	}
	if !tab.Loading {
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
	tab.Loading = true

	_, layer := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: tab.ID, serial: 5, url: "https://current/", layer: layer})

	if tab.URL != "https://current/" {
		t.Errorf("tab.URL = %q, want https://current/", tab.URL)
	}
	if tab.Layer == nil {
		t.Error("tab.Layer not set by the result the tab was waiting for")
	}
	if got := f.sched.PlanStats().Published; got != 1 {
		t.Errorf("Published = %d, want 1; the current result should have reached the rasterizer", got)
	}
	if tab.Nav.Loading || tab.Loading {
		t.Error("the tab is still marked loading after its own result landed")
	}
}

// A background tab finishing its load updates itself and nothing on screen, which is what
// makes tabs independent. The foreground keeps its document and its published plan.
func TestBackgroundTabResultLeavesTheForegroundAlone(t *testing.T) {
	f := newNavTestPath(t)
	background := f.tabMgr.Active()
	foreground := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(foreground.ID)
	if f.tabMgr.Active() != foreground {
		t.Fatal("SwitchTo did not change the active tab")
	}

	background.Nav.Serial = 1
	foreground.Nav.Serial = 8
	foreground.URL = "https://foreground/"
	_, foregroundLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 256})
	f.applyNavResult(navResult{tabID: foreground.ID, serial: 8, url: "https://foreground/", layer: foregroundLayer})
	before := f.sched.PlanStats().Published

	_, backgroundLayer := paint.BuildLayer(paint.SceneSpec{DocHeight: 512})
	f.applyNavResult(navResult{tabID: background.ID, serial: 1, url: "https://background/", layer: backgroundLayer})

	if background.URL != "https://background/" || background.Layer == nil {
		t.Errorf("background tab not updated: url=%q layer==nil:%v", background.URL, background.Layer == nil)
	}
	if foreground.URL != "https://foreground/" || foreground.Layer != foregroundLayer {
		t.Error("a background tab finishing stole the foreground tab's document")
	}
	if got := f.sched.PlanStats().Published; got != before {
		t.Errorf("Published = %d, want %d; a background load repainted the visible tab", got, before)
	}
}

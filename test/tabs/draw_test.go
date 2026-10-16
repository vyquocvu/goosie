package tabs_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestDrawDoesNotPanicOnEmptyManager(t *testing.T) {
	mgr := tabs.NewManager(nil)
	buf := frame.NewBitmap(800, 76)
	tabs.DrawTabBar(buf, mgr, 0, nil, 1)
}

func TestDrawDoesNotPanicWithTabs(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	mgr.NewTab()
	buf := frame.NewBitmap(800, 76)
	tabs.DrawTabBar(buf, mgr, 0, nil, 1)
}

func TestDrawDoesNotPanicOnNilBitmap(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tabs.DrawTabBar(nil, mgr, 0, nil, 1)
}

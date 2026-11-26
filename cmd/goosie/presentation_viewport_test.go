package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestResizeUpdatesDocumentViewportBeforeRepaint(t *testing.T) {
	f := &framePath{
		config: config{width: 800, height: 600, dpr: 1},
		sched: raster.NewScheduler(nil, nil, frame.Viewport{
			Size: frame.Size{W: 800, H: 600},
		}, 1, raster.Pref{}),
		tabMgr: tabs.NewManager(nil),
	}
	f.handleResize(1200, 900)
	got := f.documentViewport()
	want := frame.Size{W: 1200, H: 900}
	if got != want {
		t.Errorf("documentViewport() = %v, want %v after handleResize", got, want)
	}
}

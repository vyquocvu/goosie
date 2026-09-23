package main

import (
	"testing"
	"time"
)

func TestNavRepro(t *testing.T) {
	c := config{width: 720, height: 480, dpr: 2, private: true, downloadDir: t.TempDir()}
	f, err := build(c)
	if err != nil {
		t.Fatal(err)
	}
	// What the fixed resize path reports for a 1440x960 device window at dpr 2.
	f.handleResize(720, (960-152)/2)
	f.navigateTab("https://example.com/")
	tab := f.tabMgr.Active()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		tab.Nav.Mu.Lock()
		loading := tab.Nav.Loading
		tab.Nav.Mu.Unlock()
		if !loading {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("loading=%v layer=%v error=%q bounds=%v", tab.Loading, tab.Layer != nil, tab.Error, tab.Layer.Bounds)
	if tab.Layer == nil {
		t.Fatal("no layer after navigation")
	}
	// Body of example.com is ~432 logical px wide, centered in a 720 logical
	// viewport: X0 near 288 device px. A double-width layout would start ~1008.
	if tab.Layer.Bounds.X0 > 500 {
		t.Fatalf("document laid out too wide: bounds=%v", tab.Layer.Bounds)
	}
}

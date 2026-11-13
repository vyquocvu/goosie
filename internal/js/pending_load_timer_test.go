package js

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
)

// GOOSIE-FADESETTLE-001 owns the readiness rule, and the rule is only as good
// as the distinction this package makes: the timers a page queued while it was
// still loading are the ones a host must wait for, and every timer it queues
// afterwards is polling or interaction that a screenshot must not wait for.
// These are white-box because the mark is per-Runtime state no exported call
// reads directly. Measured reference for why the split matters: Chromium
// 131.0.6778.33 sampling https://www.gov.vn with setTimeout/setInterval hooked
// from document start (2026-11-13, viewport 1280x800) logged 81 timers before
// the load event, one of them 24 hours out, and the visible layout area never
// moved after 0ms; vnexpress logged 938 before load and moved once, between
// 200ms and 300ms.

func markedRuntime(t *testing.T, url string) *Runtime {
	t.Helper()
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	doc, err := dom.ParseBounded(`<!doctype html><html><body><div id="a"></div></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(Options{URL: url, DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

// qc:BR-283
func TestTimersQueuedBeforeLoadAreMarkedAsStartupWork(t *testing.T) {
	rt := markedRuntime(t, "https://example.test/")
	if err := rt.Run(`setTimeout(function(){}, 400)`, "boot.js"); err != nil {
		t.Fatal(err)
	}
	due, ok := rt.PendingLoadTimerDelay()
	if !ok {
		t.Fatal("a timer queued while the document read loading was not startup work")
	}
	if due <= 300*time.Millisecond || due > 400*time.Millisecond {
		t.Fatalf("startup timer due in %v, want just under 400ms", due)
	}
	// The earliest one answers the question, not the first one scheduled.
	if err := rt.Run(`setTimeout(function(){}, 20)`, "boot2.js"); err != nil {
		t.Fatal(err)
	}
	if due, _ := rt.PendingLoadTimerDelay(); due > 100*time.Millisecond {
		t.Fatalf("earliest startup timer is %v, want the 20ms one", due)
	}
}

// qc:BR-283
func TestTimersQueuedAfterTheLoadStepAreNotStartupWork(t *testing.T) {
	rt := markedRuntime(t, "https://example.test/")
	rt.FireDocumentLifecycle()
	if err := rt.Run(`setTimeout(function(){}, 30)`, "later.js"); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.PendingLoadTimerDelay(); ok {
		t.Fatal("a timer queued after the load step reported startup work")
	}
}

// One firing is the whole debt. A repeating timer the page boots
// has to be waited for once and then dropped, or a 100ms heartbeat would hold
// every screenshot run on the page for as long as the run is allowed to live.
//
// qc:BR-283
func TestStartupIntervalCountsUntilItsFirstFire(t *testing.T) {
	rt := markedRuntime(t, "https://example.test/")
	if err := rt.Run(`setInterval(function(){}, 10)`, "boot.js"); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.PendingLoadTimerDelay(); !ok {
		t.Fatal("a boot interval reported no startup work before its first fire")
	}
	time.Sleep(20 * time.Millisecond)
	rt.Tick()
	if _, ok := rt.PendingLoadTimerDelay(); ok {
		t.Fatal("the boot interval still counted after firing once")
	}
}

// qc:BR-283
func TestPendingLoadTimerIncludesChildBrowsingContext(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	parentDoc, err := dom.ParseBounded(`<!doctype html><html><body><iframe id="child"></iframe></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := New(Options{URL: "https://example.test/", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	childDoc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	child, err := New(Options{URL: "https://example.test/frame", DOM: childDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	frame := parentDoc.ElementByID("child")
	if frame == nil {
		t.Fatal("parsed iframe not found")
	}
	parent.iframeChildRuntimes = make(map[*dom.Node]*Runtime)
	parent.iframeChildRuntimes[frame] = child

	if _, ok := parent.PendingLoadTimerDelay(); ok {
		t.Fatal("an empty parent reported startup work")
	}
	if err := child.Run(`setTimeout(function(){}, 180)`, "child-boot.js"); err != nil {
		t.Fatal(err)
	}
	due, ok := parent.PendingLoadTimerDelay()
	if !ok {
		t.Fatal("a child frame's startup timer was invisible to the embedding document")
	}
	if due <= 100*time.Millisecond || due > 180*time.Millisecond {
		t.Fatalf("child startup timer due in %v, want just under 180ms", due)
	}
}

// qc:BR-283
func TestClosedRuntimeOwesNoStartupWork(t *testing.T) {
	rt := markedRuntime(t, "https://example.test/")
	if err := rt.Run(`setTimeout(function(){}, 300)`, "boot.js"); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.PendingLoadTimerDelay(); !ok {
		t.Fatal("nothing pending before close")
	}
	rt.Close()
	if _, ok := rt.PendingLoadTimerDelay(); ok {
		t.Fatal("a closed runtime still asked the host to wait for it")
	}
}

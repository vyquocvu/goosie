package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// The viewport expectations below are measured against Chromium with the
// viewportTargets block of test/wpt/resources/element_scroll_probe.py:
//
//   - the event is targeted at the document with bubbles=true, cancelable=false
//     and isTrusted=true, so a window listener receives it while a listener on
//     the document element does not (the document element is not on the
//     propagation path from the document);
//   - it is queued like an element scroll: nothing inline, nothing after a task,
//     exactly one before the next animation frame callback;
//   - one turn gets one event however many writes or axes moved, and a write
//     that does not move the offset contributes nothing;
//   - every channel that moves the viewport feeds the same queue — the
//     document-element setters, window.scrollTo/scrollBy, the scrolling
//     element's own scrollTo, a wheel gesture, and the frame path's signal.
const viewportPage = `<!doctype html><html><head></head><body style="margin:0">
<div style="width:1200px;height:2000px"></div>
</body></html>`

// watchViewportScroll installs three recorders, one per candidate target, each
// logging the event it receives.
func watchViewportScroll(t *testing.T, s *engine.Session, where string) {
	t.Helper()
	if _, err := s.EvaluateJS(`
		window.__seen = [];
		window.__log = function (event) {
			window.__seen.push('target=' + (event.target === document ? 'document' : event.target.nodeName) +
				':bubbles=' + event.bubbles + ':cancelable=' + event.cancelable + ':trusted=' + event.isTrusted);
		};
		` + where + `.addEventListener('scroll', window.__log);
	`); err != nil {
		t.Fatal(err)
	}
}

func viewportReport(t *testing.T, s *engine.Session, expr string) string {
	t.Helper()
	got, err := s.EvaluateJS(expr)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A viewport scroll is not delivered inline nor on the next task: the event
// reaches the page at the frame flush, with the measured flags.
func TestViewportScrollEventIsQueuedToTheFrame(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	if _, err := s.EvaluateJS(`
		document.documentElement.scrollTop = 300;
		window.__before = window.__seen.length;
	`); err != nil {
		t.Fatal(err)
	}
	if got := viewportReport(t, s, `String(window.__before) + ':' + String(window.__seen.length)`); got != "0:0" {
		t.Errorf("queueing = %q, want %q (nothing inline, nothing before the frame)", got, "0:0")
	}
	s.PumpJS(800)
	want := "target=document:bubbles=true:cancelable=false:trusted=true"
	if got := viewportReport(t, s, `window.__seen.join('|')`); got != want {
		t.Errorf("viewport scroll event = %q, want %q", got, want)
	}
}

// The event is targeted at the document and bubbles, so window listeners see it;
// the document element sits below the document in the tree and never receives it.
func TestViewportScrollEventPropagatesToWindowButNotDocumentElement(t *testing.T) {
	s := scrollSession(t, viewportPage)
	if _, err := s.EvaluateJS(`
		window.__window = 0;
		window.__root = 0;
		window.addEventListener('scroll', function () { window.__window++; });
		document.documentElement.addEventListener('scroll', function () { window.__root++; });
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EvaluateJS(`document.documentElement.scrollTop = 300;`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	if got := viewportReport(t, s, `String(window.__window) + ':' + String(window.__root)`); got != "1:0" {
		t.Errorf("window:documentElement deliveries = %q, want %q", got, "1:0")
	}
}

// An element scroll does not bubble, so a bubble-phase window listener never
// sees it; the window is still on the propagation path, so a capture listener
// there does. Measured in Chromium as bubble=0, capture=1.
func TestElementScrollEventReachesWindowCaptureOnly(t *testing.T) {
	s := scrollSession(t, scrollPage)
	if _, err := s.EvaluateJS(`
		window.__bubble = 0;
		window.__capture = 0;
		window.addEventListener('scroll', function () { window.__bubble++; });
		window.addEventListener('scroll', function () { window.__capture++; }, true);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EvaluateJS(`document.getElementById('box').scrollTop = 40;`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	if got := viewportReport(t, s, `String(window.__bubble) + ':' + String(window.__capture)`); got != "0:1" {
		t.Errorf("window bubble:capture deliveries = %q, want %q", got, "0:1")
	}
}

// The horizontal axis is a viewport scroll too: a lone scrollLeft write moves
// window.scrollX and owes the same queued event.
func TestViewportScrollXAxisQueuesItsOwnEvent(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	if _, err := s.EvaluateJS(`document.documentElement.scrollLeft = 40;`); err != nil {
		t.Fatal(err)
	}
	if got := viewportReport(t, s, `String(window.__seen.length)`); got != "0" {
		t.Errorf("deliveries before the flush = %q, want %q", got, "0")
	}
	s.PumpJS(800)
	got := viewportReport(t, s, `[window.__seen.length, window.scrollX, window.scrollY].join(',')`)
	if want := "1,40,0"; got != want {
		t.Errorf("events,scrollX,scrollY = %q, want %q", got, want)
	}
}

// Several writes in one turn — including the axis that only the horizontal
// setter moves — are coalesced into a single event.
func TestViewportScrollEventQueuesOnePerTurnForBothAxes(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	if _, err := s.EvaluateJS(`
		document.documentElement.scrollTop = 150;
		document.documentElement.scrollTop = 170;
		document.documentElement.scrollLeft = 80;
	`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	got := viewportReport(t, s, `[window.__seen.length, window.scrollX, window.scrollY].join(',')`)
	if want := "1,80,170"; got != want {
		t.Errorf("events,scrollX,scrollY = %q, want %q", got, want)
	}
}

// A write that leaves the offset where it was fires nothing; the next real move
// fires again.
func TestViewportScrollEventQueuesNothingWithoutAChange(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	if _, err := s.EvaluateJS(`document.documentElement.scrollTop = 100;`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	if got := viewportReport(t, s, `String(window.__seen.length)`); got != "1" {
		t.Fatalf("after the first move = %q, want %q", got, "1")
	}
	if _, err := s.EvaluateJS(`document.documentElement.scrollTop = 100;`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	if got := viewportReport(t, s, `String(window.__seen.length)`); got != "1" {
		t.Errorf("after a write that did not move = %q, want %q", got, "1")
	}
	if _, err := s.EvaluateJS(`document.documentElement.scrollTop = 250;`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	if got := viewportReport(t, s, `String(window.__seen.length)`); got != "2" {
		t.Errorf("after the next move = %q, want %q", got, "2")
	}
}

// window.scrollTo() and window.scrollBy() move the same offset the setters do,
// so they owe the same queued event.
func TestWindowScrollMethodsQueueViewportScroll(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	if _, err := s.EvaluateJS(`window.scrollTo(0, 400);`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	if got := viewportReport(t, s, `String(window.__seen.length) + ':' + String(window.scrollY)`); got != "1:400" {
		t.Fatalf("after scrollTo = %q, want %q", got, "1:400")
	}
	if _, err := s.EvaluateJS(`window.scrollBy(20, 20);`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	got := viewportReport(t, s, `[window.__seen.length, window.scrollX, window.scrollY].join(',')`)
	if want := "2,20,420"; got != want {
		t.Errorf("events,scrollX,scrollY = %q, want %q", got, want)
	}
}

// document.scrollingElement.scrollTo() is the same viewport scroll seen through
// the element API, so it queues rather than writing into the element map.
func TestScrollingElementScrollToQueuesViewportScroll(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	if _, err := s.EvaluateJS(`document.scrollingElement.scrollTo({ top: 500, left: 100 });`); err != nil {
		t.Fatal(err)
	}
	s.PumpJS(800)
	got := viewportReport(t, s, `[window.__seen.length, window.scrollX, window.scrollY,
		document.documentElement.scrollTop].join(',')`)
	if want := "1,100,500,500"; got != want {
		t.Errorf("events,scrollX,scrollY,scrollTop = %q, want %q", got, want)
	}
}

// The frame path's own scroll signal is only a statement that the viewport
// moved: it joins the same queue, so three signals in one turn deliver one event.
func TestEngineScrollSignalQueuesOneViewportEvent(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	s.FireScrollEvent()
	s.FireScrollEvent()
	s.FireScrollEvent()
	if got := viewportReport(t, s, `String(window.__seen.length)`); got != "0" {
		t.Errorf("deliveries before the flush = %q, want %q", got, "0")
	}
	s.PumpJS(800)
	want := "target=document:bubbles=true:cancelable=false:trusted=true"
	if got := viewportReport(t, s, `window.__seen.join('|')`); got != want {
		t.Errorf("viewport scroll event = %q, want %q", got, want)
	}
}

// A wheel gesture that the page did not cancel moves the viewport through the
// same setter, so it owes the same event.
func TestWheelScrollQueuesViewportScroll(t *testing.T) {
	s := scrollSession(t, viewportPage)
	watchViewportScroll(t, s, "document")
	s.TestDriverWheelAction(10, 10, 0, 250)
	if got := viewportReport(t, s, `String(window.__seen.length)`); got != "0" {
		t.Errorf("deliveries before the flush = %q, want %q", got, "0")
	}
	s.PumpJS(800)
	got := viewportReport(t, s, `[window.__seen.length, window.scrollY].join(',')`)
	if want := "1,250"; got != want {
		t.Errorf("events,scrollY = %q, want %q", got, want)
	}
}

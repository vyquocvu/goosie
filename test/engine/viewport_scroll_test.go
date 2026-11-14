package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

const viewportPage = `<!doctype html><html><head></head><body style="margin:0">
<div style="width:1200px;height:2000px"></div>
</body></html>`

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

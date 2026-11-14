package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func scrollSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600), engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

const scrollPage = `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:scroll;width:100px;height:100px"><div style="width:200px;height:200px"></div></div>
</body></html>`

func watchScroll(t *testing.T, s *engine.Session) {
	t.Helper()
	if _, err := s.EvaluateJS(`
		window.__seen = [];
		var box = document.getElementById('box');
		box.addEventListener('scroll', function (event) {
			window.__seen.push('target=' + event.target.id + ':bubbles=' + event.bubbles +
				':cancelable=' + event.cancelable + ':trusted=' + event.isTrusted);
		});
	`); err != nil {
		t.Fatal(err)
	}
}

func scrollWrite(t *testing.T, s *engine.Session, writes string) {
	t.Helper()
	if _, err := s.EvaluateJS(writes); err != nil {
		t.Fatal(err)
	}
}

func scrollReport(t *testing.T, s *engine.Session, expr string) string {
	t.Helper()
	got, err := s.EvaluateJS(expr)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestScrollContainerReportsItsOverflowingExtent(t *testing.T) {
	s := scrollSession(t, scrollPage)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		[box.clientWidth, box.clientHeight, box.scrollWidth, box.scrollHeight].join(',');
	`)
	if want := "100,100,200,200"; got != want {
		t.Errorf("client,scroll extents = %q, want %q", got, want)
	}
}

func TestScrollExtentIncludesTrailingPadding(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:scroll;width:100px;height:100px;padding:0 40px 60px 0"><div style="width:200px;height:200px"></div></div>
</body></html>`)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		box.scrollTop = 1000;
		box.scrollLeft = 1000;
		[box.clientWidth, box.clientHeight, box.scrollWidth, box.scrollHeight, box.scrollTop, box.scrollLeft].join(',');
	`)
	if want := "140,160,240,260,100,100"; got != want {
		t.Errorf("padded extents and clamps = %q, want %q", got, want)
	}
}

func TestNestedScrollContainerLimitsTheExtent(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:scroll;width:100px;height:100px">
  <div style="overflow:scroll;width:100px;height:100px"><div style="width:200px;height:200px"></div></div>
</div>
</body></html>`)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		box.scrollTop = 50;
		[box.scrollHeight, box.scrollTop].join(',');
	`)
	if want := "100,0"; got != want {
		t.Errorf("extent and offset with a nested scroller = %q, want %q", got, want)
	}
}

func TestScrollExtentIncludesDescendantMargins(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:scroll;width:100px;height:100px"><div style="width:100px;height:100px;margin:0 60px 60px 0"></div></div>
</body></html>`)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		box.scrollTop = 1000;
		box.scrollLeft = 1000;
		[box.clientWidth, box.clientHeight, box.scrollWidth, box.scrollHeight, box.scrollTop, box.scrollLeft].join(',');
	`)
	if want := "100,100,160,160,60,60"; got != want {
		t.Errorf("margin-extended extents and clamps = %q, want %q", got, want)
	}
}

func TestScrollExtentIncludesAbsposDescendants(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:scroll;position:relative;width:100px;height:100px"><div style="position:absolute;left:0;top:0;width:200px;height:200px"></div></div>
</body></html>`)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		box.scrollTop = 1000;
		box.scrollLeft = 1000;
		[box.scrollWidth, box.scrollHeight, box.scrollTop, box.scrollLeft].join(',');
	`)
	if want := "200,200,100,100"; got != want {
		t.Errorf("abspos extents and clamps = %q, want %q", got, want)
	}
}

func TestScrollExtentIgnoresFixedPositionDescendants(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:scroll;position:relative;width:100px;height:100px"><div style="position:fixed;left:0;top:0;width:300px;height:300px"></div></div>
</body></html>`)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		box.scrollTop = 1000;
		box.scrollLeft = 1000;
		[box.scrollWidth, box.scrollHeight, box.scrollTop, box.scrollLeft].join(',');
	`)
	if want := "100,100,0,0"; got != want {
		t.Errorf("fixed-descendant extents and clamps = %q, want %q", got, want)
	}
}

func TestElementScrollOffsetClampsToItsExtent(t *testing.T) {
	s := scrollSession(t, scrollPage)
	got := scrollReport(t, s, `
		var box = document.getElementById('box');
		box.scrollTop = 1000;
		box.scrollLeft = 1000;
		var over = box.scrollTop + ',' + box.scrollLeft;
		box.scrollTop = 30;
		box.scrollLeft = -10;
		over + '|' + box.scrollTop + ',' + box.scrollLeft;
	`)
	if want := "100,100|30,0"; got != want {
		t.Errorf("scroll offsets = %q, want %q", got, want)
	}
}

func TestElementScrollOffsetQueuesOneScrollEvent(t *testing.T) {
	s := scrollSession(t, scrollPage)
	watchScroll(t, s)
	scrollWrite(t, s, `box.scrollTop = 40;
		window.__inline = window.__seen.length;`)
	if got := scrollReport(t, s, `String(window.__inline) + ':' + String(window.__seen.length)`); got != "0:0" {
		t.Errorf("queueing = %q, want %q (nothing inline, nothing before the frame)", got, "0:0")
	}
	s.PumpJS(800)
	want := "target=box:bubbles=false:cancelable=false:trusted=true"
	if got := scrollReport(t, s, `window.__seen.join('|')`); got != want {
		t.Errorf("scroll event = %q, want %q", got, want)
	}
}

func TestElementScrollOffsetQueuesNothingWithoutAChange(t *testing.T) {
	s := scrollSession(t, scrollPage)
	watchScroll(t, s)
	scrollWrite(t, s, `
		box.scrollTop = 20;
		box.scrollLeft = 20;
		box.scrollTop = 20;
	`)
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "1" {
		t.Fatalf("events for a two-axis scroll in one turn = %q, want 1", got)
	}
	scrollWrite(t, s, `box.scrollTop = 20;`)
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "1" {
		t.Errorf("events after a write that changed nothing = %q, want 1", got)
	}
	scrollWrite(t, s, `box.scrollLeft = 0;`)
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "2" {
		t.Errorf("events after one real change = %q, want 2", got)
	}
}

func TestNonScrollableElementKeepsZeroScrollOffset(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="width:100px;height:100px"><div style="width:200px;height:200px"></div></div>
</body></html>`)
	watchScroll(t, s)
	scrollWrite(t, s, `box.scrollTop = 50;`)
	s.PumpJS(800)
	got := scrollReport(t, s, `String(box.scrollTop) + ':' + String(window.__seen.length) + ':' + String(box.scrollHeight)`)
	if want := "0:0:200"; got != want {
		t.Errorf("overflow:visible offset, events, extent = %q, want %q", got, want)
	}
}

func TestClippedElementScrollsProgrammatically(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="overflow:hidden;width:100px;height:100px"><div style="width:200px;height:200px"></div></div>
</body></html>`)
	watchScroll(t, s)
	scrollWrite(t, s, `box.scrollTop = 1000;`)
	if got := scrollReport(t, s, `String(box.scrollTop)`); got != "100" {
		t.Errorf("clamped offset immediately after the write = %q, want 100", got)
	}
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "1" {
		t.Errorf("events for a programmatically scrolled clip = %q, want 1", got)
	}
}

func TestElementScrollToAndScrollByClampAndQueue(t *testing.T) {
	s := scrollSession(t, scrollPage)
	watchScroll(t, s)
	scrollWrite(t, s, `box.scrollTo({top: 1000, left: 1000});`)
	if got, want := scrollReport(t, s, `String(box.scrollTop) + ',' + String(box.scrollLeft)`), "100,100"; got != want {
		t.Fatalf("scrollTo clamped offsets = %q, want %q", got, want)
	}
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "1" {
		t.Fatalf("events after scrollTo = %q, want 1", got)
	}
	scrollWrite(t, s, `box.scrollBy({top: -30, left: 10});`)
	if got, want := scrollReport(t, s, `String(box.scrollTop) + ',' + String(box.scrollLeft)`), "70,100"; got != want {
		t.Fatalf("scrollBy offsets = %q, want %q", got, want)
	}
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "2" {
		t.Fatalf("events after scrollBy = %q, want 2", got)
	}
	scrollWrite(t, s, `box.scrollTo(50, 60); box.scrollBy(-1000, -1000);`)
	if got, want := scrollReport(t, s, `String(box.scrollTop) + ',' + String(box.scrollLeft)`), "0,0"; got != want {
		t.Fatalf("offsets after the numeric forms = %q, want %q", got, want)
	}
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "3" {
		t.Fatalf("events for two writes in one turn = %q, want 3", got)
	}
	scrollWrite(t, s, `box.scrollTo({top: 0, left: 0});`)
	s.PumpJS(800)
	if got := scrollReport(t, s, `String(window.__seen.length)`); got != "3" {
		t.Errorf("events after a scrollTo that changed nothing = %q, want 3", got)
	}
}

func TestElementScrollToIgnoredOnNonScrollContainer(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div id="box" style="width:100px;height:100px"><div style="width:200px;height:200px"></div></div>
</body></html>`)
	watchScroll(t, s)
	scrollWrite(t, s, `box.scrollTo({top: 50, left: 50}); box.scrollBy({top: 20});`)
	s.PumpJS(800)
	got := scrollReport(t, s, `String(box.scrollTop) + ':' + String(box.scrollLeft) + ':' + String(window.__seen.length)`)
	if want := "0:0:0"; got != want {
		t.Errorf("non-container scrollTo offset and events = %q, want %q", got, want)
	}
}

func TestScrollingElementScrollToMovesTheViewport(t *testing.T) {
	s := scrollSession(t, `<!doctype html><html><head></head><body style="margin:0">
<div style="height:2000px"></div>
</body></html>`)
	scrollWrite(t, s, `document.scrollingElement.scrollTo({top: 250, left: 0});`)
	if got := scrollReport(t, s, `String(window.scrollY)`); got != "250" {
		t.Fatalf("scrollY after scrollingElement.scrollTo = %q, want 250", got)
	}
	scrollWrite(t, s, `document.scrollingElement.scrollBy({top: 120});`)
	if got := scrollReport(t, s, `String(window.scrollY)`); got != "370" {
		t.Errorf("scrollY after scrollingElement.scrollBy = %q, want 370", got)
	}
}

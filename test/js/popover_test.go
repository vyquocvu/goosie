package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestPopover_AttributeAndProperty(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <div id="p1" popover>Auto popover</div>
  <div id="p2" popover="manual">Manual popover</div>
  <div id="p3">Not a popover</div>
</body>
</html>`

	doc := domtest.Parse(html)
	rt, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	// 1. Check popover property
	v, err := rt.Evaluate("document.getElementById('p1').popover")
	if err != nil || v != "auto" {
		t.Errorf("p1.popover = %q, want 'auto', err: %v", v, err)
	}

	v, err = rt.Evaluate("document.getElementById('p2').popover")
	if err != nil || v != "manual" {
		t.Errorf("p2.popover = %q, want 'manual', err: %v", v, err)
	}

	v, err = rt.Evaluate("document.getElementById('p3').popover")
	if err != nil || v != "null" {
		t.Errorf("p3.popover = %q, want 'null', err: %v", v, err)
	}

	// 2. Set popover property
	_, err = rt.Evaluate("document.getElementById('p3').popover = 'auto'")
	if err != nil {
		t.Fatalf("set popover: %v", err)
	}
	p3 := doc.ElementByID("p3")
	if !p3.HasAttribute("popover") || p3.GetAttribute("popover") != "auto" {
		t.Errorf("p3 popover attr = %q, want 'auto'", p3.GetAttribute("popover"))
	}
}

func TestPopover_ShowHideToggle(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <div id="pop" popover>My Popover</div>
  <script>
    window.events = [];
    var el = document.getElementById('pop');
    el.addEventListener('beforetoggle', function(e) { window.events.push('beforetoggle'); });
    el.addEventListener('toggle', function(e) { window.events.push('toggle'); });
  </script>
</body>
</html>`

	doc := domtest.Parse(html)
	rt, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	pop := doc.ElementByID("pop")
	if pop.IsPopoverOpen {
		t.Fatal("popover should initially be closed")
	}

	_, err = rt.Evaluate(`
		window.events = [];
		var el = document.getElementById('pop');
		el.addEventListener('beforetoggle', function(e) { window.events.push('beforetoggle'); });
		el.addEventListener('toggle', function(e) { window.events.push('toggle'); });
	`)
	if err != nil {
		t.Fatalf("setup listeners: %v", err)
	}

	// Show popover
	_, err = rt.Evaluate("document.getElementById('pop').showPopover()")
	if err != nil {
		t.Fatalf("showPopover: %v", err)
	}
	if !pop.IsPopoverOpen {
		t.Fatal("popover should be open after showPopover()")
	}

	// Calling showPopover again should throw TypeError
	_, err = rt.Evaluate("document.getElementById('pop').showPopover()")
	if err == nil {
		t.Fatal("expected showPopover on already open popover to throw error")
	}

	// Hide popover
	_, err = rt.Evaluate("document.getElementById('pop').hidePopover()")
	if err != nil {
		t.Fatalf("hidePopover: %v", err)
	}
	if pop.IsPopoverOpen {
		t.Fatal("popover should be closed after hidePopover()")
	}

	// Toggle popover
	res, err := rt.Evaluate("document.getElementById('pop').togglePopover()")
	if err != nil || res != "true" {
		t.Fatalf("togglePopover open = %v, err: %v", res, err)
	}
	if !pop.IsPopoverOpen {
		t.Fatal("popover should be open after toggle")
	}

	res, err = rt.Evaluate("document.getElementById('pop').togglePopover()")
	if err != nil || res != "false" {
		t.Fatalf("togglePopover close = %v, err: %v", res, err)
	}
	if pop.IsPopoverOpen {
		t.Fatal("popover should be closed after toggle")
	}

	// Verify events
	evs, err := rt.Evaluate("window.events.join(',')")
	if err != nil {
		t.Fatalf("events evaluate: %v", err)
	}
	// Expected beforetoggle, toggle pairs for show, hide, toggle(open), toggle(close)
	wantEvents := "beforetoggle,toggle,beforetoggle,toggle,beforetoggle,toggle,beforetoggle,toggle"
	if evs != wantEvents {
		t.Errorf("events = %q, want %q", evs, wantEvents)
	}
}

func TestPopover_AutoDismissesOtherAutoPopovers(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <div id="pop1" popover="auto">Popover 1</div>
  <div id="pop2" popover="auto">Popover 2</div>
  <div id="man" popover="manual">Manual Popover</div>
</body>
</html>`

	doc := domtest.Parse(html)
	rt, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	pop1 := doc.ElementByID("pop1")
	pop2 := doc.ElementByID("pop2")
	man := doc.ElementByID("man")

	// Open manual
	_, _ = rt.Evaluate("document.getElementById('man').showPopover()")
	// Open pop1
	_, _ = rt.Evaluate("document.getElementById('pop1').showPopover()")

	if !man.IsPopoverOpen || !pop1.IsPopoverOpen {
		t.Fatal("both man and pop1 should be open")
	}

	// Open pop2: should close pop1, but NOT man (manual)
	_, _ = rt.Evaluate("document.getElementById('pop2').showPopover()")

	if pop1.IsPopoverOpen {
		t.Fatal("pop1 should have been closed by opening pop2")
	}
	if !pop2.IsPopoverOpen {
		t.Fatal("pop2 should be open")
	}
	if !man.IsPopoverOpen {
		t.Fatal("manual popover should remain open when auto popover opens")
	}
}

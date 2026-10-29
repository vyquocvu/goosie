package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
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
    el.addEventListener('beforetoggle', function(e) { window.events.push('beforetoggle:'+e.oldState+':'+e.newState+':'+e.cancelable+':'+e.bubbles); });
    el.addEventListener('toggle', function(e) { window.events.push('toggle:'+e.oldState+':'+e.newState+':'+e.cancelable+':'+e.bubbles); });
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
		el.addEventListener('beforetoggle', function(e) { window.events.push('beforetoggle:'+e.oldState+':'+e.newState+':'+e.cancelable+':'+e.bubbles); });
		el.addEventListener('toggle', function(e) { window.events.push('toggle:'+e.oldState+':'+e.newState+':'+e.cancelable+':'+e.bubbles); });
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
	if !rt.Tick() {
		t.Fatal("expected queued toggle event after showPopover()")
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
	if !rt.Tick() {
		t.Fatal("expected queued toggle event after hidePopover()")
	}

	// Toggle popover
	res, err := rt.Evaluate("document.getElementById('pop').togglePopover()")
	if err != nil || res != "true" {
		t.Fatalf("togglePopover open = %v, err: %v", res, err)
	}
	if !pop.IsPopoverOpen {
		t.Fatal("popover should be open after toggle")
	}
	if !rt.Tick() {
		t.Fatal("expected queued toggle event after opening togglePopover()")
	}

	res, err = rt.Evaluate("document.getElementById('pop').togglePopover()")
	if err != nil || res != "false" {
		t.Fatalf("togglePopover close = %v, err: %v", res, err)
	}
	if pop.IsPopoverOpen {
		t.Fatal("popover should be closed after toggle")
	}
	if !rt.Tick() {
		t.Fatal("expected queued toggle event after closing togglePopover()")
	}

	// Verify events
	evs, err := rt.Evaluate("window.events.join(',')")
	if err != nil {
		t.Fatalf("events evaluate: %v", err)
	}
	// Expected beforetoggle, toggle pairs for show, hide, toggle(open), toggle(close)
	wantEvents := "beforetoggle:closed:open:true:false,toggle:closed:open:false:false,beforetoggle:open:closed:false:false,toggle:open:closed:false:false,beforetoggle:closed:open:true:false,toggle:closed:open:false:false,beforetoggle:open:closed:false:false,toggle:open:closed:false:false"
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

func TestPopover_RejectsOpeningDuringReentrantAutoHide(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<div id="first" popover="auto"></div>
<div id="second" popover="auto"></div>
</body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	_, err = rt.Evaluate(`
const first = document.getElementById('first');
const second = document.getElementById('second');
first.showPopover();
let rejected = false;
first.addEventListener('beforetoggle', event => {
  if (event.newState !== 'closed') return;
  try { second.showPopover(); }
  catch (error) { rejected = error.name === 'InvalidStateError'; }
}, {once: true});
first.hidePopover();
window.rejectedReentrantPopoverOpen = rejected;
`)
	if err != nil {
		t.Fatalf("reentrant popover script: %v", err)
	}
	got, err := rt.Evaluate("window.rejectedReentrantPopoverOpen")
	if err != nil || got != "true" {
		t.Fatalf("reentrant showPopover rejected = %v, err=%v; want true", got, err)
	}
	if doc.ElementByID("first").IsPopoverOpen || doc.ElementByID("second").IsPopoverOpen {
		t.Fatal("reentrant failed show should leave both popovers closed")
	}
}

func TestPopover_TrustedPointerUpLightDismiss(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<div id="outside">outside</div>
<div id="popover" popover="auto">content</div>
</body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()
	rt.EnableTestUserActivationHook()

	_, err = rt.Evaluate(`
const popover = document.getElementById('popover');
const outside = document.getElementById('outside');
popover.showPopover();
outside.dispatchEvent(new PointerEvent('pointerup'));
window.syntheticPointerKeepsOpen = popover.matches(':popover-open');
window.__goosiePointerDown(outside, 'pointer-a');
window.__goosiePointerUp(outside, 'pointer-b');
window.trustedPointerDismissed = !popover.matches(':popover-open');
`)
	if err != nil {
		t.Fatalf("pointer dismissal script: %v", err)
	}
	got, err := rt.Evaluate("window.syntheticPointerKeepsOpen + ',' + window.trustedPointerDismissed")
	if err != nil || got != "true,true" {
		t.Fatalf("synthetic/trusted pointer dismissal = %v, err=%v; want true,true", got, err)
	}
	if doc.ElementByID("popover").IsPopoverOpen {
		t.Fatal("trusted outside pointerup should close the auto popover")
	}
}

func TestPopover_AutoKeepsAncestorWhenOpeningNestedPopover(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<div id="parent" popover="auto"><div id="child" popover="auto">child</div></div>
<div id="sibling" popover="auto">sibling</div>
</body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	parent, child, sibling := doc.ElementByID("parent"), doc.ElementByID("child"), doc.ElementByID("sibling")
	if _, err := rt.Evaluate(`document.getElementById('parent').showPopover(); document.getElementById('child').showPopover()`); err != nil {
		t.Fatalf("open nested popovers: %v", err)
	}
	if !parent.IsPopoverOpen || !child.IsPopoverOpen {
		t.Fatal("opening an auto child should keep its auto ancestor open")
	}
	if _, err := rt.Evaluate(`document.getElementById('sibling').showPopover()`); err != nil {
		t.Fatalf("open sibling: %v", err)
	}
	if parent.IsPopoverOpen || child.IsPopoverOpen || !sibling.IsPopoverOpen {
		t.Fatal("opening a sibling should dismiss the nested auto stack only")
	}
}

func TestPopover_AutoKeepsShadowIncludingAncestor(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body><div id="parent" popover="auto"></div></body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()
	rt.EnableTestUserActivationHook()

	if _, err := rt.Evaluate(`
		const parent = document.getElementById('parent');
		const root = parent.attachShadow({mode:'open'});
		const toggle = document.createElement('button');
		const innerContainer = document.createElement('div');
		root.append(toggle, innerContainer);
		const innerRoot = innerContainer.attachShadow({mode:'open'});
		const child = document.createElement('div');
		child.setAttribute('popover', 'auto');
		innerRoot.appendChild(child);
		toggle.onclick = () => child.togglePopover();
		window.childPopover = child;
		parent.showPopover();
		__goosieTestDriverClick(toggle);
	`); err != nil {
		t.Fatalf("open shadow-nested popovers: %v", err)
	}
	if !doc.ElementByID("parent").IsPopoverOpen {
		t.Fatal("opening a shadow-nested auto popover should keep the outer popover open")
	}
	if value, err := rt.Evaluate(`childPopover.matches(':popover-open')`); err != nil || value != "true" {
		t.Fatalf("shadow child :popover-open = %q, err %v; want true", value, err)
	}
}

func TestPopover_AutoOpeningClosesHintStack(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<div id="hint1" popover="hint"></div>
<div id="hint2" popover="hint"></div>
<div id="auto" popover="auto"></div>
</body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	hint1, hint2, auto := doc.ElementByID("hint1"), doc.ElementByID("hint2"), doc.ElementByID("auto")
	if _, err := rt.Evaluate(`
		document.getElementById('hint1').showPopover();
		document.getElementById('hint2').showPopover();
		document.getElementById('auto').showPopover();
	`); err != nil {
		t.Fatalf("open hint and auto popovers: %v", err)
	}
	if hint1.IsPopoverOpen || hint2.IsPopoverOpen || !auto.IsPopoverOpen {
		t.Fatal("opening an auto popover should dismiss the full hint stack")
	}
}

func TestPopover_TargetResolvesInInvokerShadowTree(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body><div id="host"></div></body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	if _, err := rt.Evaluate(`
		const root = document.getElementById('host').attachShadow({mode: 'open'});
		const button = document.createElement('button');
		button.setAttribute('popovertarget', 'inside');
		const popover = document.createElement('div');
		popover.id = 'inside';
		popover.setAttribute('popover', 'auto');
		root.append(button, popover);
		button.click();
		window.shadowPopover = popover;
	`); err != nil {
		t.Fatalf("activate shadow popover: %v", err)
	}
	if value, err := rt.Evaluate(`shadowPopover.matches(':popover-open')`); err != nil || value != "true" {
		t.Fatalf("shadow popover open = %q, err %v; want true", value, err)
	}
}

func TestPopover_AutoStackFollowsActiveInvokerChain(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<div id="p1" popover="auto">parent
  <button popovertarget="p2">open 2</button>
  <div id="p2" popover="auto">second
    <button popovertarget="p3">open 3</button>
  </div>
  <div id="p3" popover="auto">third
    <button popovertarget="p4">open 4</button>
  </div>
  <div id="p4" popover="auto">fourth</div>
</div></body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	if _, err := rt.Evaluate(`
		document.getElementById('p1').showPopover();
		document.getElementById('p1').querySelector('button').click();
		document.getElementById('p2').querySelector('button').click();
		document.getElementById('p3').querySelector('button').click();
	`); err != nil {
		t.Fatalf("open active invoker chain: %v", err)
	}
	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		if !doc.ElementByID(id).IsPopoverOpen {
			t.Fatalf("%s should remain open through the active invoker chain", id)
		}
	}
}

func TestElementFromPointSkipsClosedPopoverSubtree(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<button id="outside">outside</button>
<div id="closed" popover><span id="hidden">hidden</span></div>
</body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()
	rt.SetLayoutGeometryProvider(func(node *dom.Node) (js.LayoutBoxInfo, bool) {
		return js.LayoutBoxInfo{Width: 100, Height: 100}, true
	})
	if value, err := rt.Evaluate(`document.elementFromPoint(10, 10).id`); err != nil || value != "outside" {
		t.Fatalf("elementFromPoint = %q, err %v; want outside", value, err)
	}
}

func TestPopover_RightClickDoesNotLightDismiss(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body>
<button id="outside">outside</button>
<div id="pop" popover="auto">popup</div>
</body>`)
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()
	rt.EnableTestUserActivationHook()
	if _, err := rt.Evaluate(`
		document.getElementById('pop').showPopover();
		const outside = document.getElementById('outside');
		__goosiePointerDown(outside, 'right', 2);
		window.dismissedByRightClick = __goosiePointerUp(outside, 'right', 2);
	`); err != nil {
		t.Fatalf("simulate right click: %v", err)
	}
	if doc.ElementByID("pop").IsPopoverOpen != true {
		t.Fatal("right click must leave the auto popover open")
	}
	if value, err := rt.Evaluate(`dismissedByRightClick`); err != nil || value != "false" {
		t.Fatalf("right-click light-dismiss result = %q, err %v; want false", value, err)
	}
}

package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// eventDOM builds a DOM tree for event tests:
//
//	html
//	  body
//	    div#target
func eventDOM() *dom.Document {
	doc := &dom.Document{}
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	div := doc.NewElement("div")
	div.SetAttribute("id", "target")
	body.AppendChild(div)
	doc.Body = body
	doc.HTML = html
	return doc
}

// newEventRuntime creates a JS runtime with the event test DOM.
func newEventRuntime(t *testing.T, doc *dom.Document) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		URL: "https://example.test/",
		DOM: doc,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// TestAddEventListenerFires verifies that addEventListener("click", fn) fires
// when a click event is dispatched on the element.
func TestAddEventListenerFires(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var gotType = '';
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			gotType = e.type;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Dispatch a click event on the div from Go.
	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("gotType"); !ok || v.Str != "click" {
		t.Errorf("gotType = %+v, want %q", v, "click")
	}
}

func TestGenericEventTargetSignalOnceAndDispatchState(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var target = new EventTarget();
		var controller = new AbortController();
		var calls = 0;
		var seenTarget = false;
		var handler = function(event) {
			calls++;
			seenTarget = event.target === target && event.currentTarget === target && event.eventPhase === 2;
		};
		target.addEventListener('x', handler, { once: true, signal: controller.signal });
		target.dispatchEvent(new Event('x'));
		target.dispatchEvent(new Event('x'));
		controller.abort();
		var nullSignalThrows = false;
		try { target.addEventListener('y', null, { signal: null }); } catch (e) { nullSignalThrows = e instanceof TypeError; }
	`, "generic-event-target"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("calls"); !ok || value.Num != 1 {
		t.Errorf("calls = %+v, want 1", value)
	}
	for _, name := range []string{"seenTarget", "nullSignalThrows"} {
		if value, ok := r.Global(name); !ok || !value.Bool {
			t.Errorf("%s = %+v, want true", name, value)
		}
	}
}

func TestLegacyWindowEventReplacementAndNestedDispatch(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var initialWindowEventUndefined = window.event === undefined;
		window.event = 'direct';
		var windowEventSetterWorks = window.event === 'direct';
		window.event = undefined;
		var target = new EventTarget();
		var outer = new Event('outer');
		var inner = new Event('inner');
		var seenOuter, seenInner, argOuter, argInner, restoredOuter, restoredUndefined, authorValuePreserved, authorValueInHandler = false;
		target.addEventListener('outer', function(event) {
			if (event === outer) {
				seenOuter = window.event === outer;
				argOuter = event === outer;
				target.dispatchEvent(inner);
				restoredOuter = window.event === outer;
			} else {
				authorValueInHandler = window.event === 'author value';
			}
		});
		target.addEventListener('inner', function(event) {
			seenInner = window.event === inner;
			argInner = event === inner;
		});
		target.dispatchEvent(outer);
		restoredUndefined = window.event === undefined;
		window.event = 'author value';
		target.dispatchEvent(new Event('outer'));
		authorValuePreserved = window.event === 'author value';
	`, "legacy-window-event"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"seenOuter", "seenInner", "argOuter", "argInner", "restoredOuter", "restoredUndefined", "authorValuePreserved", "authorValueInHandler"} {
		if value, ok := r.Global(name); !ok || !value.Bool {
			t.Errorf("%s = %+v, want true", name, value)
		}
	}
	for _, name := range []string{"initialWindowEventUndefined", "windowEventSetterWorks"} {
		if value, ok := r.Global(name); !ok || !value.Bool {
			t.Errorf("%s = %+v, want true", name, value)
		}
	}
}

func TestTemplateClonedInputClickRunsActivationHandler(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var template = document.createElement('template');
		template.innerHTML = '<input type="checkbox" oninput="window.cloneInputActivated = this.checked">';
		var parent = document.createElement('input');
		parent.type = 'radio';
		document.body.appendChild(parent);
		var clone = template.content.firstElementChild.cloneNode(true);
		parent.appendChild(clone);
		clone.click();
		var cloneInputChecked = clone.checked;
	`, "template-clone-click-activation"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("cloneInputChecked"); !ok || !value.Bool {
		t.Errorf("cloneInputChecked = %+v, want true", value)
	}
	if value, ok := r.Global("cloneInputActivated"); !ok || !value.Bool {
		t.Errorf("cloneInputActivated = %+v, want true", value)
	}
}

func TestDetachedCheckableClickDoesNotFireInputOrChange(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var events = [];
		var checkbox = document.createElement('input');
		checkbox.type = 'checkbox';
		checkbox.addEventListener('input', function() { events.push('checkbox-input'); });
		checkbox.addEventListener('change', function() { events.push('checkbox-change'); });
		checkbox.click();
		var radio = document.createElement('input');
		radio.type = 'radio';
		radio.addEventListener('input', function() { events.push('radio-input'); });
		radio.addEventListener('change', function() { events.push('radio-change'); });
		radio.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
		var detachedEvents = events.join(',');
		var connected = document.createElement('input');
		connected.type = 'checkbox';
		connected.addEventListener('input', function() { events.push('connected-input'); });
		document.body.appendChild(connected);
		connected.click();
		var allEvents = events.join(',');
	`, "detached-checkable-click"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("detachedEvents"); !ok || value.Str != "" {
		t.Errorf("detached events = %+v, want none", value)
	}
	if value, ok := r.Global("allEvents"); !ok || value.Str != "connected-input" {
		t.Errorf("all events = %+v, want connected-input", value)
	}
}

func TestGenericEventTargetPassiveListenerCancellation(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var target = new EventTarget();
		var passivePrevented, activePrevented, passiveReturnValue = false;
		target.addEventListener('passive', function(event) {
			event.preventDefault();
			passivePrevented = event.defaultPrevented;
			event.returnValue = false;
			passiveReturnValue = event.defaultPrevented;
		}, { passive: true });
		target.addEventListener('active', function(event) {
			event.returnValue = false;
			activePrevented = event.defaultPrevented;
		});
		var passiveResult = target.dispatchEvent(new Event('passive', { cancelable: true }));
		var activeResult = target.dispatchEvent(new Event('active', { cancelable: true }));
	`, "generic-passive-listener"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		"passivePrevented":   false,
		"passiveReturnValue": false,
		"activePrevented":    true,
		"passiveResult":      true,
		"activeResult":       false,
	} {
		if value, ok := r.Global(name); !ok || value.Bool != want {
			t.Errorf("%s = %+v, want %t", name, value, want)
		}
	}
}

func TestNodeEventTargetPassiveListenerCancellation(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var target = document.getElementById('target');
		var prevented = true;
		target.addEventListener('passive', function(event) {
			event.preventDefault();
			prevented = event.defaultPrevented;
			event.returnValue = false;
		}, { passive: true });
		var result = target.dispatchEvent(new Event('passive', { cancelable: true }));
	`, "node-passive-listener"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("prevented"); !ok || value.Bool {
		t.Errorf("prevented = %+v, want false", value)
	}
	if value, ok := r.Global("result"); !ok || !value.Bool {
		t.Errorf("dispatch result = %+v, want true", value)
	}
}

func TestWindowEventTargetPassiveListenerCancellation(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var prevented = true;
		window.addEventListener('passive', function(event) {
			event.preventDefault();
			prevented = event.defaultPrevented;
			event.returnValue = false;
		}, { passive: true });
		var result = window.dispatchEvent(new Event('passive', { cancelable: true }));
	`, "window-passive-listener"); err != nil {
		t.Fatal(err)
	}
	if value, ok := r.Global("prevented"); !ok || value.Bool {
		t.Errorf("prevented = %+v, want false", value)
	}
	if value, ok := r.Global("result"); !ok || !value.Bool {
		t.Errorf("dispatch result = %+v, want true", value)
	}
}

func TestWPTTestDriverClickDispatchesTrustedMouseSequence(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	r.EnableTestUserActivationHook()
	if err := r.Run(`
		var target = document.createElement('button');
		document.body.appendChild(target);
		var mouseupEvent;
		var mouseupTrusted = false;
		var clickTrusted = false;
		var mouseupReentryThrows = false;
		var clickReentryThrows = false;
		var redispatchedMouseupTrusted = true;
		var redispatchedMouseupType = '';
		var onClickFired = false;
		target.addEventListener('mouseup', function(event) {
			mouseupEvent = event;
			mouseupTrusted = event.isTrusted;
			try { target.dispatchEvent(event); } catch (error) { mouseupReentryThrows = error.name === 'InvalidStateError'; }
		}, { once: true });
		target.addEventListener('click', function(event) {
			clickTrusted = event.isTrusted;
			try { target.dispatchEvent(event); } catch (error) { clickReentryThrows = error.name === 'InvalidStateError'; }
			target.addEventListener('mouseup', function(redispatched) {
				redispatchedMouseupTrusted = redispatched.isTrusted;
				redispatchedMouseupType = redispatched.type;
			}, { once: true });
			var onClick = function() { onClickFired = true; };
			target.addEventListener('click', onClick);
			target.dispatchEvent(mouseupEvent);
			target.removeEventListener('click', onClick);
		}, { once: true });
		var driverPromiseResolved = false;
		var test_driver = { click: function(element) {
			return Promise.resolve().then(function() {
				__goosieGrantUserActivation();
				__goosieTestDriverClick(element);
			});
		} };
		new test_driver.click(target).then(function() { driverPromiseResolved = true; });
	`, "testdriver-click"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		"mouseupTrusted":             true,
		"clickTrusted":               true,
		"mouseupReentryThrows":       true,
		"clickReentryThrows":         true,
		"redispatchedMouseupTrusted": false,
		"onClickFired":               false,
		"driverPromiseResolved":      true,
	} {
		if value, ok := r.Global(name); !ok || value.Bool != want {
			t.Errorf("%s = %+v, want %t", name, value, want)
		}
	}
	if value, ok := r.Global("redispatchedMouseupType"); !ok || value.Str != "mouseup" {
		t.Errorf("redispatchedMouseupType = %+v, want mouseup", value)
	}
}

func TestEventCancelBubbleTracksDispatchAndResets(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var target = document.getElementById('target');
		var parent = target.parentNode;
		var targetCalls = 0;
		var parentCalls = 0;
		target.addEventListener('bubble-check', function(event) {
			targetCalls++;
			event.cancelBubble = true;
			event.cancelBubble = false;
		});
		parent.addEventListener('bubble-check', function() { parentCalls++; });
		var preStopped = new Event('bubble-check', {bubbles: true});
		preStopped.cancelBubble = true;
		target.dispatchEvent(preStopped);
		var methodStopped = new Event('bubble-check', {bubbles: true});
		methodStopped.stopPropagation();
		target.dispatchEvent(methodStopped);
		var event = new Event('bubble-check', {bubbles: true});
		target.dispatchEvent(event);
		window.cancelBubbleResults = [targetCalls, parentCalls, preStopped.cancelBubble,
			methodStopped.cancelBubble, event.cancelBubble, event.target === target,
			event.srcElement === target].join('|');
	`, "cancel-bubble"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Evaluate(`window.cancelBubbleResults`); err != nil || got != "1|0|false|false|false|true|true" {
		t.Fatalf("cancelBubble result = %q, err %v", got, err)
	}
}

func TestInputEventConstructorAndDispatchPayload(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var target = document.getElementById('target');
		target.addEventListener('beforeinput', function(e) {
			window.inputEventFields = [e instanceof InputEvent, e.data, e.inputType, e.isComposing, e.bubbles, e.cancelable].join('|');
		});
		var event = new InputEvent('beforeinput', {data: 'é', inputType: 'insertText', isComposing: true, bubbles: true, cancelable: true});
		var transfer = new DataTransfer();
		var withTransfer = new InputEvent('x', {dataTransfer: transfer});
		var dataTransferDescriptor = Object.getOwnPropertyDescriptor(InputEvent.prototype, 'dataTransfer');
		var dataTransferIDL = dataTransferDescriptor && typeof dataTransferDescriptor.get === 'function' &&
			dataTransferDescriptor.set === undefined && dataTransferDescriptor.enumerable &&
			dataTransferDescriptor.configurable && !Object.prototype.hasOwnProperty.call(withTransfer, 'dataTransfer');
		var rejectsInvalidTransfer = false;
		try { new InputEvent('x', {dataTransfer: {}}); } catch (error) { rejectsInvalidTransfer = error instanceof TypeError; }
		window.inputEventDefaults = [event instanceof InputEvent, event instanceof Event, event.isTrusted === false, new InputEvent('x').data === null, new InputEvent('x').inputType === '', new InputEvent('x').isComposing === false, new InputEvent('x').dataTransfer === null, withTransfer.dataTransfer === transfer, dataTransferIDL, rejectsInvalidTransfer, new InputEvent('x', {data: 42, inputType: 17}).data === '42', new InputEvent('x', {data: 42, inputType: 17}).inputType === '17'].join('|');
		target.dispatchEvent(event);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Evaluate(`window.inputEventDefaults+';'+window.inputEventFields`); err != nil || got != "true|true|true|true|true|true|true|true|true|true|true|true;true|é|insertText|true|true|true" {
		t.Fatalf("InputEvent fields = %q, err %v", got, err)
	}
}

func TestClipboardEventConstructorAndIDLAttribute(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var transfer = new DataTransfer();
		var event = new ClipboardEvent('paste', {clipboardData: transfer, bubbles: true, cancelable: true});
		var descriptor = Object.getOwnPropertyDescriptor(ClipboardEvent.prototype, 'clipboardData');
		var invalidTransferRejected = false;
		try { new ClipboardEvent('paste', {clipboardData: {}}); } catch (error) { invalidTransferRejected = error instanceof TypeError; }
		window.clipboardEventContract = [window.ClipboardEvent === ClipboardEvent, event instanceof ClipboardEvent, event instanceof Event,
			event.clipboardData === transfer, new ClipboardEvent('paste').clipboardData === null,
			descriptor && typeof descriptor.get === 'function' && descriptor.set === undefined &&
			descriptor.enumerable && descriptor.configurable &&
			!Object.prototype.hasOwnProperty.call(event, 'clipboardData'), invalidTransferRejected,
			event.bubbles, event.cancelable].join('|');
	`, "clipboard-event"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Evaluate(`window.clipboardEventContract`); err != nil || got != "true|true|true|true|true|true|true|true|true" {
		t.Fatalf("ClipboardEvent contract = %q, err %v", got, err)
	}
}

func TestCompositionEventConstructorAndDispatchPayload(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var target = document.getElementById('target');
		target.addEventListener('compositionupdate', function(e) {
			window.compositionFields = [e instanceof CompositionEvent, e instanceof Event, e.data, e.isTrusted].join('|');
		});
		var event = new CompositionEvent('compositionupdate', {data: 'かな', bubbles: true});
		window.compositionDefaults = [event.data, event.isTrusted === false, new CompositionEvent('x').data === ''].join('|');
		target.dispatchEvent(event);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Evaluate(`window.compositionDefaults+';'+window.compositionFields`); err != nil || got != "かな|true|true;true|true|かな|false" {
		t.Fatalf("CompositionEvent fields = %q, err %v", got, err)
	}
}

func TestInlineEventHandlerContentAttribute(t *testing.T) {
	doc := eventDOM()
	div := doc.ElementByID("target")
	div.SetAttribute("onclick", "window.inlineOrder.push('inline'); return false")
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		window.inlineOrder = ['before'];
		var target = document.getElementById('target');
		target.addEventListener('click', function() { window.inlineOrder.push('listener'); });
		window.inlineHandlerType = typeof target.onclick;
		window.inlineOrderText = function() { return window.inlineOrder.join(','); };
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	event := dom.NewEvent("click", true, true)
	allowed := dom.DispatchEvent(div, event)
	if allowed {
		t.Fatal("inline handler return false did not cancel the event")
	}
	if got, ok := r.Global("inlineHandlerType"); !ok || got.Str != "function" {
		t.Fatalf("typeof target.onclick = %v, want function", got)
	}
	if err := r.Run(`window.inlineOrderResult = inlineOrderText();`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inlineOrderResult"); !ok || got.Str != "before,inline,listener" {
		t.Fatalf("inlineOrder = %v, want before,inline,listener", got)
	}

	if err := r.Run(`target.removeAttribute('onclick'); target.setAttribute('onclick', 'window.inlineOrder.push(\"again\")');`, "inline"); err != nil {
		t.Fatal(err)
	}
	dom.DispatchEvent(div, dom.NewEvent("click", true, true))
	if err := r.Run(`window.inlineOrderResult = inlineOrderText();`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inlineOrderResult"); !ok || got.Str != "before,inline,listener,listener,again" {
		t.Fatalf("after remove/re-add inlineOrder = %v", got)
	}
}

func TestInlineEventHandlerInvalidSourceIsInert(t *testing.T) {
	doc := eventDOM()
	doc.ElementByID("target").SetAttribute("onclick", "this is not valid javascript !!!")
	r := newEventRuntime(t, doc)
	if err := r.Run(`window.invalidListenerRan = false;`, "inline"); err != nil {
		t.Fatal(err)
	}
	dom.DispatchEvent(doc.ElementByID("target"), dom.NewEvent("click", true, true))
	if got, ok := r.Global("invalidListenerRan"); !ok || got.Bool {
		t.Fatalf("invalid handler ran: %v", got)
	}
}

func TestEventHandlerIDLAttributeReturnFalseCancels(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var target = document.getElementById('target');
		target.onclick = function() { return false; };
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(doc.ElementByID("target"), event)
	if !event.DefaultPrevented() {
		t.Fatal("IDL event handler return false did not cancel the event")
	}
}

func TestDispatchEventReflectsDefaultPreventedToOriginalEvent(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var target = document.getElementById('target');
		target.onclick = function() { return false; };
		var event = new Event('click', {cancelable: true});
		target.dispatchEvent(event);
		window.eventDefaultPrevented = event.defaultPrevented;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("eventDefaultPrevented"); !ok || !got.Bool {
		t.Fatalf("event.defaultPrevented = %+v, want true after dispatch", got)
	}
}

func TestBodyEventHandlerIDLAttributesReflectToWindow(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		window.onload = function() {};
		var alternateBody = document.createElement('body');
		var replacement = function() {};
		alternateBody.onload = replacement;
		window.bodyHandlerReflected = window.onload === replacement && alternateBody.onload === replacement;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("bodyHandlerReflected"); !ok || !got.Bool {
		t.Fatalf("body.onload did not reflect to Window: %+v", got)
	}
}

func TestWindowlessBodyEventHandlerIDLAttributesAreElementLocal(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var parsed = new DOMParser().parseFromString('<p>parsed</p>', 'text/html');
		var body = parsed.body;
		var windowHandler = function() {};
		var bodyHandler = function() {};
		window.onload = windowHandler;
		var initiallyNull = body.onload === null;
		body.onload = bodyHandler;
		var bodyOnloadStillNull = body.onload === null;
		var windowOnloadUntouched = window.onload === windowHandler;
		body.onclick = bodyHandler;
		var onclickIsLocal = body.onclick === bodyHandler;
		windowlessBodyHandlersCorrect = initiallyNull && bodyOnloadStillNull && windowOnloadUntouched && onclickIsLocal;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("windowlessBodyHandlersCorrect"); !ok || !got.Bool {
		t.Fatalf("windowless body event handler behavior = %+v, want null ignored shadowed handler and local onclick", got)
	}
}

func TestBodyEventHandlerContentAttributesReflectToWindow(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var alternateBody = document.createElement('body');
		alternateBody.setAttribute('onblur', 'window.bodyContentHandlerRan = true');
		window.dispatchEvent(new Event('blur'));
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("bodyContentHandlerRan"); !ok || !got.Bool {
		t.Fatalf("body content handler did not run on Window: %+v", got)
	}
}

func TestWindowDispatchEventPreservesCancelableState(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		window.onblur = function() { return false; };
		var event = new Event('blur', {cancelable: true});
		window.dispatchEvent(event);
		window.windowEventDefaultPrevented = event.defaultPrevented;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("windowEventDefaultPrevented"); !ok || !got.Bool {
		t.Fatalf("window-dispatched event.defaultPrevented = %+v, want true", got)
	}
}

func TestWindowOnErrorReceivesErrorEventArguments(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var errorValue = {sentinel: true};
		var received;
		window.onerror = function(message, source, line, column, error) {
			received = [arguments.length, message, source, line, column, error === errorValue, this === window];
			return true;
		};
		var event = new ErrorEvent('error', {
			cancelable: true,
			message: 'failure',
			filename: 'script.js',
			lineno: 12,
			colno: 8,
			error: errorValue
		});
		window.dispatchEvent(event);
		window.errorEventResult = received.join('|') + '|' + event.defaultPrevented;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("errorEventResult"); !ok || got.Str != "5|failure|script.js|12|8|true|true|true" {
		t.Fatalf("Window.onerror arguments/state = %+v", got)
	}
}

func TestElementOnErrorReceivesErrorEventObject(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var target = document.getElementById('target');
		target.onerror = function(event) {
			window.elementErrorResult = arguments.length === 1 && event instanceof ErrorEvent && event.message === 'element failure';
		};
		target.dispatchEvent(new ErrorEvent('error', {message: 'element failure'}));
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("elementErrorResult"); !ok || !got.Bool {
		t.Fatalf("element onerror did not receive one ErrorEvent argument: %+v", got)
	}
}

func TestEventListenerObjectExceptionReportsToListenerObjectRealm(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const listenerFrame = document.createElement('iframe');
		const methodFrame = document.createElement('iframe');
		const targetFrame = document.createElement('iframe');
		listenerFrame.srcdoc = '';
		methodFrame.srcdoc = '';
		targetFrame.srcdoc = '';
		document.body.append(listenerFrame, methodFrame, targetFrame);
		const listenerWindow = listenerFrame.contentWindow;
		const methodWindow = methodFrame.contentWindow;
		const targetWindow = targetFrame.contentWindow;
		new listenerWindow.Function('listener = {};')();
		listenerWindow.listener.handleEvent = new methodWindow.Function('throw new Error("listener failure")');
		const target = new targetWindow.EventTarget();
		let listenerRealmSawError = false;
		let listenerRealmTarget = false;
		let listenerRealmErrorType = false;
		let methodRealmSawError = false;
		let targetRealmSawError = false;
		listenerWindow.addEventListener('error', event => {
			listenerRealmSawError = true;
			listenerRealmTarget = event.target === listenerWindow;
			listenerRealmErrorType = event.error instanceof methodWindow.Error;
			event.preventDefault();
		});
		methodWindow.addEventListener('error', () => { methodRealmSawError = true; });
		targetWindow.addEventListener('error', () => { targetRealmSawError = true; });
		target.addEventListener('party', listenerWindow.listener);
		target.dispatchEvent(new Event('party'));
		window.crossGlobalObjectListenerResult = [listenerRealmSawError, listenerRealmTarget, listenerRealmErrorType, methodRealmSawError, targetRealmSawError].join('|');
	`, "cross-global-event-listener-error.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("crossGlobalObjectListenerResult"); !ok || got.Str != "true|true|true|false|false" {
		t.Fatalf("cross-global object listener exception routing = %+v, want listener target only", got)
	}
}

func TestGenericEventTargetSkipsObjectListenerFromRemovedIframe(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		window.inactiveObjectListenerCalls = 0;
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.append(frame);
		new frame.contentWindow.Function('parent.callbackObject = { handleEvent() { parent.inactiveObjectListenerCalls++; } }')();
		const callbackObject = window.callbackObject;
		const target = new EventTarget();
		target.addEventListener('inactive', callbackObject);
		frame.remove();
		target.dispatchEvent(new Event('inactive'));
		window.inactiveObjectListenerResult = window.inactiveObjectListenerCalls;
	`, "inactive-object-event-listener.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inactiveObjectListenerResult"); !ok || got.Num != 0 {
		t.Fatalf("inactive iframe object listener calls = %+v, want 0", got)
	}
}

func TestGenericEventTargetFromRemovedIframeStillDispatchesWithoutCallbacks(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.append(frame);
		new frame.contentWindow.Function('parent.target = new EventTarget()')();
		const target = window.target;
		let calls = 0;
		target.addEventListener('inactive', () => calls++);
		frame.remove();
		const event = new Event('inactive', {cancelable: true});
		const returned = target.dispatchEvent(event);
		window.inactiveTargetDispatchResult = [calls, returned, event.target === target, event.currentTarget === null, event.eventPhase === Event.NONE, event.defaultPrevented].join('|');
	`, "inactive-event-target-dispatch.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inactiveTargetDispatchResult"); !ok || got.Str != "0|true|true|true|true|false" {
		t.Fatalf("inactive EventTarget dispatch state = %+v, want callbacks skipped and dispatch state retained", got)
	}
}

func TestAbortSignalFromRemovedIframeKeepsLiveAbortedState(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.append(frame);
		new frame.contentWindow.Function('parent.controller = new AbortController()')();
		const controller = window.controller;
		frame.remove();
		controller.abort('removed-frame abort');
		window.removedFrameAbortState = [controller.signal.aborted, controller.signal.reason].join('|');
	`, "removed-iframe-abort-state.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("removedFrameAbortState"); !ok || got.Str != "true|removed-frame abort" {
		t.Fatalf("removed iframe AbortSignal state = %+v, want true|removed-frame abort", got)
	}
}

func TestNodeDispatchStopsPropagationWhenCaptureRemovesIframe(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.append(frame);
		const childDoc = frame.contentDocument;
		const target = childDoc.createElement('div');
		childDoc.body.append(target);
		window.inactivePathOrder = [];
		childDoc.addEventListener('inactive-path', () => {
			inactivePathOrder.push('capture');
			frame.remove();
		}, true);
		target.addEventListener('inactive-path', () => inactivePathOrder.push('target'));
		childDoc.addEventListener('inactive-path', () => inactivePathOrder.push('bubble'));
		target.dispatchEvent(new Event('inactive-path', {bubbles: true}));
		window.inactivePathOrder = inactivePathOrder.join(',');
	`, "inactive-event-path.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inactivePathOrder"); !ok || got.Str != "capture" {
		t.Fatalf("inactive event path order = %+v, want capture only", got)
	}
}

func TestIframeWindowDispatchEventSkipsListenersWhenRemoved(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.append(frame);
		const childWindow = frame.contentWindow;
		window.inactiveWindowCalls = 0;
		childWindow.addEventListener('inactive-window', () => inactiveWindowCalls++);
		frame.remove();
		window.inactiveWindowDispatchResult = [
			childWindow.dispatchEvent(new Event('inactive-window')),
			inactiveWindowCalls
		].join('|');
	`, "inactive-iframe-window-event.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inactiveWindowDispatchResult"); !ok || got.Str != "true|0" {
		t.Fatalf("removed iframe Window dispatch = %+v, want true|0", got)
	}
}

func TestIframeContentEventHandlerCompilesBeforeRemovalAndSkipsAfter(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.append(frame);
		const childDoc = frame.contentDocument;
		const target = childDoc.createElement('div');
		childDoc.body.append(target);
		target.setAttribute('onclick', 'top.inactiveContentHandlerCalls = (top.inactiveContentHandlerCalls || 0) + 1;');
		window.inactiveContentHandlerType = typeof target.onclick;
		frame.remove();
		target.dispatchEvent(new Event('click'));
	`, "inactive-iframe-content-handler.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("inactiveContentHandlerType"); !ok || got.Str != "function" {
		t.Fatalf("iframe content handler type = %+v, want function", got)
	}
	if got, ok := r.Global("inactiveContentHandlerCalls"); ok && got.Num != 0 {
		t.Fatalf("removed iframe content handler calls = %+v, want zero", got)
	}
}

func TestDefaultPassiveListenersOnGlobalEventTargets(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		const targets = [window, document, document.documentElement, document.body];
		const eventTypes = ['touchstart', 'touchmove', 'wheel', 'mousewheel'];
		const results = [];
		for (const target of targets.concat(document.createElement('div'))) {
			for (const type of eventTypes) {
				for (const options of [undefined, {passive: undefined}, {passive: false}]) {
					const handler = event => event.preventDefault();
					target.addEventListener(type, handler, options);
					results.push(target.dispatchEvent(new Event(type, {cancelable: true})));
					target.removeEventListener(type, handler, options);
				}
			}
		}
		window.defaultPassiveListenerResult = results.map(Boolean).join(',');
	`, "default-passive-event-listeners.js"); err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, 60)
	for target := 0; target < 5; target++ {
		for typ := 0; typ < 4; typ++ {
			if target < 4 {
				want = append(want, "true", "true", "false")
			} else {
				want = append(want, "false", "false", "false")
			}
		}
	}
	if got, ok := r.Global("defaultPassiveListenerResult"); !ok || got.Str != strings.Join(want, ",") {
		t.Fatalf("default passive listener dispatch results = %+v, want %s", got, strings.Join(want, ","))
	}
}

func TestInnerHTMLActivatesInlineEventHandlerAttributes(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		window.innerHTMLHandlerCalls = 0;
		var target = document.getElementById('target');
		target.innerHTML = '<button id="inner-handler" onclick="window.innerHTMLHandlerCalls++"></button>';
		document.getElementById('inner-handler').dispatchEvent(new Event('click'));
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("innerHTMLHandlerCalls"); !ok || got.Num != 1 {
		t.Fatalf("innerHTML-created event handler calls = %+v, want 1", got)
	}
}

func TestWindowPostMessageQueuesClonedMessageEvent(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		var messageSource = {nested: {value: 1}};
		window.onmessage = function(event) {
			window.postMessageResult = [
				event instanceof MessageEvent,
				event.data.nested.value,
				event.origin,
				event.source === window,
				event.currentTarget === window,
				event.isTrusted
			];
		};
		window.postMessage(messageSource, '*');
		messageSource.nested.value = 2;
		window.beforePostMessageTask = typeof window.postMessageResult;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("beforePostMessageTask"); !ok || got.Str != "undefined" {
		t.Fatalf("postMessage ran synchronously: %+v", got)
	}
	if !r.Tick() {
		t.Fatal("postMessage task was not queued")
	}
	if err := r.Run(`window.postMessageResultText = postMessageResult.join('|');`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("postMessageResultText"); !ok || got.Str != "true|1|https://example.test|true|true|true" {
		t.Fatalf("postMessage event fields = %+v", got)
	}
}

func TestWindowPostMessageHonorsTargetOrigin(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)
	if err := r.Run(`
		window.messageCount = 0;
		window.addEventListener('message', function() { window.messageCount++; });
		window.postMessage('drop', 'https://other.test');
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if err := r.Run(`window.messageCountAfterTargetCheck = messageCount;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("messageCountAfterTargetCheck"); !ok || got.Num != 0 {
		t.Fatalf("postMessage ignored targetOrigin: %+v", got)
	}
}

func TestWindowEventListenerOptionsOnceAndCapture(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		window.listenerOrder = [];
		window.addEventListener('probe', function() { listenerOrder.push('capture'); }, true);
		window.addEventListener('probe', function() { listenerOrder.push('once'); }, {once: true});
		document.body.dispatchEvent(new Event('probe', {bubbles: true}));
		document.body.dispatchEvent(new Event('probe', {bubbles: true}));
		window.listenerOrderText = listenerOrder.join(',');
		window.detachedWindowCalls = 0;
		window.addEventListener('detached-probe', function() { detachedWindowCalls++; });
		document.createElement('div').dispatchEvent(new Event('detached-probe', {bubbles: true}));
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("listenerOrderText"); !ok || got.Str != "capture,once,capture" {
		t.Fatalf("window event listener option order = %+v (present %v), want capture,once,capture", got, ok)
	}
	if got, ok := r.Global("detachedWindowCalls"); !ok || got.Num != 0 {
		t.Fatalf("detached event reached window = %+v (present %v), want zero calls", got, ok)
	}
}

func TestWindowEventListenerOptionsAbortSignal(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		window.abortedSignalCalls = 0;
		var controller = new AbortController();
		window.addEventListener('abort-probe', function() { abortedSignalCalls++; }, {signal: controller.signal});
		controller.abort();
		window.dispatchEvent(new Event('abort-probe'));

		window.removedSignalCalls = 0;
		var secondController = new AbortController();
		var listener = function() { removedSignalCalls++; };
		window.addEventListener('remove-probe', listener, {signal: secondController.signal});
		window.removeEventListener('remove-probe', listener);
		window.addEventListener('remove-probe', listener);
		secondController.abort();
		window.dispatchEvent(new Event('remove-probe'));

		var alreadyAborted = AbortSignal.abort();
		window.addEventListener('already-aborted-probe', function() { abortedSignalCalls++; }, {signal: alreadyAborted});
		window.dispatchEvent(new Event('already-aborted-probe'));
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("abortedSignalCalls"); !ok || got.Num != 0 {
		t.Fatalf("aborted signal listener count = %+v (present %v), want 0", got, ok)
	}
	if got, ok := r.Global("removedSignalCalls"); !ok || got.Num != 1 {
		t.Fatalf("manual removal/re-add listener count = %+v (present %v), want 1", got, ok)
	}
}

func TestSrcdocWindowPostMessageReachesParentWindow(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		window.childMessageResult = 'pending';
		window.childReplyCount = 0;
		window.addEventListener('message', function(event) {
			window.childMessageSeen = event.data;
			if (event.data === 'from-child') {
				frame.contentWindow.postMessage('wrong-origin-drop', 'https://other.test');
				frame.contentWindow.postMessage('from-parent', '*');
			} else {
				window.childReplyCount++;
				window.childMessageResult = [event.data, event.origin,
					event.source === frame.contentWindow,
					event.currentTarget === window].join('|');
			}
		});
		var frame = document.createElement('iframe');
		frame.contentWindow.addEventListener('message', function(event) {
			if (event.data === 'self-message') {
				window.childWindowMessage = [event.data, event.source === frame.contentWindow,
					event.currentTarget === frame.contentWindow].join('|');
			}
		});
		frame.srcdoc = '<script>window.addEventListener("message", function(event) { if (event.data === "from-parent" || event.data === "wrong-origin-drop") parent.postMessage("reply:" + event.data + ":" + (event.source === parent), "*"); }); parent.postMessage("from-child", "*"); window.postMessage("self-message", "*")</script>';
		document.body.appendChild(frame);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("childMessageResult"); !ok || got.Str != "pending" {
		t.Fatalf("cross-window postMessage dispatched synchronously: %+v", got)
	}
	// Pump the parser/load task and queued message tasks.
	for i := 0; i < 5; i++ {
		r.Tick()
	}
	if got, ok := r.Global("childMessageResult"); !ok || got.Str != "reply:from-parent:true|https://example.test|true|true" {
		seen, _ := r.Global("childMessageSeen")
		t.Fatalf("cross-window message result = %+v (seen: %+v)", got, seen)
	}
	if got, ok := r.Global("childReplyCount"); !ok || got.Num != 1 {
		t.Fatalf("cross-window targetOrigin filtering reply count = %+v, want 1", got)
	}
	if got, ok := r.Global("childWindowMessage"); !ok || got.Str != "self-message|true|true" {
		t.Fatalf("contentWindow message listener result = %+v", got)
	}
}

// TestEventBubblesInJS verifies that an event dispatched on a child element
// bubbles up to the parent's listener.
func TestEventBubblesInJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var bubbled = false;
		var body = document.body;
		body.addEventListener('click', function(e) {
			bubbled = true;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	// Dispatch on the child div; the body listener should fire via bubbling.
	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("bubbled"); !ok || !v.Bool {
		t.Errorf("bubbled = %+v, want true", v)
	}
}

// TestStopPropagationFromJS verifies that event.stopPropagation() prevents
// the event from reaching ancestor listeners.
func TestStopPropagationFromJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var bodyFired = false;
		var body = document.body;
		body.addEventListener('click', function(e) {
			bodyFired = true;
		});
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			e.stopPropagation();
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("bodyFired"); !ok || v.Bool {
		t.Errorf("bodyFired = %+v, want false (stopPropagation should prevent bubbling)", v)
	}
}

// TestPreventDefaultFromJS verifies that event.preventDefault() marks the
// event as default-prevented.
func TestPreventDefaultFromJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var wasDefaultPrevented = false;
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			e.preventDefault();
			wasDefaultPrevented = e.defaultPrevented;
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	result := dom.DispatchEvent(div, event)

	if v, ok := r.Global("wasDefaultPrevented"); !ok || !v.Bool {
		t.Errorf("wasDefaultPrevented = %+v, want true", v)
	}
	// DispatchEvent returns false when the event was cancelled.
	if result {
		t.Error("DispatchEvent returned true, want false (event was cancelled)")
	}
}

// TestEventTargetIsSet verifies that event.target is the element the listener
// is registered on (for at-target dispatch).
func TestEventTargetIsSet(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var gotTargetID = '';
		var el = document.getElementById('target');
		el.addEventListener('click', function(e) {
			gotTargetID = e.target.getAttribute('id');
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("gotTargetID"); !ok || v.Str != "target" {
		t.Errorf("gotTargetID = %+v, want %q", v, "target")
	}
}

// TestDispatchEventFromJS verifies that element.dispatchEvent(event) from JS
// fires the element's own listeners.
func TestDispatchEventFromJS(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var jsDispatched = false;
		var el = document.getElementById('target');
		el.addEventListener('custom', function(e) {
			jsDispatched = true;
		});
		var evt = { type: 'custom' };
		el.dispatchEvent(evt);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	if v, ok := r.Global("jsDispatched"); !ok || !v.Bool {
		t.Errorf("jsDispatched = %+v, want true", v)
	}
}

// TestRemoveEventListenerBasic verifies that removeEventListener with a named
// function reference prevents the listener from firing.
func TestRemoveEventListenerBasic(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var fired = false;
		var el = document.getElementById('target');
		function handler(e) { fired = true; }
		el.addEventListener('click', handler);
		el.removeEventListener('click', handler);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("fired"); ok && v.Bool {
		t.Error("listener should have been removed and not fire")
	}
}

// TestRemoveEventListenerCaptureMismatch verifies that removing with a
// different capture flag than was used to add does NOT remove the listener.
func TestRemoveEventListenerCaptureMismatch(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var fired = false;
		var el = document.getElementById('target');
		function handler(e) { fired = true; }
		el.addEventListener('click', handler, true);
		el.removeEventListener('click', handler, false);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("fired"); !ok || !v.Bool {
		t.Error("listener should still fire: capture flag mismatch means not removed")
	}
}

// TestRemoveEventListenerPreservesOthers verifies that removing one listener
// does not affect other listeners on the same element.
func TestRemoveEventListenerPreservesOthers(t *testing.T) {
	doc := eventDOM()
	r := newEventRuntime(t, doc)

	if err := r.Run(`
		var count = 0;
		var el = document.getElementById('target');
		function handler1(e) { count += 1; }
		function handler2(e) { count += 10; }
		el.addEventListener('click', handler1);
		el.addEventListener('click', handler2);
		el.removeEventListener('click', handler1);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	div := doc.ElementByID("target")
	event := dom.NewEvent("click", true, true)
	dom.DispatchEvent(div, event)

	if v, ok := r.Global("count"); !ok || int64(v.Num) != 10 {
		t.Errorf("count = %+v, want 10 (only handler2 should fire)", v)
	}
}

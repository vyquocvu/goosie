package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// TestBroadcastChannelConstructor verifies the constructor exists on both the
// global scope and window, and that the name property is read-only.
func TestBroadcastChannelConstructor(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var exists = typeof BroadcastChannel === 'function';
		var ch = new BroadcastChannel('test');
		var name = ch.name;
		var onWin = typeof window.BroadcastChannel === 'function';
		var nameWritable = Reflect.set(ch, 'name', 'changed');
		var nameAfterAssignment = ch.name;
		var hasOwnName = Object.prototype.hasOwnProperty.call(ch, 'name');
		var isEventTarget = ch instanceof EventTarget;
		var prototypeInheritsEventTarget = Object.getPrototypeOf(BroadcastChannel.prototype) === EventTarget.prototype;
		var eventTargetOperationsInherited = ch.addEventListener === EventTarget.prototype.addEventListener && ch.removeEventListener === EventTarget.prototype.removeEventListener && ch.dispatchEvent === EventTarget.prototype.dispatchEvent;
		var eventTargetOperationsNotOwn = !Object.prototype.hasOwnProperty.call(ch, 'addEventListener') && !Object.prototype.hasOwnProperty.call(ch, 'removeEventListener') && !Object.prototype.hasOwnProperty.call(ch, 'dispatchEvent');
		var eventTargetOperationsNonEnumerable = ['addEventListener', 'removeEventListener', 'dispatchEvent'].every(function(name) { return !Object.getOwnPropertyDescriptor(EventTarget.prototype, name).enumerable; });
		var onmessageIsInherited = !Object.prototype.hasOwnProperty.call(ch, 'onmessage') && Object.getOwnPropertyDescriptor(BroadcastChannel.prototype, 'onmessage').enumerable;
		var prototypeNameHasNoSetter = Object.getOwnPropertyDescriptor(BroadcastChannel.prototype, 'name').set === undefined;
		var methodsAreInherited = !Object.prototype.hasOwnProperty.call(ch, 'close') && !Object.prototype.hasOwnProperty.call(ch, 'postMessage');
		var methodsAreNonEnumerable = !Object.getOwnPropertyDescriptor(BroadcastChannel.prototype, 'close').enumerable && !Object.getOwnPropertyDescriptor(BroadcastChannel.prototype, 'postMessage').enumerable;
		var illegalNameReceiverThrew = false;
		try { Object.getOwnPropertyDescriptor(BroadcastChannel.prototype, 'name').get.call({}); }
		catch (error) { illegalNameReceiverThrew = error instanceof TypeError; }
		var illegalMethodReceiverThrew = false;
		try { BroadcastChannel.prototype.close.call({}); }
		catch (error) { illegalMethodReceiverThrew = error instanceof TypeError; }
		ch.close();
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if v, ok := r.Global("exists"); !ok || !v.Bool {
		t.Fatal("BroadcastChannel constructor is not a function on the global scope")
	}
	if v, ok := r.Global("onWin"); !ok || !v.Bool {
		t.Fatal("BroadcastChannel constructor is not on window")
	}
	if v, ok := r.Global("name"); !ok || v.Str != "test" {
		t.Fatalf("channel name = %+v, want %q", v, "test")
	}
	if v, ok := r.Global("nameWritable"); !ok || v.Bool {
		t.Fatalf("Reflect.set on BroadcastChannel.name = %+v, want false", v)
	}
	if v, ok := r.Global("nameAfterAssignment"); !ok || v.Str != "test" {
		t.Fatalf("channel name after assignment = %+v, want unchanged", v)
	}
	if v, ok := r.Global("hasOwnName"); !ok || v.Bool {
		t.Fatalf("BroadcastChannel.name own property = %+v, want inherited attribute", v)
	}
	for _, key := range []string{"isEventTarget", "prototypeInheritsEventTarget", "eventTargetOperationsInherited", "eventTargetOperationsNotOwn", "eventTargetOperationsNonEnumerable", "onmessageIsInherited", "prototypeNameHasNoSetter", "methodsAreInherited", "methodsAreNonEnumerable", "illegalNameReceiverThrew", "illegalMethodReceiverThrew"} {
		if v, ok := r.Global(key); !ok || !v.Bool {
			t.Fatalf("%s = %+v, want true", key, v)
		}
	}
}

// TestBroadcastChannelRequiresName pins the spec rule: constructing without a
// name throws a TypeError.
func TestBroadcastChannelRequiresName(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	err := r.Run(`
		var threw = false;
		try { new BroadcastChannel(); } catch (e) { threw = true; }
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("threw"); !ok || !v.Bool {
		t.Fatal("BroadcastChannel() without arguments did not throw")
	}
}

// TestBroadcastChannelEmptyName pins the constructor's DOMString conversion:
// an empty string is a valid channel name.
func TestBroadcastChannelEmptyName(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	err := r.Run(`
		var channel = new BroadcastChannel('');
		var name = channel.name;
		channel.close();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("name"); !ok || v.Str != "" {
		t.Fatalf("empty BroadcastChannel name = %+v, want empty string", v)
	}
}

// TestBroadcastChannelSameNameExchange is the core acceptance test: two
// channels with the same name can exchange messages via postMessage and
// onmessage.
func TestBroadcastChannelSameNameExchange(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var received = [];
		var receivedEvent = '';
		var savedMessageEvent;
		var ch1 = new BroadcastChannel('greet');
		var ch2 = new BroadcastChannel('greet');
		ch2.onmessage = function(e) {
			received.push(e.data);
			savedMessageEvent = e;
			try { receivedEvent = [e instanceof MessageEvent, e.type, e.source === null, e.ports && e.ports.length, e.bubbles, e.cancelable, e.target === ch2, e.currentTarget === ch2, e.isTrusted, e.eventPhase === 2, e.timeStamp > 0, e.composedPath()[0] === ch2].join('|'); }
			catch (error) { receivedEvent = 'error:' + error; }
		};
		ch1.postMessage('hello');
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Drain events so the queued message fires onmessage.
	r.Tick()

	if err := r.Run(`var count = received.length; var msg = received[0];`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("count"); !ok || v.Num != 1 {
		t.Fatalf("received count = %+v, want 1", v)
	}
	if v, ok := r.Global("msg"); !ok || v.Str != "hello" {
		t.Fatalf("received msg = %+v, want %q", v, "hello")
	}
	if v, ok := r.Global("receivedEvent"); !ok || v.Str != "true|message|true|0|false|false|true|true|true|true|true|true" {
		t.Fatalf("BroadcastChannel MessageEvent fields = %+v", v)
	}
	if got := evalString(t, r, `savedMessageEvent.eventPhase === 0 && savedMessageEvent.currentTarget === null && savedMessageEvent.composedPath().length === 0 && savedMessageEvent.target === ch2`); got != "true" {
		t.Fatalf("MessageEvent state after dispatch = %s, want reset currentTarget/phase/path and retained target", got)
	}

	// Cleanup.
	_ = r.Run(`ch1.close(); ch2.close();`, "cleanup.js")
}

// TestBroadcastChannelDifferentNamesDoNotInterfere verifies that channels with
// different names are isolated: posting on one does not reach the other.
func TestBroadcastChannelDifferentNamesDoNotInterfere(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var received = [];
		var ch1 = new BroadcastChannel('alpha');
		var ch2 = new BroadcastChannel('beta');
		ch2.onmessage = function(e) { received.push(e.data); };
		ch1.postMessage('should not arrive');
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.Tick()

	if err := r.Run(`var count = received.length;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("count"); !ok || v.Num != 0 {
		t.Fatalf("received count = %+v, want 0 (different names must not interfere)", v)
	}

	_ = r.Run(`ch1.close(); ch2.close();`, "cleanup.js")
}

func TestBroadcastChannelEventListenersOnceAndRemove(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var onceCalls = 0, removedCalls = 0;
		var receiver = new BroadcastChannel('listener-rules');
		var sender = new BroadcastChannel('listener-rules');
		var onceHandler = function() { onceCalls++; };
		var removedHandler = function() { removedCalls++; };
		receiver.addEventListener('message', onceHandler, {once: true});
		receiver.addEventListener('message', removedHandler);
		receiver.removeEventListener('message', removedHandler);
		sender.postMessage('first');
	`, "event-listeners.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if err := r.Run(`sender.postMessage('second');`, "event-listeners.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `String(onceCalls) + '|' + String(removedCalls)`); got != "1|0" {
		t.Fatalf("listener once/remove results = %q, want 1|0", got)
	}
}

func TestBroadcastChannelOnMessageUsesListenerRegistrationOrder(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var order = [];
		var receiver = new BroadcastChannel('handler-order');
		var sender = new BroadcastChannel('handler-order');
		var firstHandler = function() { order.push('first'); };
		var replacementHandler = function() { order.push('replacement'); };
		var listener = function() { order.push('listener'); };
		receiver.onmessage = firstHandler;
		receiver.addEventListener('message', listener);
		receiver.onmessage = replacementHandler;
		sender.postMessage('replace');
	`, "handler-order.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `order.join('|')`); got != "replacement|listener" {
		t.Fatalf("replacing onmessage changed its registration position: %q", got)
	}
	if err := r.Run(`
		order.length = 0;
		receiver.onmessage = null;
		receiver.onmessage = function() { order.push('readded'); };
		sender.postMessage('readd');
	`, "handler-order.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `order.join('|')`); got != "listener|readded" {
		t.Fatalf("re-added onmessage did not take a new registration position: %q", got)
	}
	_ = r.Run(`receiver.close(); sender.close();`, "cleanup.js")
}

func TestBroadcastChannelListenerMutationsDuringDispatch(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var calls = [];
		var mutated = false;
		var receiver = new BroadcastChannel('listener-mutation');
		var sender = new BroadcastChannel('listener-mutation');
		var late = function() { calls.push('late'); };
		var added = function() { calls.push('added'); };
		var first = function() {
			calls.push('first');
			if (!mutated) {
				mutated = true;
				receiver.removeEventListener('message', late);
				receiver.addEventListener('message', added);
			}
		};
		receiver.addEventListener('message', first);
		receiver.addEventListener('message', late);
		sender.postMessage('first');
	`, "listener-mutation.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `calls.join('|')`); got != "first" {
		t.Fatalf("removed/new listener ran during active dispatch: %q", got)
	}
	if err := r.Run(`sender.postMessage('second');`, "listener-mutation.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `calls.join('|')`); got != "first|first|added" {
		t.Fatalf("listener mutation results across dispatches = %q", got)
	}
	_ = r.Run(`receiver.close(); sender.close();`, "cleanup.js")
}

func TestBroadcastChannelStopImmediatePropagation(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var calls = [];
		var receiver = new BroadcastChannel('stop-immediate');
		var sender = new BroadcastChannel('stop-immediate');
		receiver.addEventListener('message', function(event) { calls.push('first'); event.stopImmediatePropagation(); });
		receiver.addEventListener('message', function() { calls.push('second'); });
		sender.postMessage('stop');
	`, "stop-immediate.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `calls.join('|')`); got != "first" {
		t.Fatalf("stopImmediatePropagation delivered later listeners: %q", got)
	}
	_ = r.Run(`receiver.close(); sender.close();`, "cleanup.js")
}

func TestBroadcastChannelDispatchEventUsesChannelListeners(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var dispatchCalls = [];
		var channel = new BroadcastChannel('manual-event-dispatch');
		channel.addEventListener('message', function(event) {
			dispatchCalls.push('listener:' + event.eventPhase + ':' + event.isTrusted);
			event.preventDefault();
		});
		channel.onmessage = function() { dispatchCalls.push('handler'); };
		var manualEvent = new MessageEvent('message', {data: 'manual', cancelable: true});
		var manualEventIsEvent = manualEvent instanceof Event;
		var constructedEventIsUntrusted = manualEvent.isTrusted === false;
		var dispatchResult = channel.dispatchEvent(manualEvent);
		var dispatchState = [manualEvent.target === channel, manualEvent.currentTarget === null, manualEvent.eventPhase === 0, manualEvent.defaultPrevented].join('|');
		var invalidEventThrew = false;
		try { channel.dispatchEvent({type: 'message'}); }
		catch (error) { invalidEventThrew = error instanceof TypeError; }
		var passiveChannel = new BroadcastChannel('manual-passive-event-dispatch');
		passiveChannel.addEventListener('message', function(event) { event.preventDefault(); }, {passive: true});
		var passiveEvent = new MessageEvent('message', {cancelable: true});
		var passiveDispatchResult = passiveChannel.dispatchEvent(passiveEvent);
		var passiveDefaultPrevented = passiveEvent.defaultPrevented;
		channel.close();
		passiveChannel.close();
	`, "manual-dispatch.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := evalString(t, r, `dispatchCalls.join('|')`); got != "listener:2:false|handler" {
		t.Fatalf("manual dispatch calls = %q, want listener then event handler at target", got)
	}
	if got := evalString(t, r, `String(dispatchResult) + '|' + dispatchState`); got != "false|true|true|true|true" {
		t.Fatalf("manual dispatch result/state = %q, want canceled event with reset dispatch state", got)
	}
	if got := evalString(t, r, `String(manualEventIsEvent) + '|' + String(constructedEventIsUntrusted) + '|' + String(invalidEventThrew)`); got != "true|true|true" {
		t.Fatalf("Event brand/trust validation = %q, want valid untrusted Event and TypeError for plain object", got)
	}
	if got := evalString(t, r, `String(passiveDispatchResult) + '|' + String(passiveDefaultPrevented)`); got != "true|false" {
		t.Fatalf("passive listener cancellation = %q, want uncanceled event", got)
	}
}

func TestBroadcastChannelListenerAbortSignal(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var calls = 0;
		var liveCalls = 0;
		var receiver = new BroadcastChannel('listener-signal');
		var sender = new BroadcastChannel('listener-signal');
		var controller = new AbortController();
		var liveController = new AbortController();
		var alreadyAborted = AbortSignal.abort();
		var listener = function() { calls++; };
		var liveListener = function() { liveCalls++; };
		receiver.addEventListener('message', listener, {signal: alreadyAborted});
		receiver.addEventListener('message', listener, {signal: controller.signal});
		receiver.addEventListener('message', liveListener, {signal: liveController.signal});
		controller.abort();
		sender.postMessage('aborted');
	`, "listener-signal.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `String(calls)`); got != "0" {
		t.Fatalf("aborted BroadcastChannel listener calls = %q, want 0", got)
	}
	if got := evalString(t, r, `String(liveCalls)`); got != "1" {
		t.Fatalf("active BroadcastChannel listener calls = %q, want 1", got)
	}
	if err := r.Run(`liveController.abort(); sender.postMessage('after-abort');`, "listener-signal.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `String(liveCalls)`); got != "1" {
		t.Fatalf("aborted active listener calls = %q, want it to remain 1", got)
	}
	if err := r.Run(`
		var invalidSignalThrew = false;
		try { receiver.addEventListener('message', listener, {signal: {aborted: false}}); }
		catch (error) { invalidSignalThrew = error instanceof TypeError; }
	`, "listener-signal.js"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, r, `String(invalidSignalThrew)`); got != "true" {
		t.Fatalf("invalid listener signal rejection = %q, want true", got)
	}
	_ = r.Run(`receiver.close(); sender.close();`, "cleanup.js")
}

// TestBroadcastChannelCloseStopsReceiving verifies that after close(), the
// channel no longer receives messages.
func TestBroadcastChannelCloseStopsReceiving(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var received = [];
		var ch1 = new BroadcastChannel('closeTest');
		var ch2 = new BroadcastChannel('closeTest');
		ch2.onmessage = function(e) { received.push(e.data); };
		ch1.postMessage('before close');
		ch2.close();
		ch1.postMessage('after close');
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.Tick()

	if err := r.Run(`var count = received.length; var msg = received[0];`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("count"); !ok || v.Num != 0 {
		t.Fatalf("received count = %+v, want 0 (close cancels queued messages)", v)
	}
	_ = r.Run(`ch1.close();`, "cleanup.js")
}

func TestBroadcastChannelCloseDuringMessageHandlerCancelsQueuedDelivery(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})
	if err := r.Run(`
		var received = [];
		var sender = new BroadcastChannel('close-in-handler');
		var receiver = new BroadcastChannel('close-in-handler');
		receiver.onmessage = function(event) {
			received.push(event.data);
			if (event.data === 'first') receiver.close();
		};
		sender.postMessage('first');
		sender.postMessage('second');
	`, "close-during-message.js"); err != nil {
		t.Fatal(err)
	}
	r.Tick()
	if got := evalString(t, r, `received.join(',')`); got != "first" {
		t.Fatalf("messages received after close in handler = %q, want first only", got)
	}
	_ = r.Run(`sender.close();`, "cleanup.js")
}

// TestBroadcastChannelSenderDoesNotReceive verifies that postMessage does not
// deliver to the sending channel itself -- only to peers.
func TestBroadcastChannelSenderDoesNotReceive(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var selfReceived = [];
		var ch1 = new BroadcastChannel('selfTest');
		ch1.onmessage = function(e) { selfReceived.push(e.data); };
		ch1.postMessage('echo?');
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.Tick()

	if err := r.Run(`var count = selfReceived.length;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("count"); !ok || v.Num != 0 {
		t.Fatalf("sender received count = %+v, want 0 (must not receive own messages)", v)
	}

	_ = r.Run(`ch1.close();`, "cleanup.js")
}

// TestBroadcastChannelMultiplePeers verifies that postMessage delivers to all
// other channels with the same name, not just one.
func TestBroadcastChannelMultiplePeers(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var r1 = [];
		var r2 = [];
		var r3 = [];
		var sender = new BroadcastChannel('multi');
		var peer1 = new BroadcastChannel('multi');
		var peer2 = new BroadcastChannel('multi');
		peer1.onmessage = function(e) { r1.push(e.data); };
		peer2.onmessage = function(e) { r2.push(e.data); };
		sender.onmessage = function(e) { r3.push(e.data); };
		sender.postMessage('broadcast');
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.Tick()

	if err := r.Run(`
		var c1 = r1.length;
		var c2 = r2.length;
		var c3 = r3.length;
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, tc := range []struct {
		name string
		want float64
		got  js.Value
		ok   bool
	}{
		{"peer1", 1, js.Value{}, false},
		{"peer2", 1, js.Value{}, false},
		{"sender", 0, js.Value{}, false},
	} {
		switch tc.name {
		case "peer1":
			tc.got, tc.ok = r.Global("c1")
		case "peer2":
			tc.got, tc.ok = r.Global("c2")
		case "sender":
			tc.got, tc.ok = r.Global("c3")
		}
		if !tc.ok || tc.got.Num != tc.want {
			t.Errorf("%s received = %+v, want %v", tc.name, tc.got, tc.want)
		}
	}

	_ = r.Run(`sender.close(); peer1.close(); peer2.close();`, "cleanup.js")
}

// TestBroadcastChannelObjectPayload verifies that structured data (objects)
// round-trips through postMessage.
func TestBroadcastChannelObjectPayload(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var received = null;
		var ch1 = new BroadcastChannel('obj');
		var ch2 = new BroadcastChannel('obj');
		ch2.onmessage = function(e) { received = e.data; };
		ch1.postMessage({key: 'value', num: 42});
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.Tick()

	if err := r.Run(`var k = received && received.key; var n = received && received.num;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("k"); !ok || v.Str != "value" {
		t.Fatalf("received.key = %+v, want %q", v, "value")
	}
	if v, ok := r.Global("n"); !ok || v.Num != 42 {
		t.Fatalf("received.num = %+v, want 42", v)
	}

	_ = r.Run(`ch1.close(); ch2.close();`, "cleanup.js")
}

// TestBroadcastChannelNumberPayload verifies that numeric data round-trips.
func TestBroadcastChannelNumberPayload(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var received = null;
		var ch1 = new BroadcastChannel('num');
		var ch2 = new BroadcastChannel('num');
		ch2.onmessage = function(e) { received = e.data; };
		ch1.postMessage(3.14);
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.Tick()

	if err := r.Run(`var val = received;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("val"); !ok || v.Num != 3.14 {
		t.Fatalf("received = %+v, want 3.14", v)
	}

	_ = r.Run(`ch1.close(); ch2.close();`, "cleanup.js")
}

// TestBroadcastChannelCrossRuntime verifies that channels in different
// runtimes (simulating different tabs) sharing the same hub can exchange
// messages.
func TestBroadcastChannelCrossRuntime(t *testing.T) {
	hub := js.NewBroadcastHub()

	r1, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://broadcast.example/one", BroadcastHub: hub})
	if err != nil {
		t.Fatalf("New r1: %v", err)
	}
	defer r1.Close()

	r2, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://broadcast.example/two", BroadcastHub: hub})
	if err != nil {
		t.Fatalf("New r2: %v", err)
	}
	defer r2.Close()

	// Tab 1 creates a channel and listens.
	if err := r1.Run(`
		var tab1received = [];
		var ch1 = new BroadcastChannel('cross');
		ch1.onmessage = function(e) { tab1received.push(e.data); };
	`, "tab1.js"); err != nil {
		t.Fatalf("Run r1: %v", err)
	}

	// Tab 2 creates a channel with the same name and posts.
	if err := r2.Run(`
		var ch2 = new BroadcastChannel('cross');
		ch2.postMessage('from tab2');
	`, "tab2.js"); err != nil {
		t.Fatalf("Run r2: %v", err)
	}

	// Drain events on tab 1 so the message fires.
	r1.Tick()

	if err := r1.Run(`var count = tab1received.length; var msg = tab1received[0];`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r1.Global("count"); !ok || v.Num != 1 {
		t.Fatalf("tab1 received count = %+v, want 1", v)
	}
	if v, ok := r1.Global("msg"); !ok || v.Str != "from tab2" {
		t.Fatalf("tab1 received msg = %+v, want %q", v, "from tab2")
	}
	if got := evalString(t, r1, `ch1.name`); got != "cross" {
		t.Fatalf("receiver channel name = %q, want cross", got)
	}

	_ = r1.Run(`ch1.close();`, "cleanup1.js")
	_ = r2.Run(`ch2.close();`, "cleanup2.js")
}

func TestBroadcastChannelIsScopedToOriginAndReportsSenderOrigin(t *testing.T) {
	hub := js.NewBroadcastHub()
	newOriginRuntime := func(rawURL string) *js.Runtime {
		t.Helper()
		r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: rawURL, BroadcastHub: hub})
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	first := newOriginRuntime("https://first.example/page")
	second := newOriginRuntime("https://first.example/other")
	third := newOriginRuntime("https://second.example/page")
	for _, runtime := range []*js.Runtime{first, second, third} {
		if err := runtime.Run(`
			var received = [];
			var channel = new BroadcastChannel('origin-scope');
			channel.onmessage = function(event) { received.push(event.origin + '|' + event.data); };
		`, "broadcast-origin-scope.js"); err != nil {
			t.Fatalf("create receiver channel: %v", err)
		}
	}
	if err := third.Run(`var sender = new BroadcastChannel('origin-scope'); sender.postMessage('must-not-cross-origin');`, "broadcast-origin-send.js"); err != nil {
		t.Fatalf("post cross-origin message: %v", err)
	}
	first.Tick()
	second.Tick()
	third.Tick()
	if err := third.Run(`received = [];`, "broadcast-origin-reset.js"); err != nil {
		t.Fatalf("reset same-runtime receiver: %v", err)
	}
	if got := evalString(t, first, `received.length + '|' + (received[0] || '')`); got != "0|" {
		t.Fatalf("cross-origin recipient received = %q, want 0 messages", got)
	}
	if got := evalString(t, second, `received.length + '|' + (received[0] || '')`); got != "0|" {
		t.Fatalf("same-origin peer received cross-origin message = %q, want 0", got)
	}
	if err := second.Run(`channel.onmessage = function(event) {
		received.push(event.origin + '|' + (event.data.self === event.data) + '|' + (event.data.left === event.data.right));
	};`, "broadcast-origin-listener.js"); err != nil {
		t.Fatalf("install graph receiver: %v", err)
	}
	if err := first.Run(`var shared = {token: 7}; var payload = {left: shared, right: shared}; payload.self = payload; channel.postMessage(payload);`, "broadcast-origin-send.js"); err != nil {
		t.Fatalf("post same-origin message: %v", err)
	}
	second.Tick()
	third.Tick()
	if got := evalString(t, second, `received.length + '|' + received[0]`); got != "1|https://first.example|true|true" {
		t.Fatalf("same-origin event origin/clone graph = %q, want sender origin and preserved graph", got)
	}
	if got := evalString(t, third, `received.length`); got != "0" {
		t.Fatalf("cross-origin peer received same-origin message: %s", got)
	}
}

// TestBroadcastChannelDoubleClose verifies that calling close() twice does not
// panic or error.
func TestBroadcastChannelDoubleClose(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var ch = new BroadcastChannel('double');
		ch.close();
		ch.close();
		var ok = true;
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if v, ok := r.Global("ok"); !ok || !v.Bool {
		t.Fatal("double close did not complete cleanly")
	}
}

// TestBroadcastChannelPostAfterClose verifies that postMessage after close()
// throws InvalidStateError and does not deliver.
func TestBroadcastChannelPostAfterClose(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second})

	if err := r.Run(`
		var received = [];
		var closedError = '';
		var ch1 = new BroadcastChannel('postClose');
		var ch2 = new BroadcastChannel('postClose');
		ch2.onmessage = function(e) { received.push(e.data); };
		ch1.close();
		try { ch1.postMessage('should not arrive'); } catch (error) { closedError = error.name; }
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := evalString(t, r, `closedError`); got != "InvalidStateError" {
		t.Fatalf("postMessage after close error = %q, want InvalidStateError", got)
	}

	r.Tick()

	if err := r.Run(`var count = received.length;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("count"); !ok || v.Num != 0 {
		t.Fatalf("received count = %+v, want 0 (post after close must not deliver)", v)
	}

	_ = r.Run(`ch2.close();`, "cleanup.js")
}

func TestBroadcastChannelPostMessageCloneErrorsAndUndefined(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second, URL: "https://clone.example/"})
	if err := r.Run(`
		var missingArgumentError = '';
		var cloneError = '';
		var cloneErrorCode = 0;
		var closedErrorCode = 0;
		var receivedTypes = [];
		var receiver = new BroadcastChannel('clone-errors');
		var sender = new BroadcastChannel('clone-errors');
		receiver.onmessage = function(event) { receivedTypes.push(typeof event.data); };
		try { sender.postMessage(); } catch (error) { missingArgumentError = error.name; }
		try { sender.postMessage(function() {}); } catch (error) { cloneError = error.name; cloneErrorCode = error.code; }
		sender.postMessage(undefined);
		sender.close();
		try { sender.postMessage(Symbol()); } catch (error) { closedErrorCode = error.code; }
	`, "broadcast-clone-errors.js"); err != nil {
		t.Fatalf("post messages: %v", err)
	}
	r.Tick()
	if got := evalString(t, r, `missingArgumentError + '|' + cloneError + '|' + cloneErrorCode + '|' + closedErrorCode + '|' + receivedTypes.join(',')`); got != "TypeError|DataCloneError|25|11|undefined" {
		t.Fatalf("BroadcastChannel clone results = %q, want TypeError|DataCloneError|25|11|undefined", got)
	}
}

func TestBroadcastChannelStructuredClonePreservesCyclesAndValues(t *testing.T) {
	r := newRuntime(t, js.Options{Timeout: 5 * time.Second, URL: "https://clone-graph.example/"})
	if err := r.Run(`
		var cloneGraphResult = '';
		var receiver = new BroadcastChannel('clone-graph');
		var sender = new BroadcastChannel('clone-graph');
		receiver.onmessage = function(event) {
			var value = event.data;
			cloneGraphResult = [
				value.self === value,
				value.left === value.right,
				!(0 in value.sparse) && (1 in value.sparse) && value.sparse[1] === undefined,
				Number.isNaN(value.nan), value.infinity === Infinity,
				Object.is(value.negativeZero, -0),
				Object.prototype.hasOwnProperty.call(value, '__proto__') && value.__proto__ === 7
			].join('|');
		};
		var shared = {value: 1};
		var payload = {left: shared, right: shared, nan: NaN, infinity: Infinity, negativeZero: -0};
		payload.self = payload;
		payload.sparse = [];
		payload.sparse.length = 2;
		payload.sparse[1] = undefined;
		Object.defineProperty(payload, '__proto__', {value: 7, enumerable: true});
		sender.postMessage(payload);
	`, "broadcast-clone-graph.js"); err != nil {
		t.Fatalf("post clone graph: %v", err)
	}
	r.Tick()
	if got := evalString(t, r, `cloneGraphResult`); got != "true|true|true|true|true|true|true" {
		t.Fatalf("structured clone graph = %q, want all preserved properties", got)
	}
}

func TestBroadcastChannelStructuredCloneBuiltinsAcrossRuntimes(t *testing.T) {
	hub := js.NewBroadcastHub()
	newRuntime := func(rawURL string) *js.Runtime {
		t.Helper()
		r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: rawURL, BroadcastHub: hub})
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	sender := newRuntime("https://clone-builtins.example/sender")
	receiver := newRuntime("https://clone-builtins.example/receiver")
	if err := receiver.Run(`
		var builtinsResult = '';
		var clonedBlobText = '';
		var clonedFileText = '';
		var sharedAliasMutation = false;
		var channel = new BroadcastChannel('builtins');
		channel.onmessageerror = function() { builtinsResult = 'messageerror'; };
		channel.onmessage = function(event) {
		try {
			var value = event.data;
			var bytes = new Uint8Array(value.buffer);
			var dataViewBytes = new Uint8Array(value.dataView.buffer);
			var sharedViewBytes = new Uint8Array(value.viewBuffer);
			value.blob.text().then(function(text) { clonedBlobText = text; });
			value.file.text().then(function(text) { clonedFileText = text; });
			builtinsResult = [
				value.map instanceof Map,
				value.map.get('self') === value.map,
				value.set instanceof Set && value.set.has(value.set) && value.set.has(value.map),
				value.date instanceof Date && value.date.getTime() === 1735689600123,
				value.invalidDate instanceof Date && Number.isNaN(value.invalidDate.getTime()),
				value.regexp instanceof RegExp && value.regexp.source === 'a+b' && value.regexp.flags === 'gi',
				value.integer === 9007199254740993n,
				bytes.length === 3 && bytes[0] === 9 && bytes[2] === 7,
				value.buffer === value.bufferAlias,
				value.view instanceof Int16Array && value.view[0] === -2 && value.view[1] === 32767,
				value.view.buffer === value.viewBuffer && value.view.byteOffset === 2 && value.view.byteLength === 4,
				value.overlap instanceof Uint8Array && value.overlap.buffer === value.view.buffer && value.overlap.byteOffset === 3 && value.overlap[1] === sharedViewBytes[4],
				value.bigView instanceof BigUint64Array && value.bigView[1] === 18446744073709551615n,
				value.dataView instanceof DataView && value.dataView.byteOffset === 1 && value.dataView.byteLength === 4 && value.dataView.getUint16(0) === 0x1234 && dataViewBytes[0] === 0 && dataViewBytes[1] === 0x12,
				value.blob instanceof Blob && value.blob.size === 2 && value.blob.type === 'text/plain',
				value.file instanceof File && value.file.size === 2 && value.file.type === 'text/plain' && value.file.name === 'sample.txt' && value.file.lastModified === 12345
			].join('|');
			value.overlap[1] = 5;
			sharedAliasMutation = sharedViewBytes[4] === 5;
		} catch (error) { builtinsResult = 'handler-error:' + error; }
		};
	`, "broadcast-builtins-receiver.js"); err != nil {
		t.Fatalf("install builtin receiver: %v", err)
	}
	if err := sender.Run(`
		var channel = new BroadcastChannel('builtins');
		var map = new Map(); map.set('self', map);
		var set = new Set(); set.add(set); set.add(map);
		var buffer = new Uint8Array([9, 8, 7]).buffer;
		var dataViewBuffer = new Uint8Array([0, 0x12, 0x34, 0x56, 0x78, 0]).buffer;
		var viewBuffer = new ArrayBuffer(8);
		var view = new Int16Array(viewBuffer, 2, 2); view[0] = -2; view[1] = 32767;
		var overlap = new Uint8Array(viewBuffer, 3, 2);
		var blob = new Blob([new Uint8Array([65, 66])], {type: 'text/plain'});
		var file = new File(['hi'], 'sample.txt', {type: 'text/plain', lastModified: 12345});
		channel.postMessage({
			map: map, set: set,
			date: new Date(1735689600123), invalidDate: new Date(NaN),
			regexp: /a+b/gi, integer: 9007199254740993n,
			buffer: buffer, bufferAlias: buffer,
			viewBuffer: viewBuffer, view: view, overlap: overlap,
			bigView: new BigUint64Array([0n, 18446744073709551615n]),
			dataView: new DataView(dataViewBuffer, 1, 4),
			blob: blob, file: file
		});
	`, "broadcast-builtins-sender.js"); err != nil {
		t.Fatalf("post builtin values: %v", err)
	}
	receiver.Tick()
	if got := evalString(t, receiver, `builtinsResult`); got != "true|true|true|true|true|true|true|true|true|true|true|true|true|true|true|true" {
		t.Fatalf("structured-clone builtins = %q, want all true", got)
	}
	if got := evalString(t, receiver, `sharedAliasMutation`); got != "true" {
		t.Fatalf("typed-array views did not share the cloned buffer: %q", got)
	}
	receiver.Tick()
	if got := evalString(t, receiver, `clonedBlobText + '|' + clonedFileText`); got != "AB|hi" {
		t.Fatalf("cloned Blob/File bytes = %q, want AB|hi", got)
	}
}

func TestBroadcastChannelDetachedIframeContextsDoNotExchangeMessages(t *testing.T) {
	hub := js.NewBroadcastHub()
	r := newRuntime(t, js.Options{
		DOM:          testDOM(t),
		URL:          "https://detached-frame.example/",
		BroadcastHub: hub,
	})
	if err := r.Run(`
		var iframe = document.createElement('iframe');
		document.body.appendChild(iframe);
		var ChildBroadcastChannel = iframe.contentWindow.BroadcastChannel;
		var childConstructorType = typeof ChildBroadcastChannel;
		iframe.remove();
		var childChannel = new ChildBroadcastChannel('detached-frame');
		var parentChannel = new BroadcastChannel('detached-frame');
		var parentReceived = 0;
		var childReceived = 0;
		parentChannel.onmessage = function() { parentReceived++; };
		childChannel.onmessage = function() { childReceived++; };
		childChannel.postMessage('from-detached-frame');
		parentChannel.postMessage('to-detached-frame');
	`, "detached-iframe-broadcastchannel.js"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		r.Tick()
	}
	if got := evalString(t, r, `childConstructorType + '|' + parentReceived + '|' + childReceived`); got != "function|0|0" {
		t.Fatalf("detached iframe BroadcastChannel results = %q, want function|0|0", got)
	}
	_ = r.Run(`childChannel.close(); parentChannel.close();`, "cleanup.js")
}

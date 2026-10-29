package js_test

import "testing"

func TestObjectEventListenerReportsNonCallableHandleEvent(t *testing.T) {
	r := newEventRuntime(t, eventDOM())
	if err := r.Run(`
		var target = document.getElementById('target');
		window.handleEventError = null;
		window.handleEventErrorCalls = 0;
		window.addEventListener('error', function(event) {
			window.handleEventErrorCalls++;
			window.handleEventError = event.type + '|' + event.error.name + '|' + event.message;
		});
		var listener = {
			get handleEvent() {
				window.handleEventGetterCalls++;
				return null;
			}
		};
		window.handleEventGetterCalls = 0;
		target.addEventListener('custom', listener);
		target.dispatchEvent(new Event('custom'));
	`, "event-listener-handle-event.js"); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.Global("handleEventErrorCalls"); !ok || got.Num != 1 {
		t.Fatalf("Window error events for non-callable handleEvent = %+v, want one", got)
	}
	if got, ok := r.Global("handleEventGetterCalls"); !ok || got.Num != 1 {
		t.Fatalf("handleEvent getter calls = %+v, want one per dispatch", got)
	}
	if got, ok := r.Global("handleEventError"); !ok || got.Str == "" {
		calls, _ := r.Global("handleEventGetterCalls")
		t.Fatalf("reported event/error types = %+v, getter calls %+v, want ErrorEvent and TypeError", got, calls)
	}
	got, _ := r.Global("handleEventError")
	if got.Str != "error|TypeError|Value is not an object: null" {
		t.Fatalf("reported window error = %+v, want TypeError", got)
	}
}

package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestAbortController(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var controller = new AbortController();
		var signal = controller.signal;
		var aborted = signal.aborted;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("aborted")
	if val.Bool {
		t.Error("signal should not be aborted initially")
	}
}

func TestAbortControllerAbort(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var controller = new AbortController();
		var signal = controller.signal;
		controller.abort();
		var aborted = signal.aborted;
		var reasonName = signal.reason.name;
		var reasonText = String(signal.reason);
		var reasonIsDOMException = signal.reason instanceof DOMException;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("aborted")
	if !val.Bool {
		t.Error("signal should be aborted after abort()")
	}
	if v, ok := r.Global("reasonName"); !ok || v.Str != "AbortError" {
		t.Errorf("reason.name = %q, want %q", v.Str, "AbortError")
	}
	if v, ok := r.Global("reasonText"); !ok || v.Str != "AbortError: signal is aborted without reason" {
		t.Errorf("String(reason) = %q, want %q", v.Str, "AbortError: signal is aborted without reason")
	}
	if v, ok := r.Global("reasonIsDOMException"); !ok || !v.Bool {
		t.Errorf("reason instanceof DOMException = %v, want true", v.Bool)
	}
}

func TestAbortControllerAbortWithReason(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var controller = new AbortController();
		var signal = controller.signal;
		controller.abort('custom reason');
		var reason = signal.reason;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("reason")
	if val.Str != "custom reason" {
		t.Errorf("reason = %q, want %q", val.Str, "custom reason")
	}
}

func TestAbortControllerOnAbort(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var controller = new AbortController();
		var signal = controller.signal;
		var fired = false;
		signal.onabort = function() {
			fired = true;
		};
		controller.abort();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("fired")
	if !val.Bool {
		t.Error("onabort handler should have fired")
	}
}

func TestAbortControllerDoubleAbort(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var controller = new AbortController();
		var signal = controller.signal;
		controller.abort('first');
		controller.abort('second');
		var reason = signal.reason;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("reason")
	if val.Str != "first" {
		t.Errorf("reason = %q, want %q (should keep first)", val.Str, "first")
	}
}

func TestAbortControllerOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var controller = new window.AbortController();
		var signal = controller.signal;
		controller.abort();
		var aborted = signal.aborted;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("aborted")
	if !val.Bool {
		t.Error("signal should be aborted")
	}
}

// qc:BR-123
func TestAbortSignalTimeoutReasonUndefinedBeforeFire(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var signal = AbortSignal.timeout(100);
		var aborted = signal.aborted;
		var reasonType = typeof signal.reason;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if v, _ := r.Global("aborted"); v.Bool {
		t.Error("signal should not be aborted before timer fires")
	}
	if v, _ := r.Global("reasonType"); v.Str != "undefined" {
		t.Errorf("reason type = %q, want %q before timer fires", v.Str, "undefined")
	}
}

// qc:BR-123
func TestAbortSignalTimeoutFiresWithTimeoutError(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var signal = AbortSignal.timeout(10);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	r.Tick()

	err = r.Run(`
		var aborted = signal.aborted;
		var reasonName = signal.reason.name;
		var reasonMessage = signal.reason.message;
		var reasonCode = signal.reason.code;
		var reasonIsDOMException = signal.reason instanceof DOMException;
		var reasonIsError = signal.reason instanceof Error;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run after tick: %v", err)
	}

	if v, _ := r.Global("aborted"); !v.Bool {
		t.Error("signal should be aborted after timer fires")
	}
	if v, _ := r.Global("reasonName"); v.Str != "TimeoutError" {
		t.Errorf("reason.name = %q, want %q", v.Str, "TimeoutError")
	}
	if v, _ := r.Global("reasonMessage"); v.Str != "signal timed out" {
		t.Errorf("reason.message = %q, want %q", v.Str, "signal timed out")
	}
	if v, _ := r.Global("reasonCode"); int(v.Num) != 23 {
		t.Errorf("reason.code = %v, want 23", int(v.Num))
	}
	if v, _ := r.Global("reasonIsDOMException"); !v.Bool {
		t.Error("reason should be instanceof DOMException")
	}
	if v, _ := r.Global("reasonIsError"); !v.Bool {
		t.Error("reason should be instanceof Error")
	}
}

// qc:BR-123
func TestAbortSignalTimeoutFiresAbortEvent(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var signal = AbortSignal.timeout(10);
		var eventFired = false;
		var eventReasonName = null;
		signal.addEventListener('abort', function() {
			eventFired = true;
			eventReasonName = signal.reason.name;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	r.Tick()

	if v, _ := r.Global("eventFired"); !v.Bool {
		t.Error("abort event should have fired")
	}
	if v, _ := r.Global("eventReasonName"); v.Str != "TimeoutError" {
		t.Errorf("event reason.name = %q, want %q", v.Str, "TimeoutError")
	}
}

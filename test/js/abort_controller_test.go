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
	// Measured in Chromium: abort() with no argument installs a DOMException
	// named AbortError reading "AbortError: signal is aborted without reason".
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

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
		var reason = signal.reason;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("aborted")
	if !val.Bool {
		t.Error("signal should be aborted after abort()")
	}
	val, _ = r.Global("reason")
	if val.Str != "AbortError" {
		t.Errorf("reason = %q, want %q", val.Str, "AbortError")
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

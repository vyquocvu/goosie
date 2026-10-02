package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestCryptoGetRandomValues(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var arr = new Array(10);
		crypto.getRandomValues(arr);
		var len = arr.length;
		var allZero = true;
		for (var i = 0; i < 10; i++) {
			if (arr[i] !== 0) allZero = false;
		}
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("len")
	if val.Num != 10 {
		t.Errorf("length = %v, want 10", val.Num)
	}
	val, _ = r.Global("allZero")
	if val.Bool {
		t.Error("all values should not be zero (extremely unlikely)")
	}
}

func TestCryptoOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var hasCrypto = window.crypto !== undefined;
		var hasGetRandomValues = typeof window.crypto.getRandomValues === 'function';
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("hasCrypto")
	if !val.Bool {
		t.Error("window.crypto should exist")
	}
	val, _ = r.Global("hasGetRandomValues")
	if !val.Bool {
		t.Error("window.crypto.getRandomValues should be a function")
	}
}

func TestPerformanceNow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var t1 = performance.now();
		var t2 = performance.now();
		var diff = t2 - t1;
		var isNumber = typeof t1 === 'number';
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("isNumber")
	if !val.Bool {
		t.Error("performance.now() should return a number")
	}
	val, _ = r.Global("diff")
	if val.Num < 0 {
		t.Errorf("time diff = %v, should be >= 0", val.Num)
	}
}

func TestPerformanceTimeOrigin(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var origin = performance.timeOrigin;
		var isNumber = typeof origin === 'number';
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("isNumber")
	if !val.Bool {
		t.Error("performance.timeOrigin should be a number")
	}
}

func TestPerformanceOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var hasPerf = window.performance !== undefined;
		var hasNow = typeof window.performance.now === 'function';
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("hasPerf")
	if !val.Bool {
		t.Error("window.performance should exist")
	}
	val, _ = r.Global("hasNow")
	if !val.Bool {
		t.Error("window.performance.now should be a function")
	}
}

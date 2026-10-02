package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestPromiseConstructor(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	// Test that Promise constructor exists and is callable.
	err = r.Run(`
		var p = new Promise(function(resolve, reject) {
			resolve(42);
		});
		if (typeof p !== 'object') throw new Error('Promise should return object');
		if (typeof p.then !== 'function') throw new Error('Promise should have then method');
		if (typeof p.catch !== 'function') throw new Error('Promise should have catch method');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestPromiseResolve(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var result = null;
		var p = new Promise(function(resolve) {
			resolve('hello');
		});
		p.then(function(value) {
			result = value;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Check that the promise resolved with the correct value.
	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "hello" {
		t.Errorf("result = %q, want %q", val.Str, "hello")
	}
}

func TestPromiseReject(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var error = null;
		var p = new Promise(function(resolve, reject) {
			reject('error message');
		});
		p.catch(function(reason) {
			error = reason;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("error")
	if !ok {
		t.Fatal("error not found")
	}
	if val.Str != "error message" {
		t.Errorf("error = %q, want %q", val.Str, "error message")
	}
}

func TestPromiseChain(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var result = null;
		new Promise(function(resolve) {
			resolve(1);
		}).then(function(v) {
			return v + 1;
		}).then(function(v) {
			return v * 2;
		}).then(function(v) {
			result = v;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Num != 4 {
		t.Errorf("result = %v, want 4", val.Num)
	}
}

func TestPromiseResolveStatic(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var result = null;
		Promise.resolve('static').then(function(v) {
			result = v;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "static" {
		t.Errorf("result = %q, want %q", val.Str, "static")
	}
}

func TestPromiseRejectStatic(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var error = null;
		Promise.reject('rejected').catch(function(v) {
			error = v;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("error")
	if !ok {
		t.Fatal("error not found")
	}
	if val.Str != "rejected" {
		t.Errorf("error = %q, want %q", val.Str, "rejected")
	}
}

func TestPromiseAll(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var result = null;
		var p1 = Promise.resolve(1);
		var p2 = Promise.resolve(2);
		var p3 = Promise.resolve(3);
		Promise.all([p1, p2, p3]).then(function(values) {
			result = values;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Kind != js.KindObject {
		t.Fatalf("result kind = %v, want Object", val.Kind)
	}
}

func TestPromiseRace(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var result = null;
		var p1 = new Promise(function(resolve) {
			setTimeout(function() { resolve('slow'); }, 100);
		});
		var p2 = Promise.resolve('fast');
		Promise.race([p1, p2]).then(function(value) {
			result = value;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "fast" {
		t.Errorf("result = %q, want %q", val.Str, "fast")
	}
}

func TestPromiseFinally(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var finallyCalled = false;
		var result = null;
		Promise.resolve('value').finally(function() {
			finallyCalled = true;
		}).then(function(v) {
			result = v;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("finallyCalled")
	if !ok {
		t.Fatal("finallyCalled not found")
	}
	if !val.Bool {
		t.Error("finally was not called")
	}

	val, ok = r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "value" {
		t.Errorf("result = %q, want %q", val.Str, "value")
	}
}

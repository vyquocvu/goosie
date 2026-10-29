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

func TestPromiseExecutorThrowPreservesThrownValue(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var thrown = new DOMException('clone failed', 'DataCloneError');
		var executorCalled = false;
		var catchCalled = false;
		var rejection = null;
		var preserved = false;
		var rejectionString = '';
		var rejectionName = '';
		var thrownPromise = new Promise(function() { executorCalled = true; throw thrown; });
		var rejectionState = thrownPromise.__promise__.state;
		thrownPromise.catch(function(reason) {
			catchCalled = true;
			rejection = reason;
			preserved = rejection === thrown && rejection instanceof DOMException && rejection.name === 'DataCloneError';
			rejectionString = String(rejection);
			rejectionName = rejection && rejection.name;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	value, ok := r.Global("preserved")
	if !ok || !value.Bool {
		reason, _ := r.Global("rejectionString")
		name, _ := r.Global("rejectionName")
		state, _ := r.Global("rejectionState")
		called, _ := r.Global("executorCalled")
		catchCalled, _ := r.Global("catchCalled")
		t.Fatalf("Promise rejection identity/type = %+v, called=%+v catchCalled=%+v state=%q string=%q name=%q, want exact DOMException", value, called, catchCalled, state.Str, reason.Str, name.Str)
	}
}

func TestPromiseWithResolvers(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var capability = Promise.withResolvers();
		var resolvedValue = null;
		var rejectedValue = null;
		capability.promise.then(function(value) { resolvedValue = value; });
		capability.resolve(42);
		var rejected = Promise.withResolvers();
		rejected.promise.catch(function(reason) { rejectedValue = reason; });
		rejected.reject('nope');
		class SubPromise extends Promise {}
		var subclassCapability = SubPromise.withResolvers();
		var withResolversDescriptor = Object.getOwnPropertyDescriptor(Promise, 'withResolvers');
		if (!(capability.promise instanceof Promise)) throw new Error('result must be a Promise');
		if (!(subclassCapability.promise instanceof SubPromise)) throw new Error('must use the receiver constructor');
		if (withResolversDescriptor.enumerable || withResolversDescriptor.writable !== true) throw new Error('wrong method descriptor');
		if (Object.keys(Promise).indexOf('withResolvers') !== -1) throw new Error('withResolvers must be non-enumerable');
		if (Promise.withResolvers.length !== 0) throw new Error('withResolvers.length must be 0');
		if (Promise.withResolvers.name !== 'withResolvers') throw new Error('withResolvers.name must be withResolvers');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	resolved, ok := r.Global("resolvedValue")
	if !ok || resolved.Num != 42 {
		t.Fatalf("resolved value = %v, want 42", resolved)
	}
	rejected, ok := r.Global("rejectedValue")
	if !ok || rejected.Str != "nope" {
		t.Fatalf("rejected value = %v, want nope", rejected)
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

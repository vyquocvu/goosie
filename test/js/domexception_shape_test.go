package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// TestDOMExceptionShapeMatchesChromium verifies that DOMException instances
// have the same shape as Chromium's: non-enumerable code, no inherited stack,
// and Symbol.toStringTag set to "DOMException".
// qc:BR-123
func TestDOMExceptionShapeMatchesChromium(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	// Test 1: code should be non-enumerable
	if err := r.Run(`
		var ex = new DOMException('test message', 'AbortError');
		var keys = Object.keys(ex);
		if (keys.indexOf('code') !== -1) {
			throw new Error('code should not be enumerable, got keys: ' + keys.join(','));
		}
	`, "inline"); err != nil {
		t.Errorf("code enumerable check: %v", err)
	}

	// Test 2: stack should not be present
	if err := r.Run(`
		var ex = new DOMException('test message', 'AbortError');
		if (ex.hasOwnProperty('stack') || 'stack' in ex) {
			throw new Error('stack should not be present on DOMException');
		}
	`, "inline"); err != nil {
		t.Errorf("stack check: %v", err)
	}

	// Test 3: Symbol.toStringTag should be "DOMException"
	if err := r.Run(`
		var ex = new DOMException('test message', 'AbortError');
		var tag = Object.prototype.toString.call(ex);
		if (tag !== '[object DOMException]') {
			throw new Error('toStringTag should be DOMException, got: ' + tag);
		}
	`, "inline"); err != nil {
		t.Errorf("toStringTag check: %v", err)
	}

	// Test 4: Verify the full shape matches Chromium's measurement
	if err := r.Run(`
		var ex = new DOMException('signal is aborted without reason', 'AbortError');
		
		// ownKeys should be empty (no enumerable own properties)
		var ownKeys = Object.keys(ex);
		if (ownKeys.length !== 0) {
			throw new Error('ownKeys should be empty, got: ' + ownKeys.join(','));
		}
		
		// hasStack should be false
		var hasStack = ex.stack !== undefined;
		if (hasStack) {
			throw new Error('hasStack should be false');
		}
		
		// tag should be [object DOMException]
		var tag = Object.prototype.toString.call(ex);
		if (tag !== '[object DOMException]') {
			throw new Error('tag should be [object DOMException], got: ' + tag);
		}
		
		// protoChain should be DOMException -> Error -> Object
		if (!(ex instanceof DOMException)) {
			throw new Error('should be instanceof DOMException');
		}
		if (!(ex instanceof Error)) {
			throw new Error('should be instanceof Error');
		}
		
		// code should be 20 for AbortError
		if (ex.code !== 20) {
			throw new Error('code should be 20, got: ' + ex.code);
		}
		
		// isError and isDOM should both be true
		if (!(ex instanceof Error)) {
			throw new Error('isError should be true');
		}
		if (!(ex instanceof DOMException)) {
			throw new Error('isDOM should be true');
		}
		
		// str should be "AbortError: signal is aborted without reason"
		var str = ex.toString();
		if (str !== 'AbortError: signal is aborted without reason') {
			throw new Error('str should be "AbortError: signal is aborted without reason", got: ' + str);
		}
	`, "inline"); err != nil {
		t.Errorf("full shape check: %v", err)
	}

	// Test 5: Empty DOMException should stringify to empty string
	if err := r.Run(`
		var ex = new DOMException('', '');
		var str = String(ex);
		if (str !== '') {
			throw new Error('empty DOMException should stringify to empty string, got: ' + str);
		}
	`, "inline"); err != nil {
		t.Errorf("empty DOMException check: %v", err)
	}
}

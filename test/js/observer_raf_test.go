package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestIntersectionObserverConstructor(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new IntersectionObserver(function(entries) {
			// callback
		});
		if (typeof observer !== 'object') throw new Error('Observer should be object');
		if (typeof observer.observe !== 'function') throw new Error('Observer should have observe method');
		if (typeof observer.unobserve !== 'function') throw new Error('Observer should have unobserve method');
		if (typeof observer.disconnect !== 'function') throw new Error('Observer should have disconnect method');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIntersectionObserverWithOptions(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new IntersectionObserver(function(entries) {}, {
			rootMargin: '10px 20px',
			threshold: 0.5
		});
		if (typeof observer !== 'object') throw new Error('Observer should be object');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIntersectionObserverObserve(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var obj = {};
		var observer = new IntersectionObserver(function(entries) {});
		observer.observe(obj);
		// Should not throw.
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIntersectionObserverUnobserve(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var obj = {};
		var observer = new IntersectionObserver(function(entries) {});
		observer.observe(obj);
		observer.unobserve(obj);
		// Should not throw.
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIntersectionObserverDisconnect(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var obj1 = {};
		var obj2 = {};
		var observer = new IntersectionObserver(function(entries) {});
		observer.observe(obj1);
		observer.observe(obj2);
		observer.disconnect();
		// Should not throw.
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRequestAnimationFrame(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var called = false;
		var id = requestAnimationFrame(function(timestamp) {
			called = true;
		});
		if (typeof id !== 'number') throw new Error('rAF should return number');
		if (id <= 0) throw new Error('rAF id should be positive');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRequestAnimationFrameOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var id = window.requestAnimationFrame(function(timestamp) {});
		if (typeof id !== 'number') throw new Error('window.rAF should return number');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCancelAnimationFrame(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var id = requestAnimationFrame(function(timestamp) {});
		cancelAnimationFrame(id);
		// Should not throw.
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCancelAnimationFrameOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var id = window.requestAnimationFrame(function(timestamp) {});
		window.cancelAnimationFrame(id);
		// Should not throw.
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRequestAnimationFrameMultiple(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var id1 = requestAnimationFrame(function() {});
		var id2 = requestAnimationFrame(function() {});
		var id3 = requestAnimationFrame(function() {});
		if (id1 === id2 || id2 === id3 || id1 === id3) {
			throw new Error('rAF ids should be unique');
		}
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

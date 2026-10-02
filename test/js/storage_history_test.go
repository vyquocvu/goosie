package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestLocalStorageSetGetItem(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		localStorage.setItem('key1', 'value1');
		var result = localStorage.getItem('key1');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "value1" {
		t.Errorf("result = %q, want %q", val.Str, "value1")
	}
}

func TestLocalStorageRemoveItem(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		localStorage.setItem('key1', 'value1');
		localStorage.removeItem('key1');
		var result = localStorage.getItem('key1');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Kind != js.KindNull {
		t.Errorf("result should be null after removeItem, got %v", val.Kind)
	}
}

func TestLocalStorageClear(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		localStorage.setItem('key1', 'value1');
		localStorage.setItem('key2', 'value2');
		localStorage.clear();
		var len = localStorage.length();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 0 {
		t.Errorf("length = %v, want 0 after clear", val.Num)
	}
}

func TestLocalStorageLength(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		localStorage.setItem('key1', 'value1');
		localStorage.setItem('key2', 'value2');
		localStorage.setItem('key3', 'value3');
		var len = localStorage.length();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 3 {
		t.Errorf("length = %v, want 3", val.Num)
	}
}

func TestSessionStorageBasic(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		sessionStorage.setItem('sessionKey', 'sessionValue');
		var result = sessionStorage.getItem('sessionKey');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "sessionValue" {
		t.Errorf("result = %q, want %q", val.Str, "sessionValue")
	}
}

func TestHistoryPushState(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com/page1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		history.pushState({page: 2}, '', '/page2');
		var len = history.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 2 {
		t.Errorf("history length = %v, want 2", val.Num)
	}
}

func TestHistoryReplaceState(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com/page1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		history.replaceState({page: 1}, '', '/page1-replaced');
		var len = history.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 1 {
		t.Errorf("history length = %v, want 1 (replaceState doesn't add)", val.Num)
	}
}

func TestHistoryState(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com/page1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		history.pushState({data: 'test'}, '', '/page2');
		var state = history.state;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("state")
	if !ok {
		t.Fatal("state not found")
	}
	if val.Kind != js.KindObject {
		t.Errorf("state kind = %v, want Object", val.Kind)
	}
}

func TestHistoryBackForward(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com/page1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		history.pushState({page: 2}, '', '/page2');
		history.pushState({page: 3}, '', '/page3');
		history.back();
		history.back();
		// Should be back at page1 now.
		var len = history.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 3 {
		t.Errorf("history length = %v, want 3", val.Num)
	}
}

func TestLocalStorageOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.localStorage.setItem('key', 'value');
		var result = window.localStorage.getItem('key');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("result")
	if !ok {
		t.Fatal("result not found")
	}
	if val.Str != "value" {
		t.Errorf("result = %q, want %q", val.Str, "value")
	}
}

func TestHistoryOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com/"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var len = window.history.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 1 {
		t.Errorf("history length = %v, want 1", val.Num)
	}
}

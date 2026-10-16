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
		var len = localStorage.length;
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
		var len = localStorage.length;
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

// TestLocalStorageLengthIsProperty pins Web Storage conformance: length is
// a live numeric property, not a callable. Pages read it as a number.
func TestLocalStorageLengthIsProperty(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		localStorage.setItem('a', '1');
		localStorage.setItem('b', '2');
		var isNum = (typeof localStorage.length === 'number');
		var len = localStorage.length;
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("isNum"); !ok || !v.Bool {
		t.Fatal("localStorage.length is not a number")
	}
	if v, ok := r.Global("len"); !ok || v.Num != 2 {
		t.Fatalf("localStorage.length = %v, want 2", v)
	}
}

// TestLocalStorageQuotaEnforced pins the 5 MiB admission bound: writes past
// it throw a QuotaExceededError-named failure, keep prior contents, and do
// not grow the store.
func TestLocalStorageQuotaEnforced(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var MB = new Array(900*1024+1).join('x');
		var caught = '';
		for (var i = 0; i < 5; i++) { localStorage.setItem('k'+i, MB); }
		try { localStorage.setItem('over', MB); } catch (e) { caught = e && e.name || String(e); }
		var len = localStorage.length;
		var kept = localStorage.getItem('k0') === MB;
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := r.Global("caught"); v.Str != "QuotaExceededError" {
		t.Fatalf("over-quota error name = %q, want QuotaExceededError", v.Str)
	}
	if v, _ := r.Global("len"); v.Num != 5 {
		t.Fatalf("length after refused write = %v, want 5 (no growth)", v.Num)
	}
	if v, _ := r.Global("kept"); !v.Bool {
		t.Fatal("prior contents disturbed by a refused write")
	}
}

// TestStorageQuotaIsPerStore: one store hitting quota must not spend the
// other's budget.
func TestStorageQuotaIsPerStore(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var MB = new Array(900*1024+1).join('x');
		for (var i = 0; i < 5; i++) { localStorage.setItem('k'+i, MB); }
		var sessionOK = true;
		try { sessionStorage.setItem('s', MB); } catch (e) { sessionOK = false; }
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := r.Global("sessionOK"); !v.Bool {
		t.Fatal("sessionStorage refused while only localStorage is full: budgets must be per store")
	}
}

// TestStorageRemoveFreesQuota: deleting frees budget for new writes.
func TestStorageRemoveFreesQuota(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`
		var MB = new Array(900*1024+1).join('x');
		for (var i = 0; i < 5; i++) { localStorage.setItem('k'+i, MB); }
		localStorage.removeItem('k0');
		var ok = true;
		try { localStorage.setItem('fresh', MB); } catch (e) { ok = false; }
		var len = localStorage.length;
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, _ := r.Global("ok"); !v.Bool {
		t.Fatal("write after removeItem refused: deletion must free budget")
	}
	if v, _ := r.Global("len"); v.Num != 5 {
		t.Fatalf("length = %v, want 5 after replace", v.Num)
	}
}

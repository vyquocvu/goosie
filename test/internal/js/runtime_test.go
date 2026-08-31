package js_test

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	js "github.com/vyquocvu/goosie/internal/js"
)

type memoryStorageAdapter struct {
	values map[string]map[string]string
}

func newMemoryStorageAdapter() *memoryStorageAdapter {
	return &memoryStorageAdapter{values: map[string]map[string]string{}}
}

func (s *memoryStorageAdapter) Get(origin, key string) (string, bool) {
	if s.values[origin] == nil {
		return "", false
	}
	value, ok := s.values[origin][key]
	return value, ok
}

func (s *memoryStorageAdapter) Set(origin, key, value string) error {
	if s.values[origin] == nil {
		s.values[origin] = map[string]string{}
	}
	s.values[origin][key] = value
	return nil
}

func (s *memoryStorageAdapter) Remove(origin, key string) error {
	delete(s.values[origin], key)
	return nil
}

func (s *memoryStorageAdapter) Clear(origin string) error {
	delete(s.values, origin)
	return nil
}

func (s *memoryStorageAdapter) Keys(origin string) []string {
	keys := make([]string, 0, len(s.values[origin]))
	for key := range s.values[origin] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestLocalStorageUsesOriginScopedAdapter(t *testing.T) {
	store := newMemoryStorageAdapter()

	first := js.NewRuntime()
	first.SetOrigin("https://one.test")
	first.SetLocalStorageAdapter(store)
	_, err := first.RunScript(`localStorage.setItem("theme", "dark")`)
	require.NoError(t, err)

	second := js.NewRuntime()
	second.SetOrigin("https://one.test")
	second.SetLocalStorageAdapter(store)
	value, err := second.RunScript(`localStorage.getItem("theme")`)
	require.NoError(t, err)
	require.Equal(t, "dark", value.String())

	third := js.NewRuntime()
	third.SetOrigin("https://two.test")
	third.SetLocalStorageAdapter(store)
	missing, err := third.RunScript(`localStorage.getItem("theme")`)
	require.NoError(t, err)
	require.Equal(t, "null", missing.String())
}

func TestNewRuntime(t *testing.T) {
	runtime := js.NewRuntime()
	if runtime == nil {
		t.Fatal("NewRuntime() returned nil")
	}
	if runtime.VM() == nil {
		t.Fatal("Runtime vm is nil")
	}
}

func TestConsoleLog(t *testing.T) {
	runtime := js.NewRuntime()

	_, err := runtime.RunScript(`console.log("test message");`)
	if err != nil {
		t.Errorf("console.log failed: %v", err)
	}
}

func TestDocumentGetElementByID(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body><div id="test">Test Content</div></body></html>`
	runtime.SetHTMLContent(html)

	// Test getting non-existent element
	val, err := runtime.RunScript(`document.getElementById("nonexistent");`)
	if err != nil {
		t.Errorf("getElementById failed: %v", err)
	}
	if !val.ToBoolean() {
		// Should be null/falsy for non-existent element
		t.Log("Correctly returned null for non-existent element")
	}

	// Test getting existing element
	val, err = runtime.RunScript(`
		var elem = document.getElementById("test");
		elem ? elem.textContent : null;
	`)
	if err != nil {
		t.Errorf("getElementById failed: %v", err)
	}
	if val.Export() != nil {
		result := val.String()
		if !strings.Contains(result, "Test Content") {
			t.Errorf("Expected textContent to contain 'Test Content', got %v", result)
		}
	}
}

func TestSetHTMLContent(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body>Test</body></html>`
	runtime.SetHTMLContent(html)

	if runtime.HTMLCache() != html {
		t.Errorf("SetHTMLContent() did not set htmlCache correctly")
	}
}

func TestRunScript(t *testing.T) {
	runtime := js.NewRuntime()

	tests := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{
			name:    "valid script",
			script:  `var x = 1 + 1;`,
			wantErr: false,
		},
		{
			name:    "console.log",
			script:  `console.log("test");`,
			wantErr: false,
		},
		{
			name:    "invalid syntax",
			script:  `var x = ;`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runtime.RunScript(tt.script)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunScript() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetElementsByClassName(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body>
		<div class="item">Item 1</div>
		<p class="item">Item 2</p>
		<span class="other">Other</span>
	</body></html>`
	runtime.SetHTMLContent(html)

	val, err := runtime.RunScript(`
		var elements = document.getElementsByClassName("item");
		elements.length;
	`)
	if err != nil {
		t.Errorf("getElementsByClassName failed: %v", err)
	}
	if val.ToInteger() != 2 {
		t.Errorf("Expected 2 elements, got %d", val.ToInteger())
	}
}

func TestGetElementsByTagName(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body>
		<div>Div 1</div>
		<div>Div 2</div>
		<p>Paragraph</p>
	</body></html>`
	runtime.SetHTMLContent(html)

	val, err := runtime.RunScript(`
		var elements = document.getElementsByTagName("div");
		elements.length;
	`)
	if err != nil {
		t.Errorf("getElementsByTagName failed: %v", err)
	}
	if val.ToInteger() != 2 {
		t.Errorf("Expected 2 elements, got %d", val.ToInteger())
	}
}

func TestQuerySelector(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body>
		<div id="main" class="container">Main Content</div>
		<p class="text">Paragraph</p>
	</body></html>`
	runtime.SetHTMLContent(html)

	tests := []struct {
		name     string
		script   string
		wantNull bool
	}{
		{
			name:     "ID selector",
			script:   `document.querySelector("#main")`,
			wantNull: false,
		},
		{
			name:     "class selector",
			script:   `document.querySelector(".text")`,
			wantNull: false,
		},
		{
			name:     "tag selector",
			script:   `document.querySelector("p")`,
			wantNull: false,
		},
		{
			name:     "non-matching selector",
			script:   `document.querySelector("#nonexistent")`,
			wantNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := runtime.RunScript(tt.script)
			if err != nil {
				t.Errorf("querySelector failed: %v", err)
			}
			isNull := val == nil || val.String() == "null"
			if isNull != tt.wantNull {
				t.Errorf("querySelector returned null=%v, want null=%v", isNull, tt.wantNull)
			}
		})
	}
}

func TestQuerySelectorAll(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body>
		<div class="item">Item 1</div>
		<div class="item">Item 2</div>
		<p class="item">Item 3</p>
	</body></html>`
	runtime.SetHTMLContent(html)

	val, err := runtime.RunScript(`
		var elements = document.querySelectorAll(".item");
		elements.length;
	`)
	if err != nil {
		t.Errorf("querySelectorAll failed: %v", err)
	}
	if val.ToInteger() != 3 {
		t.Errorf("Expected 3 elements, got %d", val.ToInteger())
	}
}

func TestElementQuerySelector(t *testing.T) {
	runtime := js.NewRuntime()
	html := `<html><body>
		<div id="container">
			<span class="highlight">Hello</span>
			<a href="https://example.com" class="link">Link</a>
		</div>
	</body></html>`
	runtime.SetHTMLContent(html)

	val, err := runtime.RunScript(`
		var container = document.getElementById("container");
		var span = container.querySelector(".highlight");
		span ? span.textContent : "null";
	`)
	if err != nil {
		t.Fatalf("container.querySelector failed: %v", err)
	}
	if val.String() != "Hello" {
		t.Errorf("Expected 'Hello', got %s", val.String())
	}

	valAll, err := runtime.RunScript(`
		var container = document.getElementById("container");
		var links = container.querySelectorAll(".link");
		links.length;
	`)
	if err != nil {
		t.Fatalf("container.querySelectorAll failed: %v", err)
	}
	if valAll.ToInteger() != 1 {
		t.Errorf("Expected 1 link, got %d", valAll.ToInteger())
	}
}

func TestCryptoAPI(t *testing.T) {
	runtime := js.NewRuntime()
	val, err := runtime.RunScript(`
		typeof window.crypto.getRandomValues === "function" &&
		typeof crypto.randomUUID === "function" &&
		crypto.randomUUID().length === 36;
	`)
	if err != nil {
		t.Fatalf("crypto test failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("Expected crypto APIs to be available and valid")
	}

	valRand, err := runtime.RunScript(`
		var arr = new Uint8Array(8);
		crypto.getRandomValues(arr);
		arr.length === 8;
	`)
	if err != nil {
		t.Fatalf("crypto.getRandomValues failed: %v", err)
	}
	if !valRand.ToBoolean() {
		t.Errorf("Expected getRandomValues to succeed on Uint8Array")
	}
}

func TestGlobalLocation(t *testing.T) {
	runtime := js.NewRuntime()
	val, err := runtime.RunScript(`typeof location !== "undefined" && location.href === window.location.href`)
	if err != nil {
		t.Fatalf("global location test failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("Expected global location to match window.location")
	}
}

func TestCreateElement(t *testing.T) {
	runtime := js.NewRuntime()

	val, err := runtime.RunScript(`
		var div = document.createElement("div");
		div.tagName;
	`)
	if err != nil {
		t.Errorf("createElement failed: %v", err)
	}
	if val.String() != "div" {
		t.Errorf("Expected tagName 'div', got %s", val.String())
	}
}

func TestAppendChild(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body><div id="parent">Parent</div></body></html>`
	runtime.SetHTMLContent(html)

	val, err := runtime.RunScript(`
		var parent = document.getElementById("parent");
		var child = document.createElement("span");
		child.textContent = "Child";
		parent.appendChild(child);
		parent.children.length;
	`)
	if err != nil {
		t.Errorf("appendChild failed: %v", err)
	}
	if val.ToInteger() != 1 {
		t.Errorf("Expected 1 child, got %d", val.ToInteger())
	}
}

// TestRuntimeRemoveChild exercises the JavaScript `removeChild`
// API exposed on DOM element objects. The JS binding manipulates
// a Go slice of children on the element object; this test
// verifies the binding wires createElement / appendChild /
// removeChild / children.length correctly across the JS↔Go
// boundary.
func TestRuntimeRemoveChild(t *testing.T) {
	runtime := js.NewRuntime()

	val, err := runtime.RunScript(`
		var parent = document.createElement("div");
		var child1 = document.createElement("span");
		var child2 = document.createElement("p");
		parent.appendChild(child1);
		parent.appendChild(child2);
		parent.removeChild(child1);
		parent.children.length;
	`)
	if err != nil {
		t.Errorf("removeChild failed: %v", err)
	}
	if val.ToInteger() != 1 {
		t.Errorf("Expected 1 child after removal, got %d", val.ToInteger())
	}
}

func TestReplaceChild(t *testing.T) {
	runtime := js.NewRuntime()

	val, err := runtime.RunScript(`
		var parent = document.createElement("div");
		var oldChild = document.createElement("span");
		var newChild = document.createElement("p");
		oldChild.textContent = "Old";
		newChild.textContent = "New";
		parent.appendChild(oldChild);
		parent.replaceChild(newChild, oldChild);
		parent.children[0].textContent;
	`)
	if err != nil {
		t.Errorf("replaceChild failed: %v", err)
	}
	if val.String() != "New" {
		t.Errorf("Expected 'New', got %s", val.String())
	}
}

func TestInsertBefore(t *testing.T) {
	runtime := js.NewRuntime()

	val, err := runtime.RunScript(`
		var parent = document.createElement("div");
		var child1 = document.createElement("span");
		var child2 = document.createElement("p");
		child1.textContent = "First";
		child2.textContent = "Second";
		parent.appendChild(child2);
		parent.insertBefore(child1, child2);
		parent.children[0].textContent;
	`)
	if err != nil {
		t.Errorf("insertBefore failed: %v", err)
	}
	if val.String() != "First" {
		t.Errorf("Expected 'First' at index 0, got %s", val.String())
	}
}

func TestAddEventListener(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body><button id="btn">Click</button></body></html>`
	runtime.SetHTMLContent(html)

	_, err := runtime.RunScript(`
		var btn = document.getElementById("btn");
		var clicked = false;
		btn.addEventListener("click", function() {
			clicked = true;
		});
	`)
	if err != nil {
		t.Errorf("addEventListener failed: %v", err)
	}
}

func TestRemoveEventListener(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body><button id="btn">Click</button></body></html>`
	runtime.SetHTMLContent(html)

	_, err := runtime.RunScript(`
		var btn = document.getElementById("btn");
		var handler = function() {
			console.log("clicked");
		};
		btn.addEventListener("click", handler);
		btn.removeEventListener("click", handler);
	`)
	if err != nil {
		t.Errorf("removeEventListener failed: %v", err)
	}
}

func TestElementProperties(t *testing.T) {
	runtime := js.NewRuntime()

	html := `<html><body>
		<div id="test" class="container active">Test Content</div>
	</body></html>`
	runtime.SetHTMLContent(html)

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "tagName",
			script: `document.querySelector("#test").tagName`,
			want:   "div",
		},
		{
			name:   "id",
			script: `document.querySelector("#test").id`,
			want:   "test",
		},
		{
			name:   "textContent",
			script: `document.querySelector("#test").textContent`,
			want:   "Test Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := runtime.RunScript(tt.script)
			if err != nil {
				t.Errorf("Failed to get property: %v", err)
			}
			got := val.String()
			if !strings.Contains(got, tt.want) {
				t.Errorf("Expected to contain %q, got %q", tt.want, got)
			}
		})
	}
}

// Test Browser APIs

func TestWindowLocation(t *testing.T) {
	runtime := js.NewRuntime()

	// Test setting URL
	_, err := runtime.RunScript(`
window.location.setURL("https://example.com:8080/path/to/page?key=value&foo=bar#section");
`)
	if err != nil {
		t.Errorf("setURL failed: %v", err)
	}

	// Test protocol
	val, err := runtime.RunScript(`window.location.protocol`)
	if err != nil {
		t.Errorf("protocol failed: %v", err)
	}
	if val.String() != "https:" {
		t.Errorf("Expected protocol 'https:', got %s", val.String())
	}

	// Test hostname
	val, err = runtime.RunScript(`window.location.hostname`)
	if err != nil {
		t.Errorf("hostname failed: %v", err)
	}
	if val.String() != "example.com" {
		t.Errorf("Expected hostname 'example.com', got %s", val.String())
	}

	// Test pathname
	val, err = runtime.RunScript(`window.location.pathname`)
	if err != nil {
		t.Errorf("pathname failed: %v", err)
	}
	if val.String() != "/path/to/page" {
		t.Errorf("Expected pathname '/path/to/page', got %s", val.String())
	}
}

func TestLocationQueryParams(t *testing.T) {
	runtime := js.NewRuntime()

	// Set URL with query params
	runtime.RunScript(`window.location.setURL("https://example.com?name=John&age=30");`)

	// Test getQueryParam
	val, err := runtime.RunScript(`window.location.getQueryParam("name")`)
	if err != nil {
		t.Errorf("getQueryParam failed: %v", err)
	}
	if val.String() != "John" {
		t.Errorf("Expected 'John', got %s", val.String())
	}

	// Test setQueryParam
	val, err = runtime.RunScript(`window.location.setQueryParam("city", "NYC")`)
	if err != nil {
		t.Errorf("setQueryParam failed: %v", err)
	}

	// Verify new param was added
	val, err = runtime.RunScript(`window.location.getQueryParam("city")`)
	if err != nil {
		t.Errorf("getQueryParam for new param failed: %v", err)
	}
	if val.String() != "NYC" {
		t.Errorf("Expected 'NYC', got %s", val.String())
	}
}

func TestWindowHistory(t *testing.T) {
	runtime := js.NewRuntime()

	// Test pushState
	_, err := runtime.RunScript(`
window.history.pushState({}, "Page 1", "/page1");
window.history.pushState({}, "Page 2", "/page2");
window.history.pushState({}, "Page 3", "/page3");
`)
	if err != nil {
		t.Errorf("pushState failed: %v", err)
	}

	// Test length
	val, err := runtime.RunScript(`window.history.length()`)
	if err != nil {
		t.Errorf("history.length failed: %v", err)
	}
	if val.ToInteger() != 3 {
		t.Errorf("Expected history length 3, got %d", val.ToInteger())
	}

	// Test back
	_, err = runtime.RunScript(`window.history.back()`)
	if err != nil {
		t.Errorf("history.back failed: %v", err)
	}

	// Test forward
	_, err = runtime.RunScript(`window.history.forward()`)
	if err != nil {
		t.Errorf("history.forward failed: %v", err)
	}

	// Test go
	_, err = runtime.RunScript(`window.history.go(-1)`)
	if err != nil {
		t.Errorf("history.go failed: %v", err)
	}
}

func TestHistoryReplaceState(t *testing.T) {
	runtime := js.NewRuntime()

	// Push initial state
	runtime.RunScript(`window.history.pushState({}, "Page 1", "/page1")`)

	// Replace current state
	_, err := runtime.RunScript(`window.history.replaceState({}, "Page 1 Updated", "/page1-updated")`)
	if err != nil {
		t.Errorf("replaceState failed: %v", err)
	}

	// History length should remain 1
	val, _ := runtime.RunScript(`window.history.length()`)
	if val.ToInteger() != 1 {
		t.Errorf("Expected history length 1 after replaceState, got %d", val.ToInteger())
	}
}

func TestSetTimeout(t *testing.T) {
	runtime := js.NewRuntime()
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var executed = false;
var timerId = setTimeout(function() {
executed = true;
}, 10);
`)
	mu.Unlock()
	if err != nil {
		t.Errorf("setTimeout failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`executed`)
	mu.Unlock()
	if err != nil {
		t.Errorf("Failed to check executed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("setTimeout callback was not executed")
	}
}

func TestWindowTimerAliasesAndFrameGlobals(t *testing.T) {
	runtime := js.NewRuntime()
	defer runtime.Cleanup()
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var windowExecuted = false;
var timerId = window.setTimeout(function() {
windowExecuted = true;
}, 10);
`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("window.setTimeout failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`windowExecuted && top === window && parent === window && self === window`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("failed to check browser globals: %v", err)
	}
	if !val.ToBoolean() {
		t.Fatalf("expected window timer aliases and frame globals to be available")
	}
}

func TestClearTimeout(t *testing.T) {
	runtime := js.NewRuntime()
	defer runtime.Cleanup()
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var executed = false;
var timerId = setTimeout(function() {
executed = true;
}, 10);
clearTimeout(timerId);
`)
	mu.Unlock()
	if err != nil {
		t.Errorf("clearTimeout failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`executed`)
	mu.Unlock()
	if err != nil {
		t.Errorf("Failed to check executed: %v", err)
	}
	if val.ToBoolean() {
		t.Errorf("setTimeout callback should not have been executed after clearTimeout")
	}
}

func TestSetInterval(t *testing.T) {
	runtime := js.NewRuntime()
	defer runtime.Cleanup()
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var counter = 0;
var intervalId = setInterval(function() {
counter++;
}, 10);
`)
	mu.Unlock()
	if err != nil {
		t.Errorf("setInterval failed: %v", err)
	}

	time.Sleep(55 * time.Millisecond)

	mu.Lock()
	runtime.RunScript(`clearInterval(intervalId)`)
	val, err := runtime.RunScript(`counter`)
	mu.Unlock()
	if err != nil {
		t.Errorf("Failed to check counter: %v", err)
	}

	counter := val.ToInteger()
	if counter < 2 {
		t.Errorf("Expected counter >= 2, got %d", counter)
	}
}

func TestClearInterval(t *testing.T) {
	runtime := js.NewRuntime()
	defer runtime.Cleanup()
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var counter = 0;
var intervalId = setInterval(function() {
counter++;
}, 10);
clearInterval(intervalId);
`)
	mu.Unlock()
	if err != nil {
		t.Errorf("clearInterval failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`counter`)
	mu.Unlock()
	if err != nil {
		t.Errorf("Failed to check counter: %v", err)
	}
	if val.ToInteger() != 0 {
		t.Errorf("Expected counter 0 after clearInterval, got %d", val.ToInteger())
	}
}

func TestLocalStorage(t *testing.T) {
	runtime := js.NewRuntime()

	// Test setItem
	_, err := runtime.RunScript(`localStorage.setItem("user", "John")`)
	if err != nil {
		t.Errorf("localStorage.setItem failed: %v", err)
	}

	// Test getItem
	val, err := runtime.RunScript(`localStorage.getItem("user")`)
	if err != nil {
		t.Errorf("localStorage.getItem failed: %v", err)
	}

	result := val.String()
	if !strings.Contains(result, "John") {
		t.Errorf("Expected localStorage value to contain 'John', got %s", result)
	}

	// Test length
	val, err = runtime.RunScript(`localStorage.length()`)
	if err != nil {
		t.Errorf("localStorage.length failed: %v", err)
	}
	if val.ToInteger() != 1 {
		t.Errorf("Expected localStorage length 1, got %d", val.ToInteger())
	}

	// Test removeItem
	_, err = runtime.RunScript(`localStorage.removeItem("user")`)
	if err != nil {
		t.Errorf("localStorage.removeItem failed: %v", err)
	}

	val, err = runtime.RunScript(`localStorage.getItem("user")`)
	if err != nil {
		t.Errorf("localStorage.getItem after remove failed: %v", err)
	}
	if val.String() != "null" {
		t.Errorf("Expected null after removeItem, got %s", val.String())
	}
}

func TestLocalStorageClear(t *testing.T) {
	runtime := js.NewRuntime()

	// Add multiple items
	runtime.RunScript(`
localStorage.setItem("key1", "value1");
localStorage.setItem("key2", "value2");
localStorage.setItem("key3", "value3");
`)

	// Test clear
	_, err := runtime.RunScript(`localStorage.clear()`)
	if err != nil {
		t.Errorf("localStorage.clear failed: %v", err)
	}

	// Check length is 0
	val, _ := runtime.RunScript(`localStorage.length()`)
	if val.ToInteger() != 0 {
		t.Errorf("Expected localStorage length 0 after clear, got %d", val.ToInteger())
	}
}

func TestSessionStorage(t *testing.T) {
	runtime := js.NewRuntime()

	// Test setItem
	_, err := runtime.RunScript(`sessionStorage.setItem("sessionKey", "sessionValue")`)
	if err != nil {
		t.Errorf("sessionStorage.setItem failed: %v", err)
	}

	// Test getItem
	val, err := runtime.RunScript(`sessionStorage.getItem("sessionKey")`)
	if err != nil {
		t.Errorf("sessionStorage.getItem failed: %v", err)
	}

	result := val.String()
	if !strings.Contains(result, "sessionValue") {
		t.Errorf("Expected sessionStorage value to contain 'sessionValue', got %s", result)
	}

	// Test removeItem
	_, err = runtime.RunScript(`sessionStorage.removeItem("sessionKey")`)
	if err != nil {
		t.Errorf("sessionStorage.removeItem failed: %v", err)
	}

	val, err = runtime.RunScript(`sessionStorage.getItem("sessionKey")`)
	if err != nil {
		t.Errorf("sessionStorage.getItem after remove failed: %v", err)
	}
	if val.String() != "null" {
		t.Errorf("Expected null after removeItem, got %s", val.String())
	}
}

func TestSessionStorageKey(t *testing.T) {
	runtime := js.NewRuntime()

	// Add items
	runtime.RunScript(`
sessionStorage.setItem("key1", "value1");
sessionStorage.setItem("key2", "value2");
`)

	// Test key method
	val, err := runtime.RunScript(`sessionStorage.key(0)`)
	if err != nil {
		t.Errorf("sessionStorage.key failed: %v", err)
	}

	// Should return one of the keys
	key := val.String()
	if key != "key1" && key != "key2" {
		t.Errorf("Expected key1 or key2, got %s", key)
	}
}

func TestFetchAPI(t *testing.T) {
	runtime := js.NewRuntime()
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var fetchCalled = false;
fetch("https://api.example.com/data")
.then(function(response) {
fetchCalled = true;
});
`)
	mu.Unlock()
	if err != nil {
		t.Errorf("fetch failed: %v", err)
	}

	// Give time for async operation
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`fetchCalled`)
	mu.Unlock()
	if err != nil {
		t.Errorf("Failed to check fetchCalled: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("fetch callback was not executed")
	}
}

func TestRuntimeCleanup(t *testing.T) {
	runtime := js.NewRuntime()

	// Create some timers
	runtime.RunScript(`
setTimeout(function() {}, 1000);
setInterval(function() {}, 1000);
`)

	if len(runtime.Timers()) != 2 {
		t.Errorf("Expected 2 timers, got %d", len(runtime.Timers()))
	}

	// Cleanup
	runtime.Cleanup()

	if len(runtime.Timers()) != 0 {
		t.Errorf("Expected 0 timers after cleanup, got %d", len(runtime.Timers()))
	}
}

func TestLocationSearchAndHashPrefixes(t *testing.T) {
	runtime := js.NewRuntime()

	// Set URL with query and hash
	runtime.RunScript(`window.location.setURL("https://example.com/path?key=value#section");`)

	// Test search includes '?' prefix
	val, err := runtime.RunScript(`window.location.search`)
	if err != nil {
		t.Errorf("search failed: %v", err)
	}
	if val.String() != "?key=value" {
		t.Errorf("Expected search '?key=value', got %s", val.String())
	}

	// Test hash includes '#' prefix
	val, err = runtime.RunScript(`window.location.hash`)
	if err != nil {
		t.Errorf("hash failed: %v", err)
	}
	if val.String() != "#section" {
		t.Errorf("Expected hash '#section', got %s", val.String())
	}

	// Test empty query and hash
	runtime.RunScript(`window.location.setURL("https://example.com/path");`)

	val, _ = runtime.RunScript(`window.location.search`)
	if val.String() != "" {
		t.Errorf("Expected empty search, got %s", val.String())
	}

	val, _ = runtime.RunScript(`window.location.hash`)
	if val.String() != "" {
		t.Errorf("Expected empty hash, got %s", val.String())
	}
}

func TestClearTimeoutMultipleTimes(t *testing.T) {
	runtime := js.NewRuntime()
	defer runtime.Cleanup()

	// Create a timer and clear it multiple times
	_, err := runtime.RunScript(`
var timerId = setTimeout(function() {}, 1000);
clearTimeout(timerId);
clearTimeout(timerId); // Should not panic
clearTimeout(timerId); // Should not panic
`)
	if err != nil {
		t.Errorf("clearTimeout multiple times failed: %v", err)
	}
}

func TestClearIntervalMultipleTimes(t *testing.T) {
	runtime := js.NewRuntime()
	defer runtime.Cleanup()

	// Create an interval and clear it multiple times
	_, err := runtime.RunScript(`
var intervalId = setInterval(function() {}, 1000);
clearInterval(intervalId);
clearInterval(intervalId); // Should not panic
clearInterval(intervalId); // Should not panic
`)
	if err != nil {
		t.Errorf("clearInterval multiple times failed: %v", err)
	}
}

func TestConsoleError(t *testing.T) {
	runtime := js.NewRuntime()

	_, err := runtime.RunScript(`console.error("test error");`)
	if err != nil {
		t.Errorf("console.error failed: %v", err)
	}

	messages := runtime.GetConsoleMessages()
	if len(messages) == 0 {
		t.Errorf("Expected console message to be logged")
	}

	if messages[0].Level != "error" {
		t.Errorf("Expected level to be 'error', got %s", messages[0].Level)
	}

	if messages[0].Message != "test error" {
		t.Errorf("Expected message 'test error', got %s", messages[0].Message)
	}
}

func TestConsoleWarn(t *testing.T) {
	runtime := js.NewRuntime()

	_, err := runtime.RunScript(`console.warn("test warning");`)
	if err != nil {
		t.Errorf("console.warn failed: %v", err)
	}

	messages := runtime.GetConsoleMessages()
	if len(messages) == 0 {
		t.Errorf("Expected console message to be logged")
	}

	if messages[0].Level != "warn" {
		t.Errorf("Expected level to be 'warn', got %s", messages[0].Level)
	}
}

func TestConsoleInfo(t *testing.T) {
	runtime := js.NewRuntime()

	_, err := runtime.RunScript(`console.info("test info");`)
	if err != nil {
		t.Errorf("console.info failed: %v", err)
	}

	messages := runtime.GetConsoleMessages()
	if len(messages) == 0 {
		t.Errorf("Expected console message to be logged")
	}

	if messages[0].Level != "info" {
		t.Errorf("Expected level to be 'info', got %s", messages[0].Level)
	}
}

func TestConsoleTable(t *testing.T) {
	runtime := js.NewRuntime()

	// Test with an array
	_, err := runtime.RunScript(`console.table([1, 2, 3, 4, 5]);`)
	if err != nil {
		t.Errorf("console.table with array failed: %v", err)
	}

	messages := runtime.GetConsoleMessages()
	if len(messages) == 0 {
		t.Errorf("Expected console message to be logged")
	}

	if messages[0].Level != "table" {
		t.Errorf("Expected level to be 'table', got %s", messages[0].Level)
	}

	// Test with an object
	runtime.ClearConsoleMessages()
	_, err = runtime.RunScript(`console.table({name: "John", age: 30, city: "New York"});`)
	if err != nil {
		t.Errorf("console.table with object failed: %v", err)
	}

	messages = runtime.GetConsoleMessages()
	if len(messages) == 0 {
		t.Errorf("Expected console message to be logged")
	}

	if messages[0].Level != "table" {
		t.Errorf("Expected level to be 'table', got %s", messages[0].Level)
	}
}

func TestGetConsoleMessages(t *testing.T) {
	runtime := js.NewRuntime()

	// Log multiple messages
	runtime.RunScript(`console.log("message 1");`)
	runtime.RunScript(`console.error("message 2");`)
	runtime.RunScript(`console.warn("message 3");`)

	messages := runtime.GetConsoleMessages()

	if len(messages) != 3 {
		t.Errorf("Expected 3 console messages, got %d", len(messages))
	}

	if messages[0].Level != "log" || messages[0].Message != "message 1" {
		t.Errorf("First message incorrect: %v", messages[0])
	}

	if messages[1].Level != "error" || messages[1].Message != "message 2" {
		t.Errorf("Second message incorrect: %v", messages[1])
	}

	if messages[2].Level != "warn" || messages[2].Message != "message 3" {
		t.Errorf("Third message incorrect: %v", messages[2])
	}
}

func TestClearConsoleMessages(t *testing.T) {
	runtime := js.NewRuntime()

	runtime.RunScript(`console.log("message 1");`)
	runtime.RunScript(`console.log("message 2");`)

	messages := runtime.GetConsoleMessages()
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages before clear, got %d", len(messages))
	}

	runtime.ClearConsoleMessages()

	messages = runtime.GetConsoleMessages()
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(messages))
	}
}

func TestJavaScriptErrorTracking(t *testing.T) {
	runtime := js.NewRuntime()

	// Execute invalid JavaScript
	_, err := runtime.RunScript(`var x = ;`)
	if err == nil {
		t.Errorf("Expected error for invalid JavaScript")
	}

	errors := runtime.GetJavaScriptErrors()
	if len(errors) == 0 {
		t.Errorf("Expected JavaScript error to be tracked")
	}

	// Check that error was also logged to console
	messages := runtime.GetConsoleMessages()
	foundError := false
	for _, msg := range messages {
		if msg.Level == "error" && strings.Contains(msg.Message, "JavaScript Error") {
			foundError = true
			break
		}
	}

	if !foundError {
		t.Errorf("Expected JavaScript error to be logged to console")
	}
}

func TestClearJavaScriptErrors(t *testing.T) {
	runtime := js.NewRuntime()

	// Generate an error
	runtime.RunScript(`var x = ;`)

	errors := runtime.GetJavaScriptErrors()
	if len(errors) == 0 {
		t.Errorf("Expected error to be tracked")
	}

	runtime.ClearJavaScriptErrors()

	errors = runtime.GetJavaScriptErrors()
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors after clear, got %d", len(errors))
	}
}

// mockFetcher implements HTTPFetcher for testing.
type mockFetcher struct {
	body string
	err  error
}

func (m *mockFetcher) Fetch(url string) (string, error) {
	return m.body, m.err
}

func TestSetFetcher(t *testing.T) {
	runtime := js.NewRuntime()
	mock := &mockFetcher{body: "hello world"}
	runtime.SetFetcher(mock)
	if runtime.Fetcher() != mock {
		t.Errorf("SetFetcher did not set the fetcher")
	}
}

func TestFetchAPIWithRealFetcher_Text(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetFetcher(&mockFetcher{body: "hello from server"})
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var textResult = "";
fetch("https://example.com/api")
  .then(function(response) {
    response.text().then(function(text) {
      textResult = text;
    });
  });
`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("fetch script failed: %v", err)
	}

	// Allow goroutine to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`textResult`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("reading textResult failed: %v", err)
	}
	if val.String() != "hello from server" {
		t.Errorf("expected 'hello from server', got %q", val.String())
	}
}

func TestFetchAPIWithRealFetcher_JSON(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetFetcher(&mockFetcher{body: `{"name":"goosie","version":1}`})
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var jsonResult = null;
fetch("https://example.com/api")
  .then(function(response) {
    response.json().then(function(data) {
      jsonResult = data;
    });
  });
`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("fetch script failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`jsonResult !== null && jsonResult.name === "goosie"`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("reading jsonResult failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected jsonResult.name to be 'goosie'")
	}
}

func TestFetchAPIWithRealFetcher_ErrorPropagation(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetFetcher(&mockFetcher{err: fmt.Errorf("connection refused")})
	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
var catchCalled = false;
var catchMsg = "";
fetch("https://example.com/api")
  .then(function(response) {})
  .catch(function(err) {
    catchCalled = true;
    catchMsg = err.message;
  });
`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("fetch script failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`catchCalled`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("reading catchCalled failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected catch callback to be called on fetch error")
	}

	mu.Lock()
	val, err = runtime.RunScript(`catchMsg`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("reading catchMsg failed: %v", err)
	}
	if val.String() != "connection refused" {
		t.Errorf("expected catchMsg to be 'connection refused', got %q", val.String())
	}
}

// --- File access policy tests ---

func TestFetchFileAccessFromRemoteOriginBlocked(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetOrigin("https://example.com")
	runtime.SetFetcher(&mockFetcher{body: "should not be called"})

	// The rejection is synchronous (before goroutine), so no enqueueTask needed.
	val, err := runtime.RunScript(`
		var result = "";
		var errMsg = "";
		fetch("file:///etc/passwd").then(function(res) {
			result = "unexpected_success";
		}).catch(function(err) {
			errMsg = err.message;
		});
		result + "|" + errMsg;
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// The catch callback should fire synchronously and set errMsg.
	want := "|fetch: local file access denied from remote origin"
	if val.String() != want {
		t.Fatalf("got %q, want %q", val.String(), want)
	}
}

func TestCheckFileFetchAccess_EmptyOrigin(t *testing.T) {
	err := js.CheckFileFetchAccess("", "file:///etc/passwd")
	if err != nil {
		t.Fatalf("js.CheckFileFetchAccess with empty origin should return nil, got: %v", err)
	}
}

func TestCheckFileFetchAccess_RemoteOrigin(t *testing.T) {
	err := js.CheckFileFetchAccess("https://example.com", "file:///etc/passwd")
	if err == nil {
		t.Fatal("js.CheckFileFetchAccess with remote origin should return error")
	}
	if err.Error() != "fetch: local file access denied from remote origin" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCheckFileFetchAccess_FileOrigin(t *testing.T) {
	err := js.CheckFileFetchAccess("file:///home/page.html", "file:///etc/passwd")
	if err != nil {
		t.Fatalf("js.CheckFileFetchAccess with file origin should return nil, got: %v", err)
	}
}

func TestCheckFileFetchAccess_NonFileTarget(t *testing.T) {
	err := js.CheckFileFetchAccess("https://example.com", "https://api.example.com/data")
	if err != nil {
		t.Fatalf("js.CheckFileFetchAccess with non-file target should return nil, got: %v", err)
	}
}

func TestFetchNonFileURLFromRemoteOrigin(t *testing.T) {
	// Non-file URLs must not be blocked.
	runtime := js.NewRuntime()
	runtime.SetOrigin("https://example.com")
	runtime.SetFetcher(&mockFetcher{body: `{"status":"ok"}`})

	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
		var result = "";
		fetch("https://api.example.com/data").then(function(res) {
			result = "success";
		}).catch(function(err) {
			result = "blocked";
		});
	`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`result`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("reading result failed: %v", err)
	}
	if val.String() != "success" {
		t.Fatalf("expected non-file fetch from remote origin to be allowed, got: %q", val.String())
	}
}

func TestWindowOpen_BlockedFromRemoteOrigin(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetOrigin("https://example.com")

	var opened bool
	runtime.OnOpenWindow = func(url, name string) {
		opened = true
	}

	val, err := runtime.RunScript(`window.open("https://evil.com/popup")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "null" && val.String() != "undefined" {
		t.Fatalf("expected null from remote origin window.open, got %v", val)
	}
	if opened {
		t.Fatal("OnOpenWindow should not be called for blocked popup from remote origin")
	}
}

func TestWindowOpen_AllowedFromFileOrigin(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetOrigin("file:///home/page.html")

	var openedURL, openedName string
	runtime.OnOpenWindow = func(url, name string) {
		openedURL = url
		openedName = name
	}

	_, err := runtime.RunScript(`window.open("https://example.com", "mywindow")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if openedURL != "https://example.com" {
		t.Fatalf("expected opened URL 'https://example.com', got %q", openedURL)
	}
	if openedName != "mywindow" {
		t.Fatalf("expected opened name 'mywindow', got %q", openedName)
	}
}

func TestWindowOpen_AllowedFromEmptyOrigin(t *testing.T) {
	runtime := js.NewRuntime()
	// No SetOrigin call — empty origin (pre-navigation)

	var opened bool
	runtime.OnOpenWindow = func(url, name string) {
		opened = true
	}

	_, err := runtime.RunScript(`window.open("https://example.com")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !opened {
		t.Fatal("OnOpenWindow should be called for popup from empty origin")
	}
}

func TestWindowOpen_ProxyObject(t *testing.T) {
	runtime := js.NewRuntime()

	runtime.OnOpenWindow = func(url, name string) {}

	val, err := runtime.RunScript(`window.open("https://example.com")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() == "null" || val.String() == "undefined" {
		t.Fatal("expected non-null window proxy")
	}
	// Verify via JS that proxy.closed is false.
	closed, err := runtime.RunScript(`window.open("https://example.com").closed`)
	if err != nil {
		t.Fatalf("reading proxy.closed failed: %v", err)
	}
	if closed.String() != "false" {
		t.Fatalf("expected proxy.closed to be false, got %v", closed)
	}
}

func TestWindowOpen_NoCallbackNoCrash(t *testing.T) {
	runtime := js.NewRuntime()
	runtime.SetOrigin("file:///home/page.html")
	// OnOpenWindow is nil — should not crash

	_, err := runtime.RunScript(`window.open("https://example.com")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Capability gating integration tests
// ---------------------------------------------------------------------------

func TestCapability_FetchDenied(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {js.CapabilityNetwork: js.APIDenied},
		},
	})
	runtime := js.NewRuntime()
	runtime.SetEnforcer(e)
	runtime.SetOrigin("https://example.com")
	runtime.SetFetcher(&mockFetcher{body: "data"})

	_, err := runtime.RunScript(`
		var caught = false;
		fetch("https://example.com/api").catch(function() { caught = true; });
		if (!caught) throw new Error("fetch should have been rejected");
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestCapability_FetchAllowed(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {js.CapabilityNetwork: js.APIAllowed},
		},
	})
	runtime := js.NewRuntime()
	runtime.SetEnforcer(e)
	runtime.SetOrigin("https://example.com")
	runtime.SetFetcher(&mockFetcher{body: `{"ok":true}`})

	var mu sync.Mutex
	runtime.SetEnqueueTask(func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	})

	mu.Lock()
	_, err := runtime.RunScript(`
		var result = "";
		fetch("https://example.com/api").then(function(r) { result = "ok"; }).catch(function() { result = "err"; });
	`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	val, err := runtime.RunScript(`result`)
	mu.Unlock()
	if err != nil {
		t.Fatalf("reading result failed: %v", err)
	}
	if val.String() != "ok" {
		t.Fatalf("expected fetch to be allowed, got %q", val.String())
	}
}

func TestCapability_StorageDenied(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {js.CapabilityStorage: js.APIDenied},
		},
	})
	runtime := js.NewRuntime()
	runtime.SetEnforcer(e)

	// All storage methods should return null/undefined when denied.
	check := func(script string) {
		v, err := runtime.RunScript(script)
		if err != nil {
			t.Fatalf("script %q failed: %v", script, err)
		}
		if v.String() != "null" && v.String() != "undefined" && v.String() != "0" {
			t.Fatalf("script %q expected null/undefined/0, got %q", script, v.String())
		}
	}
	check(`localStorage.getItem("x")`)
	check(`localStorage.setItem("x", "1")`)
	check(`localStorage.key(0)`)
	check(`localStorage.removeItem("x")`)
	check(`localStorage.clear()`)
	check(`sessionStorage.getItem("x")`)
	check(`sessionStorage.setItem("x", "1")`)
}

func TestCapability_StorageAllowed(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {js.CapabilityStorage: js.APIAllowed},
		},
	})
	runtime := js.NewRuntime()
	runtime.SetEnforcer(e)

	_, err := runtime.RunScript(`
		localStorage.setItem("key1", "value1");
		var v = localStorage.getItem("key1");
		if (v !== "value1") throw new Error("expected value1, got " + v);
		if (localStorage.length() < 1) throw new Error("length should be > 0");
		localStorage.clear();
	`)
	if err != nil {
		t.Fatalf("storage capability allowed test failed: %v", err)
	}
}

func TestCapability_NavigationDenied(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {js.CapabilityNavigation: js.APIDenied},
		},
	})
	runtime := js.NewRuntime()
	runtime.SetEnforcer(e)
	runtime.SetOrigin("file:///home/page.html")

	val, err := runtime.RunScript(`window.open("https://example.com")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "null" && val.String() != "undefined" {
		t.Fatalf("expected null when navigation capability denied, got %v", val)
	}
}

func TestCapability_NavigationAllowed(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {js.CapabilityNavigation: js.APIAllowed},
		},
	})
	runtime := js.NewRuntime()
	runtime.SetEnforcer(e)
	runtime.SetOrigin("file:///home/page.html")

	var called bool
	runtime.OnOpenWindow = func(url, name string) {
		called = true
	}

	_, err := runtime.RunScript(`window.open("https://example.com")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !called {
		t.Fatal("OnOpenWindow should be called when navigation allowed")
	}
}

func TestCapability_NoEnforcerAllowsAll(t *testing.T) {
	// When no ScriptEnforcer is set, all capabilities are allowed.
	runtime := js.NewRuntime()
	runtime.SetOrigin("file:///home/page.html")
	runtime.SetFetcher(&mockFetcher{body: "data"})

	// localStorage works.
	_, err := runtime.RunScript(`localStorage.setItem("k", "v"); var x = localStorage.getItem("k"); if (x !== "v") throw new Error(x)`)
	if err != nil {
		t.Fatalf("localStorage should work without enforcer: %v", err)
	}

	// sessionStorage works.
	_, err = runtime.RunScript(`sessionStorage.setItem("k", "v"); var x = sessionStorage.getItem("k"); if (x !== "v") throw new Error(x)`)
	if err != nil {
		t.Fatalf("sessionStorage should work without enforcer: %v", err)
	}
}

func TestDefaultSecurePolicy(t *testing.T) {
	p := js.DefaultSecurePolicy()
	if p.DefaultAPIPermission != js.APIDenied {
		t.Errorf("DefaultAPIPermission = %d, want js.APIDenied", p.DefaultAPIPermission)
	}

	// Core capabilities should be allowed via wildcard.
	perms := p.OriginPermissions["*"]
	if perms == nil {
		t.Fatal("wildcard origin permissions missing")
	}
	if perm, ok := perms[js.CapabilityNetwork]; !ok || perm != js.APIAllowed {
		t.Errorf("js.CapabilityNetwork should be js.APIAllowed")
	}
	if perm, ok := perms[js.CapabilityStorage]; !ok || perm != js.APIAllowed {
		t.Errorf("js.CapabilityStorage should be js.APIAllowed")
	}
	if perm, ok := perms[js.CapabilityNavigation]; !ok || perm != js.APIAllowed {
		t.Errorf("js.CapabilityNavigation should be js.APIAllowed")
	}

	// Unsupported capabilities should NOT be in the wildcard (fall through to denied).
	e := js.NewScriptEnforcer(p)
	if err := e.CheckAPIPermission("https://x.com", js.CapabilityGeolocation); err != js.ErrAPIPermissionDenied {
		t.Errorf("geolocation should be denied by default: %v", err)
	}
	if err := e.CheckAPIPermission("https://x.com", js.CapabilityClipboard); err != js.ErrAPIPermissionDenied {
		t.Errorf("clipboard should be denied by default: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Navigator API gating tests
// ---------------------------------------------------------------------------

func TestNavigatorGeolocation_DefaultDenied(t *testing.T) {
	rt := js.NewRuntime()
	e := js.NewScriptEnforcer(js.DefaultSecurePolicy())
	rt.SetEnforcer(e)

	v, err := rt.RunScript(`
		var rejected = false;
		navigator.geolocation.getCurrentPosition(
			function() { rejected = false; },
			function() { rejected = true; }
		);
		rejected;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "true" {
		t.Errorf("geolocation getCurrentPosition should call error callback when denied, got %s", v.String())
	}
}

func TestNavigatorGeolocation_WatchPositionDefaultDenied(t *testing.T) {
	rt := js.NewRuntime()
	e := js.NewScriptEnforcer(js.DefaultSecurePolicy())
	rt.SetEnforcer(e)

	v, err := rt.RunScript(`
		var rejected = false;
		navigator.geolocation.watchPosition(
			function() { rejected = false; },
			function() { rejected = true; }
		);
		rejected;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "true" {
		t.Errorf("geolocation watchPosition should call error callback when denied, got %s", v.String())
	}
}

func TestNavigatorClipboard_DefaultDenied(t *testing.T) {
	rt := js.NewRuntime()
	e := js.NewScriptEnforcer(js.DefaultSecurePolicy())
	rt.SetEnforcer(e)

	v, err := rt.RunScript(`
		var msg = "";
		var catchCalled = false;
		navigator.clipboard.readText().catch(function(e) {
			catchCalled = true;
			msg = e.message;
		});
		catchCalled;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "true" {
		t.Errorf("clipboard readText should reject when denied, got %s", v.String())
	}
}

func TestNavigatorClipboard_WriteTextDefaultDenied(t *testing.T) {
	rt := js.NewRuntime()
	e := js.NewScriptEnforcer(js.DefaultSecurePolicy())
	rt.SetEnforcer(e)

	v, err := rt.RunScript(`
		var catchCalled = false;
		navigator.clipboard.writeText("hello").catch(function() {
			catchCalled = true;
		});
		catchCalled;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "true" {
		t.Errorf("clipboard writeText should reject when denied, got %s", v.String())
	}
}

func TestNotificationConstructor_DefaultDenied(t *testing.T) {
	rt := js.NewRuntime()
	e := js.NewScriptEnforcer(js.DefaultSecurePolicy())
	rt.SetEnforcer(e)

	_, err := rt.RunScript(`new Notification("test")`)
	if err == nil {
		t.Fatal("Notification constructor should throw when denied")
	}
	// The error message should include the permission denied text.
	if !strings.Contains(err.Error(), "API permission denied") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNavigatorAPIs_NoEnforcer(t *testing.T) {
	// Without an enforcer, all capabilities are allowed — but the APIs
	// still return "not implemented" since they aren't built.
	rt := js.NewRuntime()

	v, err := rt.RunScript(`
		var catchCalled = false;
		navigator.geolocation.getCurrentPosition(
			function() { catchCalled = false; },
			function() { catchCalled = true; }
		);
		catchCalled;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "true" {
		t.Errorf("should reject with 'not implemented' when no enforcer, got %s", v.String())
	}
}

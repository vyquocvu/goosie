package js

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNewRuntime(t *testing.T) {
	runtime := NewRuntime()
	if runtime == nil {
		t.Fatal("NewRuntime() returned nil")
	}
	if runtime.vm == nil {
		t.Fatal("Runtime vm is nil")
	}
}

func TestConsoleLog(t *testing.T) {
	runtime := NewRuntime()
	
	_, err := runtime.RunScript(`console.log("test message");`)
	if err != nil {
		t.Errorf("console.log failed: %v", err)
	}
}

func TestDocumentGetElementByID(t *testing.T) {
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
	html := `<html><body>Test</body></html>`
	runtime.SetHTMLContent(html)
	
	if runtime.htmlCache != html {
		t.Errorf("SetHTMLContent() did not set htmlCache correctly")
	}
}

func TestRunScript(t *testing.T) {
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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

func TestCreateElement(t *testing.T) {
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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

func TestRemoveChild(t *testing.T) {
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
runtime := NewRuntime()

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
runtime := NewRuntime()

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
runtime := NewRuntime()

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
runtime := NewRuntime()

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
runtime := NewRuntime()

// Test setTimeout
_, err := runtime.RunScript(`
var executed = false;
var timerId = setTimeout(function() {
executed = true;
}, 10);
`)
if err != nil {
t.Errorf("setTimeout failed: %v", err)
}

// Wait for timer to execute
time.Sleep(50 * time.Millisecond)

val, err := runtime.RunScript(`executed`)
if err != nil {
t.Errorf("Failed to check executed: %v", err)
}
if !val.ToBoolean() {
t.Errorf("setTimeout callback was not executed")
}
}

func TestClearTimeout(t *testing.T) {
runtime := NewRuntime()
defer runtime.Cleanup()

// Test clearTimeout
_, err := runtime.RunScript(`
var executed = false;
var timerId = setTimeout(function() {
executed = true;
}, 10);
clearTimeout(timerId);
`)
if err != nil {
t.Errorf("clearTimeout failed: %v", err)
}

// Wait to ensure timer doesn't execute
time.Sleep(50 * time.Millisecond)

val, err := runtime.RunScript(`executed`)
if err != nil {
t.Errorf("Failed to check executed: %v", err)
}
if val.ToBoolean() {
t.Errorf("setTimeout callback should not have been executed after clearTimeout")
}
}

func TestSetInterval(t *testing.T) {
runtime := NewRuntime()
defer runtime.Cleanup()

// Test setInterval
_, err := runtime.RunScript(`
var counter = 0;
var intervalId = setInterval(function() {
counter++;
}, 10);
`)
if err != nil {
t.Errorf("setInterval failed: %v", err)
}

// Wait for multiple executions
time.Sleep(55 * time.Millisecond)

// Clear the interval
runtime.RunScript(`clearInterval(intervalId)`)

val, err := runtime.RunScript(`counter`)
if err != nil {
t.Errorf("Failed to check counter: %v", err)
}

counter := val.ToInteger()
if counter < 2 {
t.Errorf("Expected counter >= 2, got %d", counter)
}
}

func TestClearInterval(t *testing.T) {
runtime := NewRuntime()
defer runtime.Cleanup()

// Test clearInterval
_, err := runtime.RunScript(`
var counter = 0;
var intervalId = setInterval(function() {
counter++;
}, 10);
clearInterval(intervalId);
`)
if err != nil {
t.Errorf("clearInterval failed: %v", err)
}

// Wait to ensure interval doesn't execute
time.Sleep(50 * time.Millisecond)

val, err := runtime.RunScript(`counter`)
if err != nil {
t.Errorf("Failed to check counter: %v", err)
}
if val.ToInteger() != 0 {
t.Errorf("Expected counter 0 after clearInterval, got %d", val.ToInteger())
}
}

func TestLocalStorage(t *testing.T) {
runtime := NewRuntime()

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
runtime := NewRuntime()

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
runtime := NewRuntime()

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
runtime := NewRuntime()

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
runtime := NewRuntime()

// Test basic fetch
_, err := runtime.RunScript(`
var fetchCalled = false;
fetch("https://api.example.com/data")
.then(function(response) {
fetchCalled = true;
});
`)
if err != nil {
t.Errorf("fetch failed: %v", err)
}

// Give time for async operation
time.Sleep(50 * time.Millisecond)

val, err := runtime.RunScript(`fetchCalled`)
if err != nil {
t.Errorf("Failed to check fetchCalled: %v", err)
}
if !val.ToBoolean() {
t.Errorf("fetch callback was not executed")
}
}

func TestRuntimeCleanup(t *testing.T) {
runtime := NewRuntime()

// Create some timers
runtime.RunScript(`
setTimeout(function() {}, 1000);
setInterval(function() {}, 1000);
`)

if len(runtime.timers) != 2 {
t.Errorf("Expected 2 timers, got %d", len(runtime.timers))
}

// Cleanup
runtime.Cleanup()

if len(runtime.timers) != 0 {
t.Errorf("Expected 0 timers after cleanup, got %d", len(runtime.timers))
}
}

func TestLocationSearchAndHashPrefixes(t *testing.T) {
runtime := NewRuntime()

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
runtime := NewRuntime()
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
runtime := NewRuntime()
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	
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
	runtime := NewRuntime()
	mock := &mockFetcher{body: "hello world"}
	runtime.SetFetcher(mock)
	if runtime.fetcher != mock {
		t.Errorf("SetFetcher did not set the fetcher")
	}
}

func TestFetchAPIWithRealFetcher_Text(t *testing.T) {
	runtime := NewRuntime()
	runtime.SetFetcher(&mockFetcher{body: "hello from server"})

	_, err := runtime.RunScript(`
var textResult = "";
fetch("https://example.com/api")
  .then(function(response) {
    response.text().then(function(text) {
      textResult = text;
    });
  });
`)
	if err != nil {
		t.Fatalf("fetch script failed: %v", err)
	}

	// Allow goroutine to complete
	time.Sleep(100 * time.Millisecond)

	val, err := runtime.RunScript(`textResult`)
	if err != nil {
		t.Fatalf("reading textResult failed: %v", err)
	}
	if val.String() != "hello from server" {
		t.Errorf("expected 'hello from server', got %q", val.String())
	}
}

func TestFetchAPIWithRealFetcher_JSON(t *testing.T) {
	runtime := NewRuntime()
	runtime.SetFetcher(&mockFetcher{body: `{"name":"goosie","version":1}`})

	_, err := runtime.RunScript(`
var jsonResult = null;
fetch("https://example.com/api")
  .then(function(response) {
    response.json().then(function(data) {
      jsonResult = data;
    });
  });
`)
	if err != nil {
		t.Fatalf("fetch script failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	val, err := runtime.RunScript(`jsonResult !== null && jsonResult.name === "goosie"`)
	if err != nil {
		t.Fatalf("reading jsonResult failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected jsonResult.name to be 'goosie'")
	}
}

func TestFetchAPIWithRealFetcher_ErrorPropagation(t *testing.T) {
	runtime := NewRuntime()
	runtime.SetFetcher(&mockFetcher{err: fmt.Errorf("connection refused")})

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
	if err != nil {
		t.Fatalf("fetch script failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	val, err := runtime.RunScript(`catchCalled`)
	if err != nil {
		t.Fatalf("reading catchCalled failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected catch callback to be called on fetch error")
	}

	val, err = runtime.RunScript(`catchMsg`)
	if err != nil {
		t.Fatalf("reading catchMsg failed: %v", err)
	}
	if !strings.Contains(val.String(), "connection refused") {
		t.Errorf("expected error message to contain 'connection refused', got %q", val.String())
	}
}


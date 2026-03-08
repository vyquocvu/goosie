package js

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// Timer represents a scheduled timer
type Timer struct {
	ID       int
	Callback goja.Callable
	Interval time.Duration
	Repeat   bool
	Timer    *time.Timer
	Ticker   *time.Ticker
	Cancel   chan bool
	mu       sync.Mutex
	stopped  bool  // Track if timer has been stopped
}

// ConsoleMessage represents a console log message
type ConsoleMessage struct {
	Level     string        // "log", "error", "warn", "info", "table"
	Message   string        // Formatted message
	Timestamp time.Time     // When the message was logged
	Data      interface{}   // Raw data for table display
}

// Console table formatting constants
const (
	tableArrayValueWidth    = 40
	tableArrayValueTruncate = 37
	tableMapKeyWidth        = 20
	tableMapKeyTruncate     = 17
	tableMapValueWidth      = 20
	tableMapValueTruncate   = 17
)

// Runtime wraps the Goja JavaScript runtime
type Runtime struct {
	vm         *goja.Runtime
	parser     *dom.Parser
	htmlCache  string
	eventListeners map[string][]goja.Callable
	// Browser API storage
	localStorage   map[string]string
	sessionStorage map[string]string
	timers         map[int]*Timer
	timerIDCounter int
	// History tracking
	historyStack   []string
	historyIndex   int
	// Console messages
	consoleMessages []ConsoleMessage
	consoleMu       sync.Mutex
	// JavaScript errors
	jsErrors        []string
	jsErrorsMu      sync.Mutex
}

// NewRuntime creates a new JavaScript runtime with console.log and document APIs
func NewRuntime() *Runtime {
	vm := goja.New()
	parser := dom.NewParser()
	
	runtime := &Runtime{
		vm:              vm,
		parser:          parser,
		eventListeners:  make(map[string][]goja.Callable),
		localStorage:    make(map[string]string),
		sessionStorage:  make(map[string]string),
		timers:          make(map[int]*Timer),
		timerIDCounter:  1,
		historyStack:    []string{},
		historyIndex:    -1,
		consoleMessages: make([]ConsoleMessage, 0),
		jsErrors:        make([]string, 0),
	}

	// Setup enhanced console API
	runtime.setupConsoleAPI()

	// Setup document object with all DOM APIs
	runtime.setupDocumentAPI()
	
	// Setup window object with browser APIs
	runtime.setupWindowAPI()

	return runtime
}

// setupConsoleAPI configures all console methods with message tracking
func (r *Runtime) setupConsoleAPI() {
	console := r.vm.NewObject()
	
	// Helper function to format arguments
	formatArgs := func(args []goja.Value) string {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = fmt.Sprintf("%v", arg.Export())
		}
		return strings.Join(parts, " ")
	}
	
	// Helper function to log a console message
	logMessage := func(level string, args []goja.Value, data interface{}) {
		message := formatArgs(args)
		r.consoleMu.Lock()
		r.consoleMessages = append(r.consoleMessages, ConsoleMessage{
			Level:     level,
			Message:   message,
			Timestamp: time.Now(),
			Data:      data,
		})
		r.consoleMu.Unlock()
		
		// Also print to stdout with level prefix
		prefix := ""
		switch level {
		case "error":
			prefix = "[ERROR] "
		case "warn":
			prefix = "[WARN] "
		case "info":
			prefix = "[INFO] "
		case "table":
			prefix = "[TABLE] "
		}
		fmt.Println(prefix + message)
	}
	
	// console.log
	console.Set("log", func(call goja.FunctionCall) goja.Value {
		logMessage("log", call.Arguments, nil)
		return goja.Undefined()
	})
	
	// console.error
	console.Set("error", func(call goja.FunctionCall) goja.Value {
		logMessage("error", call.Arguments, nil)
		return goja.Undefined()
	})
	
	// console.warn
	console.Set("warn", func(call goja.FunctionCall) goja.Value {
		logMessage("warn", call.Arguments, nil)
		return goja.Undefined()
	})
	
	// console.info
	console.Set("info", func(call goja.FunctionCall) goja.Value {
		logMessage("info", call.Arguments, nil)
		return goja.Undefined()
	})
	
	// console.table - format data as a table
	console.Set("table", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		data := call.Arguments[0].Export()
		
		// Format the table data
		var tableStr strings.Builder
		tableStr.WriteString("\n")
		
		switch v := data.(type) {
		case []interface{}:
			// Array of values
			tableStr.WriteString("┌─────┬─────────────────────────────────────────┐\n")
			tableStr.WriteString("│ (i) │ Value                                   │\n")
			tableStr.WriteString("├─────┼─────────────────────────────────────────┤\n")
			for i, item := range v {
				value := fmt.Sprintf("%v", item)
				if len(value) > tableArrayValueWidth {
					value = value[:tableArrayValueTruncate] + "..."
				}
				tableStr.WriteString(fmt.Sprintf("│ %-3d │ %-*s│\n", i, tableArrayValueWidth, value))
			}
			tableStr.WriteString("└─────┴─────────────────────────────────────────┘")
		case map[string]interface{}:
			// Object/map
			tableStr.WriteString("┌─────────────────────┬──────────────────────┐\n")
			tableStr.WriteString("│ Key                 │ Value                │\n")
			tableStr.WriteString("├─────────────────────┼──────────────────────┤\n")
			for key, val := range v {
				keyStr := key
				if len(keyStr) > tableMapKeyWidth {
					keyStr = keyStr[:tableMapKeyTruncate] + "..."
				}
				valStr := fmt.Sprintf("%v", val)
				if len(valStr) > tableMapValueWidth {
					valStr = valStr[:tableMapValueTruncate] + "..."
				}
				tableStr.WriteString(fmt.Sprintf("│ %-*s│ %-*s│\n", tableMapKeyWidth, keyStr, tableMapValueWidth, valStr))
			}
			tableStr.WriteString("└─────────────────────┴──────────────────────┘")
		default:
			// Single value
			tableStr.WriteString(fmt.Sprintf("Value: %v", v))
		}
		
		args := []goja.Value{r.vm.ToValue(tableStr.String())}
		logMessage("table", args, data)
		return goja.Undefined()
	})
	
	r.vm.Set("console", console)
}

// setupDocumentAPI configures all document-related APIs
func (r *Runtime) setupDocumentAPI() {
	document := r.vm.NewObject()
	
	// document.getElementById
	document.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		id := call.Arguments[0].String()
		
		if r.htmlCache == "" {
			return goja.Null()
		}
		
		element, err := r.parser.GetElementByIDFull(r.htmlCache, id)
		if err != nil || element == nil {
			return goja.Null()
		}
		
		return r.createElementObject(element.ID, element.TextContent, element.TagName, element)
	})
	
	// document.getElementsByClassName
	document.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.NewArray()
		}
		className := call.Arguments[0].String()
		
		if r.htmlCache == "" {
			return r.vm.NewArray()
		}
		
		elements, err := r.parser.GetElementsByClassName(r.htmlCache, className)
		if err != nil {
			return r.vm.NewArray()
		}
		
		return r.createElementArray(elements)
	})
	
	// document.getElementsByTagName
	document.Set("getElementsByTagName", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.NewArray()
		}
		tagName := call.Arguments[0].String()
		
		if r.htmlCache == "" {
			return r.vm.NewArray()
		}
		
		elements, err := r.parser.GetElementsByTagName(r.htmlCache, tagName)
		if err != nil {
			return r.vm.NewArray()
		}
		
		return r.createElementArray(elements)
	})
	
	// document.querySelector
	document.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		selector := call.Arguments[0].String()
		
		if r.htmlCache == "" {
			return goja.Null()
		}
		
		element, err := r.parser.QuerySelector(r.htmlCache, selector)
		if err != nil || element == nil {
			return goja.Null()
		}
		
		return r.createElementObject(element.ID, element.TextContent, element.TagName, element)
	})
	
	// document.querySelectorAll
	document.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.NewArray()
		}
		selector := call.Arguments[0].String()
		
		if r.htmlCache == "" {
			return r.vm.NewArray()
		}
		
		elements, err := r.parser.QuerySelectorAll(r.htmlCache, selector)
		if err != nil {
			return r.vm.NewArray()
		}
		
		return r.createElementArray(elements)
	})
	
	// document.createElement
	document.Set("createElement", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		tagName := call.Arguments[0].String()
		
		obj := r.vm.NewObject()
		obj.Set("tagName", tagName)
		obj.Set("textContent", "")
		obj.Set("children", r.vm.NewArray())
		
		// Add manipulation methods
		r.addManipulationMethods(obj)
		
		return obj
	})
	
	r.vm.Set("document", document)
}

// createElementObject creates a JavaScript object representing a DOM element
func (r *Runtime) createElementObject(id, textContent, tagName string, elem *dom.Element) goja.Value {
	obj := r.vm.NewObject()
	obj.Set("textContent", textContent)
	
	if id != "" {
		obj.Set("id", id)
	}
	
	if tagName != "" {
		obj.Set("tagName", tagName)
	} else if elem != nil {
		obj.Set("tagName", elem.TagName)
	}
	
	if elem != nil {
		// Add classes
		if len(elem.Classes) > 0 {
			classesVal := r.vm.ToValue(elem.Classes)
			obj.Set("classList", classesVal)
		}
		
		// Add attributes
		if len(elem.Attributes) > 0 {
			attrs := r.vm.NewObject()
			for key, val := range elem.Attributes {
				attrs.Set(key, val)
			}
			obj.Set("attributes", attrs)
		}
	}
	
	// Add manipulation methods
	r.addManipulationMethods(obj)
	r.addEventMethods(obj)
	
	return obj
}

// createElementArray creates a JavaScript array of DOM elements
func (r *Runtime) createElementArray(elements []*dom.Element) goja.Value {
	elemValues := make([]interface{}, len(elements))
	for i, elem := range elements {
		elemObj := r.createElementObject(elem.ID, elem.TextContent, elem.TagName, elem)
		elemValues[i] = elemObj
	}
	return r.vm.ToValue(elemValues)
}

// addManipulationMethods adds DOM manipulation methods to an element object
func (r *Runtime) addManipulationMethods(obj *goja.Object) {
	// appendChild
	obj.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		child := call.Arguments[0]
		
		// Use JavaScript to manipulate the children array
		_, err := r.vm.RunString(`
			(function(parent, child) {
				if (!parent.children) {
					parent.children = [];
				}
				parent.children.push(child);
			})
		`)
		if err != nil {
			return goja.Undefined()
		}
		
		fn, _ := goja.AssertFunction(r.vm.Get("_"))
		if fn != nil {
			fn(goja.Undefined(), obj, child)
		}
		
		// Direct manipulation approach
		childrenVal := obj.Get("children")
		var childrenArr []goja.Value
		
		if childrenVal != nil && childrenVal != goja.Undefined() {
			if childrenObj := childrenVal.ToObject(r.vm); childrenObj != nil {
				length := childrenObj.Get("length").ToInteger()
				for i := int64(0); i < length; i++ {
					childrenArr = append(childrenArr, childrenObj.Get(fmt.Sprintf("%d", i)))
				}
			}
		}
		
		childrenArr = append(childrenArr, child)
		obj.Set("children", r.vm.ToValue(childrenArr))
		
		return child
	})
	
	// removeChild
	obj.Set("removeChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		childToRemove := call.Arguments[0]
		childToRemoveObj := childToRemove.ToObject(r.vm)
		
		childrenVal := obj.Get("children")
		if childrenVal == nil || childrenVal == goja.Undefined() {
			return goja.Undefined()
		}
		
		var childrenArr []goja.Value
		if childrenObj := childrenVal.ToObject(r.vm); childrenObj != nil {
			length := childrenObj.Get("length").ToInteger()
			for i := int64(0); i < length; i++ {
				child := childrenObj.Get(fmt.Sprintf("%d", i))
				childObj := child.ToObject(r.vm)
				// Compare by checking if they reference the same object
				if childObj != childToRemoveObj {
					childrenArr = append(childrenArr, child)
				}
			}
		}
		
		obj.Set("children", r.vm.ToValue(childrenArr))
		return childToRemove
	})
	
	// replaceChild
	obj.Set("replaceChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		newChild := call.Arguments[0]
		oldChild := call.Arguments[1]
		oldChildObj := oldChild.ToObject(r.vm)
		
		childrenVal := obj.Get("children")
		if childrenVal == nil || childrenVal == goja.Undefined() {
			return goja.Undefined()
		}
		
		var childrenArr []goja.Value
		if childrenObj := childrenVal.ToObject(r.vm); childrenObj != nil {
			length := childrenObj.Get("length").ToInteger()
			for i := int64(0); i < length; i++ {
				child := childrenObj.Get(fmt.Sprintf("%d", i))
				childObj := child.ToObject(r.vm)
				if childObj == oldChildObj {
					childrenArr = append(childrenArr, newChild)
				} else {
					childrenArr = append(childrenArr, child)
				}
			}
		}
		
		obj.Set("children", r.vm.ToValue(childrenArr))
		return oldChild
	})
	
	// insertBefore
	obj.Set("insertBefore", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		newChild := call.Arguments[0]
		refChild := call.Arguments[1]
		refChildObj := refChild.ToObject(r.vm)
		
		childrenVal := obj.Get("children")
		var childrenArr []goja.Value
		
		if childrenVal != nil && childrenVal != goja.Undefined() {
			if childrenObj := childrenVal.ToObject(r.vm); childrenObj != nil {
				length := childrenObj.Get("length").ToInteger()
				for i := int64(0); i < length; i++ {
					childrenArr = append(childrenArr, childrenObj.Get(fmt.Sprintf("%d", i)))
				}
			}
		}
		
		newChildrenArr := make([]goja.Value, 0)
		inserted := false
		
		for _, child := range childrenArr {
			childObj := child.ToObject(r.vm)
			if childObj == refChildObj && !inserted {
				newChildrenArr = append(newChildrenArr, newChild)
				inserted = true
			}
			newChildrenArr = append(newChildrenArr, child)
		}
		
		if !inserted {
			newChildrenArr = append(newChildrenArr, newChild)
		}
		
		obj.Set("children", r.vm.ToValue(newChildrenArr))
		return newChild
	})
}

// addEventMethods adds event listener methods to an element object
func (r *Runtime) addEventMethods(obj *goja.Object) {
	// addEventListener
	obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		eventType := call.Arguments[0].String()
		callback, ok := goja.AssertFunction(call.Arguments[1])
		if !ok {
			return goja.Undefined()
		}
		
		// Store the listener
		key := fmt.Sprintf("%p:%s", obj, eventType)
		r.eventListeners[key] = append(r.eventListeners[key], callback)
		
		return goja.Undefined()
	})
	
	// removeEventListener
	obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		eventType := call.Arguments[0].String()
		
		// Clear all listeners for this event type (simplified implementation)
		// In a full implementation, we'd need to track function identity
		key := fmt.Sprintf("%p:%s", obj, eventType)
		r.eventListeners[key] = make([]goja.Callable, 0)
		
		return goja.Undefined()
	})
}

// SetHTMLContent sets the HTML content for document operations
func (r *Runtime) SetHTMLContent(html string) {
	r.htmlCache = html
}

// RunScript executes JavaScript code and catches errors
func (r *Runtime) RunScript(script string) (goja.Value, error) {
	val, err := r.vm.RunString(script)
	if err != nil {
		// Log JavaScript error
		errorMsg := fmt.Sprintf("JavaScript Error: %v", err)
		r.jsErrorsMu.Lock()
		r.jsErrors = append(r.jsErrors, errorMsg)
		r.jsErrorsMu.Unlock()
		
		// Also add to console as an error
		r.consoleMu.Lock()
		r.consoleMessages = append(r.consoleMessages, ConsoleMessage{
			Level:     "error",
			Message:   errorMsg,
			Timestamp: time.Now(),
			Data:      nil,
		})
		r.consoleMu.Unlock()
		
		fmt.Println("[JS ERROR]", errorMsg)
	}
	return val, err
}

// GetConsoleMessages returns all console messages
func (r *Runtime) GetConsoleMessages() []ConsoleMessage {
	r.consoleMu.Lock()
	defer r.consoleMu.Unlock()
	
	// Return a copy to prevent concurrent modification
	messages := make([]ConsoleMessage, len(r.consoleMessages))
	copy(messages, r.consoleMessages)
	return messages
}

// ClearConsoleMessages clears all console messages
func (r *Runtime) ClearConsoleMessages() {
	r.consoleMu.Lock()
	defer r.consoleMu.Unlock()
	r.consoleMessages = make([]ConsoleMessage, 0)
}

// GetJavaScriptErrors returns all JavaScript errors
func (r *Runtime) GetJavaScriptErrors() []string {
	r.jsErrorsMu.Lock()
	defer r.jsErrorsMu.Unlock()
	
	// Return a copy
	errors := make([]string, len(r.jsErrors))
	copy(errors, r.jsErrors)
	return errors
}

// ClearJavaScriptErrors clears all JavaScript errors
func (r *Runtime) ClearJavaScriptErrors() {
	r.jsErrorsMu.Lock()
	defer r.jsErrorsMu.Unlock()
	r.jsErrors = make([]string, 0)
}

// setupWindowAPI configures window object with browser APIs
func (r *Runtime) setupWindowAPI() {
	window := r.vm.NewObject()
	
	// Setup window.location
	r.setupLocationAPI(window)
	
	// Setup window.history
	r.setupHistoryAPI(window)
	
	// Setup localStorage
	r.setupLocalStorageAPI()
	
	// Setup sessionStorage
	r.setupSessionStorageAPI()
	
	// Setup setTimeout and setInterval
	r.setupTimerAPIs()
	
	// Setup fetch API
	r.setupFetchAPI()
	
	r.vm.Set("window", window)
}

// setupLocationAPI configures window.location object
func (r *Runtime) setupLocationAPI(window *goja.Object) {
	location := r.vm.NewObject()
	currentURL := "about:blank"
	
	// href - full URL
	location.Set("href", currentURL)
	
	// protocol
	location.Set("protocol", "")
	
	// host
	location.Set("host", "")
	
	// hostname
	location.Set("hostname", "")
	
	// port
	location.Set("port", "")
	
	// pathname
	location.Set("pathname", "")
	
	// search - query string
	location.Set("search", "")
	
	// hash
	location.Set("hash", "")
	
	// reload - refresh the page
	location.Set("reload", func(call goja.FunctionCall) goja.Value {
		// In a real browser this would reload the page
		// For now, we just log the action
		fmt.Println("Location reload called")
		return goja.Undefined()
	})
	
	// Helper to parse URL and update location properties
	location.Set("setURL", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		urlStr := call.Arguments[0].String()
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return goja.Undefined()
		}
		
		location.Set("href", urlStr)
		location.Set("protocol", parsedURL.Scheme+":")
		location.Set("host", parsedURL.Host)
		location.Set("hostname", parsedURL.Hostname())
		location.Set("port", parsedURL.Port())
		location.Set("pathname", parsedURL.Path)
		
		// Add '?' prefix to search if query exists
		search := ""
		if parsedURL.RawQuery != "" {
			search = "?" + parsedURL.RawQuery
		}
		location.Set("search", search)
		
		// Add '#' prefix to hash if fragment exists
		hash := ""
		if parsedURL.Fragment != "" {
			hash = "#" + parsedURL.Fragment
		}
		location.Set("hash", hash)
		
		return goja.Undefined()
	})
	
	// Helper to get query parameters
	location.Set("getQueryParam", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		
		paramName := call.Arguments[0].String()
		hrefVal := location.Get("href")
		if hrefVal == nil || hrefVal == goja.Undefined() {
			return goja.Null()
		}
		
		urlStr := hrefVal.String()
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return goja.Null()
		}
		
		params := parsedURL.Query()
		value := params.Get(paramName)
		if value == "" {
			return goja.Null()
		}
		
		return r.vm.ToValue(value)
	})
	
	// Helper to set query parameters
	location.Set("setQueryParam", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		paramName := call.Arguments[0].String()
		paramValue := call.Arguments[1].String()
		
		hrefVal := location.Get("href")
		if hrefVal == nil || hrefVal == goja.Undefined() {
			return goja.Undefined()
		}
		
		urlStr := hrefVal.String()
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return goja.Undefined()
		}
		
		params := parsedURL.Query()
		params.Set(paramName, paramValue)
		parsedURL.RawQuery = params.Encode()
		
		newURL := parsedURL.String()
		location.Set("href", newURL)
		
		// Update search with '?' prefix
		search := ""
		if parsedURL.RawQuery != "" {
			search = "?" + parsedURL.RawQuery
		}
		location.Set("search", search)
		
		return r.vm.ToValue(newURL)
	})
	
	window.Set("location", location)
}

// setupHistoryAPI configures window.history object
func (r *Runtime) setupHistoryAPI(window *goja.Object) {
	history := r.vm.NewObject()
	
	// length - number of entries in history
	history.Set("length", func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.historyStack))
	})
	
	// back - go back one page
	history.Set("back", func(call goja.FunctionCall) goja.Value {
		if r.historyIndex > 0 {
			r.historyIndex--
			fmt.Printf("History: navigated back to index %d\n", r.historyIndex)
		}
		return goja.Undefined()
	})
	
	// forward - go forward one page
	history.Set("forward", func(call goja.FunctionCall) goja.Value {
		if r.historyIndex < len(r.historyStack)-1 {
			r.historyIndex++
			fmt.Printf("History: navigated forward to index %d\n", r.historyIndex)
		}
		return goja.Undefined()
	})
	
	// go - navigate by relative position
	history.Set("go", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		delta := int(call.Arguments[0].ToInteger())
		newIndex := r.historyIndex + delta
		
		if newIndex >= 0 && newIndex < len(r.historyStack) {
			r.historyIndex = newIndex
			fmt.Printf("History: navigated to index %d\n", r.historyIndex)
		}
		
		return goja.Undefined()
	})
	
	// pushState - add a new history entry
	history.Set("pushState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			return goja.Undefined()
		}
		
		// state, title, url
		urlStr := call.Arguments[2].String()
		
		// Truncate forward history if we're not at the end
		if r.historyIndex < len(r.historyStack)-1 {
			r.historyStack = r.historyStack[:r.historyIndex+1]
		}
		
		r.historyStack = append(r.historyStack, urlStr)
		r.historyIndex = len(r.historyStack) - 1
		
		return goja.Undefined()
	})
	
	// replaceState - replace current history entry
	history.Set("replaceState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			return goja.Undefined()
		}
		
		// state, title, url
		urlStr := call.Arguments[2].String()
		
		if r.historyIndex >= 0 && r.historyIndex < len(r.historyStack) {
			r.historyStack[r.historyIndex] = urlStr
		}
		
		return goja.Undefined()
	})
	
	window.Set("history", history)
}

// setupLocalStorageAPI configures localStorage
func (r *Runtime) setupLocalStorageAPI() {
	localStorage := r.vm.NewObject()
	
	// getItem
	localStorage.Set("getItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		
		key := call.Arguments[0].String()
		value, exists := r.localStorage[key]
		if !exists {
			return goja.Null()
		}
		
		// Strip version prefix
		if strings.HasPrefix(value, "v1:") {
			return r.vm.ToValue(strings.TrimPrefix(value, "v1:"))
		}
		
		return r.vm.ToValue(value)
	})
	
	// setItem - with validation
	localStorage.Set("setItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()
		
		// Basic validation: check key and value are not empty
		if key == "" {
			fmt.Println("localStorage: key cannot be empty")
			return goja.Undefined()
		}
		
		// Store with version prefix for versioning support
		versionedValue := "v1:" + value
		r.localStorage[key] = versionedValue
		
		return goja.Undefined()
	})
	
	// removeItem
	localStorage.Set("removeItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		key := call.Arguments[0].String()
		delete(r.localStorage, key)
		
		return goja.Undefined()
	})
	
	// clear - remove all items
	localStorage.Set("clear", func(call goja.FunctionCall) goja.Value {
		r.localStorage = make(map[string]string)
		return goja.Undefined()
	})
	
	// key - get key at index
	localStorage.Set("key", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		
		index := int(call.Arguments[0].ToInteger())
		keys := make([]string, 0, len(r.localStorage))
		for k := range r.localStorage {
			keys = append(keys, k)
		}
		
		if index < 0 || index >= len(keys) {
			return goja.Null()
		}
		
		return r.vm.ToValue(keys[index])
	})
	
	// length property
	localStorage.Set("length", func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.localStorage))
	})
	
	r.vm.Set("localStorage", localStorage)
}

// setupSessionStorageAPI configures sessionStorage
func (r *Runtime) setupSessionStorageAPI() {
	sessionStorage := r.vm.NewObject()
	
	// getItem
	sessionStorage.Set("getItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		
		key := call.Arguments[0].String()
		value, exists := r.sessionStorage[key]
		if !exists {
			return goja.Null()
		}
		
		// Check if value has expired (simple implementation)
		parts := strings.SplitN(value, ":", 3)
		if len(parts) == 3 && parts[0] == "exp" {
			// Format: exp:timestamp:value
			// For now, we don't implement actual expiration
			return r.vm.ToValue(parts[2])
		}
		
		// Strip session prefix if present
		if strings.HasPrefix(value, "session:") {
			return r.vm.ToValue(strings.TrimPrefix(value, "session:"))
		}
		
		return r.vm.ToValue(value)
	})
	
	// setItem - with session schema support
	sessionStorage.Set("setItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		key := call.Arguments[0].String()
		value := call.Arguments[1].String()
		
		// Basic validation
		if key == "" {
			fmt.Println("sessionStorage: key cannot be empty")
			return goja.Undefined()
		}
		
		// Store with schema prefix (could be extended for expiration)
		schemaValue := "session:" + value
		r.sessionStorage[key] = schemaValue
		
		return goja.Undefined()
	})
	
	// removeItem
	sessionStorage.Set("removeItem", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		key := call.Arguments[0].String()
		delete(r.sessionStorage, key)
		
		return goja.Undefined()
	})
	
	// clear - remove all items
	sessionStorage.Set("clear", func(call goja.FunctionCall) goja.Value {
		r.sessionStorage = make(map[string]string)
		return goja.Undefined()
	})
	
	// key - get key at index
	sessionStorage.Set("key", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		
		index := int(call.Arguments[0].ToInteger())
		keys := make([]string, 0, len(r.sessionStorage))
		for k := range r.sessionStorage {
			keys = append(keys, k)
		}
		
		if index < 0 || index >= len(keys) {
			return goja.Null()
		}
		
		return r.vm.ToValue(keys[index])
	})
	
	// length property
	sessionStorage.Set("length", func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.sessionStorage))
	})
	
	r.vm.Set("sessionStorage", sessionStorage)
}

// setupTimerAPIs configures setTimeout and setInterval
func (r *Runtime) setupTimerAPIs() {
	// setTimeout
	r.vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			return goja.Undefined()
		}
		
		delay := time.Duration(call.Arguments[1].ToInteger()) * time.Millisecond
		
		timerID := r.timerIDCounter
		r.timerIDCounter++
		
		timer := &Timer{
			ID:       timerID,
			Callback: callback,
			Interval: delay,
			Repeat:   false,
			Cancel:   make(chan bool),
		}
		
		timer.Timer = time.AfterFunc(delay, func() {
			timer.mu.Lock()
			defer timer.mu.Unlock()
			
			select {
			case <-timer.Cancel:
				return
			default:
				callback(goja.Undefined())
				delete(r.timers, timerID)
			}
		})
		
		r.timers[timerID] = timer
		
		return r.vm.ToValue(timerID)
	})
	
	// clearTimeout
	r.vm.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		timerID := int(call.Arguments[0].ToInteger())
		timer, exists := r.timers[timerID]
		if exists {
			timer.mu.Lock()
			if !timer.stopped {
				close(timer.Cancel)
				timer.stopped = true
				if timer.Timer != nil {
					timer.Timer.Stop()
				}
			}
			timer.mu.Unlock()
			delete(r.timers, timerID)
		}
		
		return goja.Undefined()
	})
	
	// setInterval
	r.vm.Set("setInterval", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		
		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			return goja.Undefined()
		}
		
		interval := time.Duration(call.Arguments[1].ToInteger()) * time.Millisecond
		
		timerID := r.timerIDCounter
		r.timerIDCounter++
		
		timer := &Timer{
			ID:       timerID,
			Callback: callback,
			Interval: interval,
			Repeat:   true,
			Cancel:   make(chan bool),
		}
		
		timer.Ticker = time.NewTicker(interval)
		
		go func() {
			for {
				select {
				case <-timer.Cancel:
					return
				case <-timer.Ticker.C:
					timer.mu.Lock()
					callback(goja.Undefined())
					timer.mu.Unlock()
				}
			}
		}()
		
		r.timers[timerID] = timer
		
		return r.vm.ToValue(timerID)
	})
	
	// clearInterval
	r.vm.Set("clearInterval", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		
		timerID := int(call.Arguments[0].ToInteger())
		timer, exists := r.timers[timerID]
		if exists {
			timer.mu.Lock()
			if !timer.stopped {
				close(timer.Cancel)
				timer.stopped = true
				if timer.Ticker != nil {
					timer.Ticker.Stop()
				}
			}
			timer.mu.Unlock()
			delete(r.timers, timerID)
		}
		
		return goja.Undefined()
	})
}

// setupFetchAPI configures fetch API with error handling
func (r *Runtime) setupFetchAPI() {
	r.vm.Set("fetch", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue(r.createRejectedPromise("fetch requires a URL"))
		}
		
		urlStr := call.Arguments[0].String()
		
		// Create a promise-like object
		promise := r.vm.NewObject()
		
		// then method
		promise.Set("then", func(thenCall goja.FunctionCall) goja.Value {
			if len(thenCall.Arguments) == 0 {
				return promise
			}
			
			onSuccess, ok := goja.AssertFunction(thenCall.Arguments[0])
			if !ok {
				return promise
			}
			
			// Simulate async fetch (in real implementation, would use net/http)
			go func() {
				// Create response object
				response := r.vm.NewObject()
				response.Set("ok", true)
				response.Set("status", 200)
				response.Set("statusText", "OK")
				response.Set("url", urlStr)
				
				// json method
				response.Set("json", func(jsonCall goja.FunctionCall) goja.Value {
					jsonPromise := r.vm.NewObject()
					jsonPromise.Set("then", func(jsonThenCall goja.FunctionCall) goja.Value {
						// Return mock data
						return r.vm.ToValue(map[string]interface{}{"data": "mock"})
					})
					return jsonPromise
				})
				
				// text method
				response.Set("text", func(textCall goja.FunctionCall) goja.Value {
					textPromise := r.vm.NewObject()
					textPromise.Set("then", func(textThenCall goja.FunctionCall) goja.Value {
						return r.vm.ToValue("mock response text")
					})
					return textPromise
				})
				
				onSuccess(goja.Undefined(), response)
			}()
			
			return promise
		})
		
		// catch method
		promise.Set("catch", func(catchCall goja.FunctionCall) goja.Value {
			return promise
		})
		
		return promise
	})
}

// createRejectedPromise creates a rejected promise with an error message
func (r *Runtime) createRejectedPromise(errMsg string) *goja.Object {
	promise := r.vm.NewObject()
	
	promise.Set("then", func(call goja.FunctionCall) goja.Value {
		return promise
	})
	
	promise.Set("catch", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			onError, ok := goja.AssertFunction(call.Arguments[0])
			if ok {
				errorObj := r.vm.NewObject()
				errorObj.Set("message", errMsg)
				onError(goja.Undefined(), errorObj)
			}
		}
		return promise
	})
	
	return promise
}

// Cleanup cleans up all timers and resources
func (r *Runtime) Cleanup() {
	for _, timer := range r.timers {
		timer.mu.Lock()
		if !timer.stopped {
			close(timer.Cancel)
			timer.stopped = true
			if timer.Timer != nil {
				timer.Timer.Stop()
			}
			if timer.Ticker != nil {
				timer.Ticker.Stop()
			}
		}
		timer.mu.Unlock()
	}
	r.timers = make(map[int]*Timer)
}

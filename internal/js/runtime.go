package js

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// HTTPFetcher is the interface for making HTTP GET requests used by the fetch() JS API.
type HTTPFetcher interface {
	Fetch(url string) (string, error)
}

type LocalStorageAdapter interface {
	Get(origin, key string) (string, bool)
	Set(origin, key, value string) error
	Remove(origin, key string) error
	Clear(origin string) error
	Keys(origin string) []string
}

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
	stopped  bool // Track if timer has been stopped
}

// ConsoleMessage represents a console log message
type ConsoleMessage struct {
	Level     string      // "log", "error", "warn", "info", "table"
	Message   string      // Formatted message
	Timestamp time.Time   // When the message was logged
	Data      interface{} // Raw data for table display
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
	vm             *goja.Runtime
	parser         *dom.Parser
	htmlCache      string
	eventListeners map[string][]goja.Callable
	// Browser API storage
	localStorage        map[string]string
	sessionStorage      map[string]string
	localStorageAdapter LocalStorageAdapter
	origin              string
	timers              map[int]*Timer
	timerIDCounter      int
	// History tracking
	historyStack []string
	historyIndex int
	// Console messages
	consoleMessages []ConsoleMessage
	consoleMu       sync.Mutex
	// JavaScript errors
	jsErrors   []string
	jsErrorsMu sync.Mutex
	// HTTP fetcher for the fetch() API
	fetcher           HTTPFetcher
	isPopulatingJSDOM bool
	onDOMMutation     func(string)
	// enqueueTask allows routing asynchronous callbacks back to the owner goroutine.
	enqueueTask func(func())
	// OnOpenWindow is called when JS calls window.open() and the popup
	// is not blocked by policy. The callback receives the target URL and
	// the window name (target). If nil, window.open() is a no-op.
	OnOpenWindow func(url, name string)

	// OnNavigate is called when JS programmatically navigates via
	// window.location.href assignment, location.assign(), or
	// location.replace(). The callback receives the target URL string.
	// If nil, the navigation is silently ignored (href changes are
	// still reflected in the location object).
	OnNavigate func(url string)

	// OnRuntimeUnsupportedFeature is invoked when JavaScript uses an
	// engine-unsupported DOM API surface (e.g. document.createElement
	// with a tag the engine does not implement, like 'canvas', 'video',
	// 'audio', 'iframe'). Reports each detected kind at most once per
	// Runtime lifetime (deduplicated). Nil callback is a no-op.
	//
	// This complements the HTML parser's OnUnsupportedFeature callback
	// (in internal/dom): the parser detects unsupported tags in the
	// incoming HTML stream, while this callback detects them when the
	// page's JavaScript dynamically constructs them.
	OnRuntimeUnsupportedFeature func(dom.UnsupportedFeature)

	// runtimeDetected tracks which unsupported feature kinds have
	// already been reported by JavaScript during this page's lifetime.
	// Used to deduplicate per-kind reports. Access guarded by
	// runtimeDetectedMu.
	runtimeDetected   map[dom.UnsupportedFeatureKind]bool
	runtimeDetectedMu sync.Mutex

	// enforcer enforces script policy limits and API capabilities.
	// When nil (default), all capabilities are allowed.
	enforcer *ScriptEnforcer
}

// SetEnforcer attaches a ScriptEnforcer to this runtime for capability
// and execution-limit enforcement. Passing nil removes enforcement.
func (r *Runtime) SetEnforcer(e *ScriptEnforcer) {
	r.enforcer = e
}

// SetRuntimeUnsupportedFeatureCallback installs the callback that
// receives runtime-detected unsupported engine feature events from
// JavaScript (e.g. document.createElement('canvas')). Passing nil
// removes the callback. Each unsupported feature kind is reported
// at most once per Runtime (deduplicated).
func (r *Runtime) SetRuntimeUnsupportedFeatureCallback(cb func(dom.UnsupportedFeature)) {
	r.OnRuntimeUnsupportedFeature = cb
}

// reportRuntimeUnsupportedFeature notifies the registered callback of
// a runtime-detected unsupported feature. Each kind is reported at
// most once per Runtime. Safe to call from goroutines other than the
// JS owner (it serializes under runtimeDetectedMu). Nil callback is
// a no-op. The report is best-effort and does not propagate errors.
func (r *Runtime) reportRuntimeUnsupportedFeature(kind dom.UnsupportedFeatureKind) {
	if kind == 0 {
		return
	}
	cb := r.OnRuntimeUnsupportedFeature
	if cb == nil {
		return
	}

	r.runtimeDetectedMu.Lock()
	alreadyReported := r.runtimeDetected[kind]
	if !alreadyReported {
		if r.runtimeDetected == nil {
			r.runtimeDetected = make(map[dom.UnsupportedFeatureKind]bool, 8)
		}
		r.runtimeDetected[kind] = true
	}
	r.runtimeDetectedMu.Unlock()

	if alreadyReported {
		return
	}
	cb(dom.UnsupportedFeature{Kind: kind})
}

// ResetRuntimeUnsupportedFeatures clears the deduplication cache so
// the same kind can be reported again. Useful for tests and for
// re-using a Runtime across navigations.
func (r *Runtime) ResetRuntimeUnsupportedFeatures() {
	r.runtimeDetectedMu.Lock()
	r.runtimeDetected = nil
	r.runtimeDetectedMu.Unlock()
}

// allowCapability checks whether the given API capability is permitted
// for the runtime's current origin. Returns nil if allowed, or
// ErrAPIPermissionDenied if blocked. When no enforcer is set all
// capabilities are allowed (backward compatible).
func (r *Runtime) allowCapability(cap string) error {
	if r.enforcer == nil {
		return nil
	}
	return r.enforcer.CheckAPIPermission(r.origin, cap)
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
		origin:          "about:blank",
		timers:          make(map[int]*Timer),
		timerIDCounter:  1,
		historyStack:    []string{},
		historyIndex:    -1,
		consoleMessages: make([]ConsoleMessage, 0),
		jsErrors:        make([]string, 0),
		runtimeDetected: make(map[dom.UnsupportedFeatureKind]bool, 8),
	}

	// Inject ES6+ polyfills before any other setup
	if _, err := vm.RunString(polyfillsJS); err != nil {
		// Log but don't panic — partial polyfill is better than no runtime
		fmt.Printf("polyfill injection failed: %v\n", err)
	}

	// Setup enhanced console API
	runtime.setupConsoleAPI()

	// Setup window object with browser APIs
	runtime.setupWindowAPI()

	// Setup document object with all DOM APIs
	runtime.setupDocumentAPI()

	return runtime
}

func (r *Runtime) SetOrigin(origin string) {
	r.origin = origin
}

func (r *Runtime) SetLocalStorageAdapter(adapter LocalStorageAdapter) {
	r.localStorageAdapter = adapter
	r.setupLocalStorageAPI()
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

// setupDocumentAPI configures all document-related APIs using a full JS DOM system
func (r *Runtime) setupDocumentAPI() {
	jsDOMScript := `
(function() {
  class Node {
    constructor(nodeType, nodeName) {
      this.nodeType = nodeType;
      this.nodeName = nodeName;
      this.parentNode = null;
      this.childNodes = [];
      this._listeners = {};
    }
    
    get firstChild() { return this.childNodes[0] || null; }
    get lastChild() { return this.childNodes[this.childNodes.length - 1] || null; }
    
    get nextSibling() {
      if (!this.parentNode) return null;
      const idx = this.parentNode.childNodes.indexOf(this);
      return this.parentNode.childNodes[idx + 1] || null;
    }
    
    get previousSibling() {
      if (!this.parentNode) return null;
      const idx = this.parentNode.childNodes.indexOf(this);
      return this.parentNode.childNodes[idx - 1] || null;
    }
    
    appendChild(child) {
      if (child.parentNode) {
        child.parentNode.removeChild(child);
      }
      child.parentNode = this;
      this.childNodes.push(child);
      if (window.__onDOMChanged) window.__onDOMChanged();
      return child;
    }
    
    removeChild(child) {
      const idx = this.childNodes.indexOf(child);
      if (idx !== -1) {
        this.childNodes.splice(idx, 1);
        child.parentNode = null;
        if (window.__onDOMChanged) window.__onDOMChanged();
        return child;
      }
      throw new Error("NotFoundErr");
    }
    
    insertBefore(newChild, refChild) {
      if (!refChild) {
        return this.appendChild(newChild);
      }
      const idx = this.childNodes.indexOf(refChild);
      if (idx === -1) throw new Error("NotFoundErr");
      
      if (newChild.parentNode) {
        newChild.parentNode.removeChild(newChild);
      }
      newChild.parentNode = this;
      this.childNodes.splice(idx, 0, newChild);
      if (window.__onDOMChanged) window.__onDOMChanged();
      return newChild;
    }
    
    replaceChild(newChild, oldChild) {
      const idx = this.childNodes.indexOf(oldChild);
      if (idx === -1) throw new Error("NotFoundErr");
      
      if (newChild.parentNode) {
        newChild.parentNode.removeChild(newChild);
      }
      oldChild.parentNode = null;
      newChild.parentNode = this;
      this.childNodes[idx] = newChild;
      if (window.__onDOMChanged) window.__onDOMChanged();
      return oldChild;
    }
    
    addEventListener(type, listener) {
      if (!this._listeners[type]) this._listeners[type] = [];
      this._listeners[type].push(listener);
    }
    
    removeEventListener(type, listener) {
      if (!this._listeners[type]) return;
      this._listeners[type] = this._listeners[type].filter(l => l !== listener);
    }
    
    dispatchEvent(event) {
      const listeners = this._listeners[event.type] || [];
      listeners.forEach(l => {
        try {
          if (typeof l === "function") l.call(this, event);
          else if (l.handleEvent) l.handleEvent(event);
        } catch(e) {
          console.error(e);
        }
      });
      if (this.parentNode && event.bubbles) {
        this.parentNode.dispatchEvent(event);
      }
    }

    cloneNode(deep) {
      let clone;
      if (this.nodeType === 1) {
        clone = new Element(this.tagName);
        for (const key in this.attributes) {
          if (Object.prototype.hasOwnProperty.call(this.attributes, key) && key !== "__goosie_id") {
            clone.attributes[key] = this.attributes[key];
          }
        }
      } else if (this.nodeType === 3) {
        clone = new TextNode(this.textContent);
      } else if (this.nodeType === 9) {
        clone = new Document();
        clone.childNodes = [];
        clone.documentElement = null;
        clone.head = null;
        clone.body = null;
      } else {
        clone = new Node(this.nodeType, this.nodeName);
      }

      if (deep) {
        this.childNodes.forEach(child => {
          const childClone = child.cloneNode(true);
          childClone.parentNode = clone;
          clone.childNodes.push(childClone);
          if (childClone.nodeType === 1) {
            const tag = childClone.tagName.toLowerCase();
            if (tag === "html") clone.documentElement = childClone;
            if (tag === "head") clone.head = childClone;
            if (tag === "body") clone.body = childClone;
          }
        });
      }

      return clone;
    }
  }
  
  class TextNode extends Node {
    constructor(text) {
      super(3, "#text");
      this._textContent = text;
    }
    get textContent() { return this._textContent; }
    set textContent(val) {
      this._textContent = val;
      if (window.__onDOMChanged) window.__onDOMChanged();
    }
    get nodeValue() { return this._textContent; }
    set nodeValue(val) {
      this._textContent = val;
      if (window.__onDOMChanged) window.__onDOMChanged();
    }
  }
  
  class Element extends Node {
    constructor(tagName) {
      super(1, tagName);
      this.tagName = tagName;
      this.attributes = {};
      if (window.__goosieIdCounter) {
        this.attributes["__goosie_id"] = String(window.__goosieIdCounter++);
      }
      
      const self = this;
      this.style = new Proxy({
        _element: self,
        get cssText() {
          return this._element.getAttribute("style") || "";
        },
        set cssText(val) {
          this._element.setAttribute("style", val);
          for (const k in this) {
            if (!k.startsWith("_") && k !== "cssText") delete this[k];
          }
          val.split(";").forEach(pair => {
            const parts = pair.split(":");
            if (parts.length === 2) {
              const prop = parts[0].trim().replace(/-([a-z])/g, g => g[1].toUpperCase());
              this[prop] = parts[1].trim();
            }
          });
        }
      }, {
        set(target, prop, value) {
          if (prop === "cssText") {
            target.cssText = value;
            return true;
          }
          target[prop] = value;
          const styleStr = Object.keys(target)
            .filter(k => !k.startsWith("_") && k !== "cssText")
            .map(k => k.replace(/([A-Z])/g, "-$1").toLowerCase() + ": " + target[k])
            .join("; ");
          target._element.setAttribute("style", styleStr);
          return true;
        },
        get(target, prop) {
          if (prop === "cssText") return target.cssText;
          return target[prop];
        }
      });
      this.style.setProperty = (name, value) => {
        const camel = name.startsWith('--') ? name : name.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
        this.style[camel] = value;
      };
      this.style.getPropertyValue = (name) => {
        const camel = name.startsWith('--') ? name : name.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
        return this.style[camel] || '';
      };
      this.style.removeProperty = (name) => {
        const camel = name.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
        delete this.style[camel];
      };
    }
    
    get id() { return this.getAttribute("id") || ""; }
    set id(val) { this.setAttribute("id", val); }
    
    get className() { return this.getAttribute("class") || ""; }
    set className(val) { this.setAttribute("class", val); }
    
    get classList() {
      const self = this;
      return {
        add(...classes) {
          const current = self.className ? self.className.split(/\\s+/) : [];
          classes.forEach(c => {
            if (current.indexOf(c) === -1) current.push(c);
          });
          self.className = current.join(" ");
        },
        remove(...classes) {
          let current = self.className ? self.className.split(/\\s+/) : [];
          current = current.filter(c => classes.indexOf(c) === -1);
          self.className = current.join(" ");
        },
        contains(c) {
          const current = self.className ? self.className.split(/\\s+/) : [];
          return current.indexOf(c) !== -1;
        },
        toggle(c) {
          const current = self.className ? self.className.split(/\\s+/) : [];
          const idx = current.indexOf(c);
          if (idx === -1) {
            current.push(c);
          } else {
            current.splice(idx, 1);
          }
          self.className = current.join(" ");
        }
      };
    }
    
    getAttribute(name) {
      const val = this.attributes[name.toLowerCase()];
      return val !== undefined ? String(val) : null;
    }
    
    setAttribute(name, value) {
      this.attributes[name.toLowerCase()] = String(value);
      if (window.__onDOMChanged) window.__onDOMChanged();
    }
    
    removeAttribute(name) {
      delete this.attributes[name.toLowerCase()];
      if (window.__onDOMChanged) window.__onDOMChanged();
    }
    
    get children() {
      return this.childNodes.filter(n => n.nodeType === 1);
    }
    
    get firstElementChild() {
      return this.children[0] || null;
    }
    
    get lastElementChild() {
      const c = this.children;
      return c[c.length - 1] || null;
    }
    
    get nextElementSibling() {
      if (!this.parentNode) return null;
      const sibs = this.parentNode.children;
      const idx = sibs.indexOf(this);
      return sibs[idx + 1] || null;
    }
    
    get previousElementSibling() {
      if (!this.parentNode) return null;
      const sibs = this.parentNode.children;
      const idx = sibs.indexOf(this);
      return sibs[idx - 1] || null;
    }
    
    get textContent() {
      return this.childNodes.map(n => n.textContent).join("");
    }
    
    set textContent(val) {
      this.childNodes = [new TextNode(val)];
      this.childNodes[0].parentNode = this;
      if (window.__onDOMChanged) window.__onDOMChanged();
    }
    
    get innerHTML() {
      return this.childNodes.map(n => window.__serialize(n)).join("");
    }
    
    set innerHTML(htmlStr) {
      const parsedNodes = window.__parseHTMLFragment(htmlStr);
      this.childNodes = [];
      parsedNodes.forEach(n => {
        n.parentNode = this;
        this.childNodes.push(n);
      });
      if (window.__onDOMChanged) window.__onDOMChanged();
    }
    
    getElementById(id) {
      if (this.id === id) return this;
      for (const child of this.children) {
        const found = child.getElementById(id);
        if (found) return found;
      }
      return null;
    }
    
    getElementsByClassName(className) {
      let results = [];
      if (this.classList.contains(className)) results.push(this);
      for (const child of this.children) {
        results = results.concat(child.getElementsByClassName(className));
      }
      return results;
    }
    
    getElementsByTagName(tagName) {
      let results = [];
      const tagUpper = tagName.toUpperCase();
      const searchTag = tagName.toUpperCase();
      if (this.tagName.toUpperCase() === searchTag || tagName === "*") results.push(this);
      for (const child of this.children) {
        results = results.concat(child.getElementsByTagName(tagName));
      }
      return results;
    }

    get dataset() {
      const self = this;
      return new Proxy({}, {
        get(_, prop) {
          const attr = 'data-' + prop.replace(/([A-Z])/g, '-$1').toLowerCase();
          return self.getAttribute(attr);
        },
        set(_, prop, value) {
          const attr = 'data-' + prop.replace(/([A-Z])/g, '-$1').toLowerCase();
          self.setAttribute(attr, String(value));
          return true;
        }
      });
    }

    hasAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name.toLowerCase()); }

    closest(selector) {
      let el = this;
      while (el) {
        if (el.matches && el.matches(selector)) return el;
        el = el.parentNode;
      }
      return null;
    }

    matches(selector) {
      if (!selector) return false;
      selector = selector.trim();
      if (selector.startsWith('#')) return this.getAttribute('id') === selector.slice(1);
      if (selector.startsWith('.')) return (this.classList && this.classList.contains(selector.slice(1)));
      if (/^[a-zA-Z]/.test(selector)) return this.tagName && this.tagName.toLowerCase() === selector.toLowerCase();
      if (selector.includes('.')) return selector.split('.').filter(Boolean).every(cls => this.classList && this.classList.contains(cls));
      return false;
    }

    get outerHTML() {
      return '<' + (this.tagName || 'div') + '>' + this.innerHTML + '</' + (this.tagName || 'div') + '>';
    }

    insertAdjacentHTML(position, html) {
      const textNode = document.createTextNode(html);
      switch(position) {
        case 'beforebegin': if (this.parentNode) this.parentNode.insertBefore(textNode, this); break;
        case 'afterbegin':  this.insertBefore(textNode, this.childNodes[0]); break;
        case 'beforeend':   this.appendChild(textNode); break;
        case 'afterend':    if (this.parentNode) this.parentNode.insertBefore(textNode, this.nextSibling); break;
      }
    }

    getBoundingClientRect() {
      return { top:0, right:0, bottom:0, left:0, width:0, height:0, x:0, y:0,
               toJSON: function() { return this; } };
    }
    get offsetWidth()  { return 0; }
    get offsetHeight() { return 0; }
    get offsetTop()    { return 0; }
    get offsetLeft()   { return 0; }
    get scrollWidth()  { return 0; }
    get scrollHeight() { return 0; }
    get scrollTop()    { return 0; }
    set scrollTop(_)   {}
    get scrollLeft()   { return 0; }
    set scrollLeft(_)  {}
    get clientWidth()  { return 0; }
    get clientHeight() { return 0; }
  }
  
  class DOMImplementation {
    createHTMLDocument(title) {
      const doc = new Document();
      if (title !== undefined) {
        const titleEl = doc.createElement("title");
        titleEl.textContent = title;
        doc.head.appendChild(titleEl);
      }
      return doc;
    }
  }

  class Document extends Node {
    constructor() {
      super(9, "#document");
      this.implementation = new DOMImplementation();
      this.documentElement = new Element("html");
      this.documentElement.parentNode = this;
      this.childNodes.push(this.documentElement);
      
      this.head = new Element("head");
      this.body = new Element("body");
      
      this.documentElement.appendChild(this.head);
      this.documentElement.appendChild(this.body);
    }
    
    createElement(tagName) {
      // Notify the engine when JavaScript constructs an element the engine
      // does not implement. This is the "Canvas API required by page
      // behavior" M12.1 runtime trigger — it complements the HTML parser's
      // detection of unsupported tags in the incoming markup. The Go-side
      // hook deduplicates by kind so the report is once per page per kind.
      if (typeof window.__reportRuntimeUnsupportedFeature === "function") {
        window.__reportRuntimeUnsupportedFeature(tagName);
      }
      return new Element(tagName);
    }
    
    createTextNode(text) {
      return new TextNode(text);
    }
    
    getElementById(id) {
      return this.documentElement.getElementById(id);
    }
    
    getElementsByClassName(className) {
      return this.documentElement.getElementsByClassName(className);
    }
    
    getElementsByTagName(tagName) {
      return this.documentElement.getElementsByTagName(tagName);
    }
    
    querySelector(sel) {
      return window.__querySelectorHelper(sel);
    }
    
    querySelectorAll(sel) {
      return window.__querySelectorAllHelper(sel);
    }
  }
  
  class Event {
    constructor(type, options = {}) {
      this.type = type;
      this.bubbles = !!options.bubbles;
      this.cancelable = !!options.cancelable;
    }
  }
  
  class CustomEvent extends Event {
    constructor(type, options = {}) {
      super(type, options);
      this.detail = options.detail || null;
    }
  }
  
  window.Event = Event;
  window.CustomEvent = CustomEvent;
  globalThis.Event = Event;
  globalThis.CustomEvent = CustomEvent;
  
  window.Node = Node;
  window.Element = Element;
  window.TextNode = TextNode;
  window.Document = Document;
  
  globalThis.Node = Node;
  globalThis.Element = Element;
  globalThis.TextNode = TextNode;
  globalThis.Document = Document;
  
  window.__goosieIdCounter = 1;
  
  const document = new Document();
  window.document = document;
  globalThis.document = document;
  
  window.__serialize = function(node) {
    if (!node) return "";
    if (node.nodeType === 3) {
      return node.textContent;
    }
    if (node.nodeType === 1) {
      const tag = node.tagName.toLowerCase();
      let html = "<" + tag;
      for (const key in node.attributes) {
        html += " " + key + '="' + node.attributes[key] + '"';
      }
      html += ">";
      
      const selfClosing = ["img", "input", "br", "hr", "link", "meta"];
      if (selfClosing.indexOf(tag) !== -1) {
        return html;
      }
      
      html += node.childNodes.map(n => window.__serialize(n)).join("");
      html += "</" + tag + ">";
      return html;
    }
    if (node.nodeType === 9) {
      return window.__serialize(node.documentElement);
    }
    return "";
  };
  
  window.__onDOMChanged = function() {
    if (typeof __onDOMChangedGo === "function") {
      __onDOMChangedGo();
    }
  };

  // Bridge: JavaScript-side hook that forwards to __reportRuntimeUnsupportedFeatureGo.
  // Used by document.createElement to detect when pages build engine-unsupported
  // elements dynamically (canvas, video, audio, iframe, object, embed).
  // Unknown tag names are ignored by the Go side, so passing them through
  // is safe.
  window.__reportRuntimeUnsupportedFeature = function(tagName) {
    if (typeof __reportRuntimeUnsupportedFeatureGo === "function") {
      __reportRuntimeUnsupportedFeatureGo(tagName);
    }
  };
  
  window.__parseHTMLFragment = function(htmlStr) {
    if (typeof __parseHTMLFragmentGo === "function") {
      return __parseHTMLFragmentGo(htmlStr);
    }
    return [];
  };
  
  window.__findElementByGoosieId = function(root, id) {
    if (!root) return null;
    if (root.nodeType === 1 && root.getAttribute("__goosie_id") === id) return root;
    for (const child of root.childNodes) {
      const found = window.__findElementByGoosieId(child, id);
      if (found) return found;
    }
    return null;
  };
  
  window.__querySelectorHelper = function(selector) {
    const foundGoosieId = __querySelectorGo(selector);
    if (!foundGoosieId) return null;
    return window.__findElementByGoosieId(document.documentElement, foundGoosieId);
  };
  
  window.__querySelectorAllHelper = function(selector) {
    const ids = __querySelectorAllGo(selector);
    if (!ids) return [];
    const results = [];
    for (let i = 0; i < ids.length; i++) {
      const found = window.__findElementByGoosieId(document.documentElement, ids[i]);
      if (found) results.push(found);
    }
    return results;
  };

  // Window geometry
  window.innerWidth  = 1280;
  window.innerHeight = 800;
  window.outerWidth  = 1280;
  window.outerHeight = 800;
  window.pageXOffset = 0;
  window.pageYOffset = 0;
  window.scrollX     = 0;
  window.scrollY     = 0;
  window.scrollTo    = function(x, y) { window.scrollX = x || 0; window.scrollY = y || 0; };
  window.scrollBy    = function(dx, dy) { window.scrollX += dx || 0; window.scrollY += dy || 0; };
  window.devicePixelRatio = 1;
  window.screen = { width: 1280, height: 800, availWidth: 1280, availHeight: 800 };

  // getComputedStyle — returns inline styles
  window.getComputedStyle = function(el) {
    var inline = el && el.getAttribute ? (el.getAttribute('style') || '') : '';
    var styleMap = {};
    inline.split(';').forEach(function(pair) {
      var idx = pair.indexOf(':');
      if (idx !== -1) styleMap[pair.slice(0,idx).trim()] = pair.slice(idx+1).trim();
    });
    return {
      getPropertyValue: function(name) { return styleMap[name] || ''; },
      setProperty: function() {},
      removeProperty: function() {}
    };
  };

  // document additions
  document.readyState = 'complete';
  document.cookie = '';
  Object.defineProperty(document, 'title', {
    configurable: true,
    get: function() { return document._title || ''; },
    set: function(v) { document._title = v; }
  });
  document.createDocumentFragment = function() {
    var frag = new Element('fragment');
    frag.nodeType = 11;
    return frag;
  };

  // MutationObserver stub
  window.MutationObserver = function(callback) {
    this._callback = callback;
  };
  window.MutationObserver.prototype.observe = function(target, options) {
    var self = this;
    var prev = window.__onDOMChanged;
    window.__onDOMChanged = function() {
      if (prev) prev();
      self._callback([{ type: 'childList', target: target, addedNodes: [], removedNodes: [] }]);
    };
  };
  window.MutationObserver.prototype.disconnect = function() {};
  window.MutationObserver.prototype.takeRecords = function() { return []; };
  globalThis.MutationObserver = window.MutationObserver;

  // IntersectionObserver stub — fires immediately with isIntersecting: true
  window.IntersectionObserver = function(callback) { this._callback = callback; };
  window.IntersectionObserver.prototype.observe = function(target) {
    var self = this;
    queueMicrotask(function() {
      self._callback([{
        isIntersecting: true, intersectionRatio: 1, target: target,
        boundingClientRect: {top:0,left:0,width:0,height:0,bottom:0,right:0},
        intersectionRect: {top:0,left:0,width:0,height:0,bottom:0,right:0},
        rootBounds: null, time: 0
      }]);
    });
    if (typeof __flushMicrotasks === "function") __flushMicrotasks();
  };
  window.IntersectionObserver.prototype.unobserve = function() {};
  window.IntersectionObserver.prototype.disconnect = function() {};
  globalThis.IntersectionObserver = window.IntersectionObserver;

  // ResizeObserver stub
  window.ResizeObserver = function(callback) { this._callback = callback; };
  window.ResizeObserver.prototype.observe = function(target) {
    var self = this;
    queueMicrotask(function() {
      self._callback([{ target: target, contentRect: {width:0,height:0,top:0,left:0} }]);
    });
    if (typeof __flushMicrotasks === "function") __flushMicrotasks();
  };
  window.ResizeObserver.prototype.unobserve = function() {};
  window.ResizeObserver.prototype.disconnect = function() {};
  globalThis.ResizeObserver = window.ResizeObserver;

  // requestAnimationFrame / cancelAnimationFrame
  var _rafId = 0;
  window.requestAnimationFrame = function(fn) {
    var id = ++_rafId;
    queueMicrotask(function() { try { fn(Date.now()); } catch(e) {} });
    if (typeof __flushMicrotasks === "function") __flushMicrotasks();
    return id;
  };
  window.cancelAnimationFrame = function() {};
  globalThis.requestAnimationFrame = window.requestAnimationFrame;
  globalThis.cancelAnimationFrame = window.cancelAnimationFrame;

  // CustomEvent / Event override (more complete implementation)
  window.CustomEvent = function(type, options) {
    this.type = type || '';
    this.detail = options && options.detail;
    this.bubbles = (options && options.bubbles) || false;
    this.cancelable = (options && options.cancelable) || false;
    this.defaultPrevented = false;
    this.target = null;
    this.currentTarget = null;
  };
  window.CustomEvent.prototype.preventDefault = function() { this.defaultPrevented = true; };
  window.CustomEvent.prototype.stopPropagation = function() {};
  window.CustomEvent.prototype.stopImmediatePropagation = function() {};
  window.Event = window.CustomEvent;
  globalThis.CustomEvent = window.CustomEvent;
  globalThis.Event = window.Event;
})();
`

	_, err := r.vm.RunString(jsDOMScript)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize JS DOM: %v", err))
	}

	// Register Go helpers
	r.vm.Set("__querySelectorGo", func(selector string) string {
		if r.htmlCache == "" {
			return ""
		}
		r.serializeJSDOMToCache()
		elem, err := r.parser.QuerySelector(r.htmlCache, selector)
		if err != nil || elem == nil {
			return ""
		}
		if id, ok := elem.Attributes["__goosie_id"]; ok {
			return id
		}
		return ""
	})

	r.vm.Set("__querySelectorAllGo", func(selector string) []string {
		if r.htmlCache == "" {
			return nil
		}
		r.serializeJSDOMToCache()
		elems, err := r.parser.QuerySelectorAll(r.htmlCache, selector)
		if err != nil {
			return nil
		}
		ids := make([]string, 0, len(elems))
		for _, elem := range elems {
			if id, ok := elem.Attributes["__goosie_id"]; ok {
				ids = append(ids, id)
			}
		}
		return ids
	})

	r.vm.Set("__parseHTMLFragmentGo", func(htmlStr string) goja.Value {
		nodes, err := html.ParseFragment(strings.NewReader(htmlStr), &html.Node{
			Type:     html.ElementNode,
			Data:     "body",
			DataAtom: atom.Body,
		})
		if err != nil {
			return r.vm.NewArray()
		}
		jsNodes := make([]goja.Value, 0)
		for _, n := range nodes {
			jsNode := r.convertGoNodeToJS(n)
			if jsNode != nil {
				jsNodes = append(jsNodes, jsNode)
			}
		}
		return r.vm.ToValue(jsNodes)
	})

	r.vm.Set("__onDOMChangedGo", func() {
		if r.isPopulatingJSDOM {
			return
		}
		r.serializeJSDOMToCache()
		if r.onDOMMutation != nil {
			r.onDOMMutation(r.htmlCache)
		}
	})

	// Bridge: JavaScript calls window.__reportRuntimeUnsupportedFeature(tagName)
	// (defined in the injected DOM setup below) which forwards here. We
	// translate the tag name into the typed UnsupportedFeatureKind and
	// invoke the deduplicated report path. Unknown tags are no-ops.
	r.vm.Set("__reportRuntimeUnsupportedFeatureGo", func(tagName string) {
		var kind dom.UnsupportedFeatureKind
		switch strings.ToLower(strings.TrimSpace(tagName)) {
		case "canvas":
			kind = dom.FeatureCanvas
		case "video":
			kind = dom.FeatureVideo
		case "audio":
			kind = dom.FeatureAudio
		case "iframe":
			kind = dom.FeatureIframe
		case "object":
			kind = dom.FeatureObject
		case "embed":
			kind = dom.FeatureEmbed
		default:
			return
		}
		r.reportRuntimeUnsupportedFeature(kind)
	})
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

// SetHTMLContent sets the HTML content and populates the JS DOM tree
func (r *Runtime) SetHTMLContent(htmlStr string) {
	r.htmlCache = htmlStr

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return
	}

	r.isPopulatingJSDOM = true
	defer func() { r.isPopulatingJSDOM = false }()

	jsDoc := r.vm.Get("document").ToObject(r.vm)
	jsHead := jsDoc.Get("head").ToObject(r.vm)
	jsBody := jsDoc.Get("body").ToObject(r.vm)

	jsHead.Set("childNodes", r.vm.NewArray())
	jsBody.Set("childNodes", r.vm.NewArray())

	var findNodes func(*html.Node) (*html.Node, *html.Node, *html.Node)
	findNodes = func(n *html.Node) (*html.Node, *html.Node, *html.Node) {
		var htmlN, head, body *html.Node
		if n.Type == html.ElementNode {
			if n.Data == "html" {
				htmlN = n
			} else if n.Data == "head" {
				head = n
			} else if n.Data == "body" {
				body = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			hn, h, b := findNodes(c)
			if hn != nil {
				htmlN = hn
			}
			if h != nil {
				head = h
			}
			if b != nil {
				body = b
			}
		}
		return htmlN, head, body
	}

	htmlNode, headNode, bodyNode := findNodes(doc)

	if htmlNode != nil {
		jsDocElement := jsDoc.Get("documentElement").ToObject(r.vm)
		for _, attr := range htmlNode.Attr {
			setAttribute, _ := goja.AssertFunction(jsDocElement.Get("setAttribute"))
			setAttribute(jsDocElement, r.vm.ToValue(attr.Key), r.vm.ToValue(attr.Val))
		}
	}

	if headNode != nil {
		for _, attr := range headNode.Attr {
			setAttribute, _ := goja.AssertFunction(jsHead.Get("setAttribute"))
			setAttribute(jsHead, r.vm.ToValue(attr.Key), r.vm.ToValue(attr.Val))
		}
		r.populateJSNode(headNode, jsHead)
	}

	if bodyNode != nil {
		for _, attr := range bodyNode.Attr {
			setAttribute, _ := goja.AssertFunction(jsBody.Get("setAttribute"))
			setAttribute(jsBody, r.vm.ToValue(attr.Key), r.vm.ToValue(attr.Val))
		}
		r.populateJSNode(bodyNode, jsBody)
	}
}

// LoadHTML is an alias for SetHTMLContent, provided for test convenience.
func (r *Runtime) LoadHTML(htmlStr string) {
	r.SetHTMLContent(htmlStr)
}

// SetFetcher sets the HTTP fetcher used by the fetch() JS API.
// When set, fetch() makes real HTTP requests instead of returning mock data.
func (r *Runtime) SetFetcher(f HTTPFetcher) {
	r.fetcher = f
}

// RunScript executes JavaScript code and catches errors. Before
// delegating to the underlying Goja VM, the script body is scanned
// for JS constructs the engine does not support (currently: dynamic
// import() expressions). Detected kinds are reported via the runtime
// detection callback so the fallback layer can mark the page for
// compatibility. The scan is nil-safe and short-circuits when the
// script does not contain the word "import".
func (r *Runtime) RunScript(script string) (goja.Value, error) {
	r.ScanAndReportUnsupportedJSFeatures(script)
	val, err := r.vm.RunString(script)
	// Flush microtask queue (drives Promise .then callbacks synchronously)
	if flush := r.vm.Get("__flushMicrotasks"); flush != nil {
		if fn, ok := goja.AssertFunction(flush); ok {
			fn(goja.Undefined()) //nolint:errcheck
		}
	}
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

// ActiveTimersCount returns the number of active JS timers (setTimeout/setInterval).
func (r *Runtime) ActiveTimersCount() int {
	return len(r.timers)
}

// RunningScriptCount returns the number of currently executing scripts.
// Currently a simple boolean: 1 if a script is running, 0 otherwise.
// In the future this may track concurrent script execution.
func (r *Runtime) RunningScriptCount() int {
	// The goja runtime does not expose running state externally.
	// Return 0 as a placeholder — the value is still useful for
	// confirming whether scripts are active.
	return 0
}

// ClearJavaScriptErrors clears all JavaScript errors
func (r *Runtime) ClearJavaScriptErrors() {
	r.jsErrorsMu.Lock()
	defer r.jsErrorsMu.Unlock()
	r.jsErrors = make([]string, 0)
}

// capMethod returns a JS function that checks the given capability.
// If denied, it returns a rejected promise. If allowed, it returns a
// rejected promise indicating the API is not yet implemented.
func (r *Runtime) capMethod(cap string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if err := r.allowCapability(cap); err != nil {
			return r.vm.ToValue(r.createRejectedPromise(err.Error()))
		}
		return r.vm.ToValue(r.createRejectedPromise(cap + " API not yet implemented"))
	}
}

// capCallbackMethod returns a JS function that checks the given capability
// for callback-based APIs (e.g. getCurrentPosition). When the capability
// is denied (or the API is not implemented), it calls the error callback
// (second argument) with an error object.
func (r *Runtime) capCallbackMethod(cap string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if err := r.allowCapability(cap); err != nil {
			if len(call.Arguments) >= 2 {
				if errFn, ok := goja.AssertFunction(call.Arguments[1]); ok {
					_, _ = errFn(goja.Undefined(), r.vm.ToValue(err.Error()))
				}
			}
			return goja.Undefined()
		}
		if len(call.Arguments) >= 2 {
			if errFn, ok := goja.AssertFunction(call.Arguments[1]); ok {
				_, _ = errFn(goja.Undefined(), r.vm.ToValue(cap+" API not yet implemented"))
			}
		}
		return goja.Undefined()
	}
}

// setupNavigatorAPI extends the navigator object with capability-gated
// stubs for geolocation, clipboard, and other advanced APIs.
func (r *Runtime) setupNavigatorAPI() {
	nav := r.vm.Get("navigator")
	var navObj *goja.Object
	if nav == nil || goja.IsUndefined(nav) {
		navObj = r.vm.NewObject()
		navObj.Set("userAgent", "Goosie/1.0")
		r.vm.Set("navigator", navObj)
	} else {
		navObj = nav.ToObject(r.vm)
	}

	// Geolocation — gated behind CapabilityGeolocation
	geo := r.vm.NewObject()
	geo.Set("getCurrentPosition", r.capCallbackMethod(CapabilityGeolocation))
	geo.Set("watchPosition", r.capCallbackMethod(CapabilityGeolocation))
	geo.Set("clearWatch", r.capMethod(CapabilityGeolocation))
	navObj.Set("geolocation", geo)

	// Clipboard — gated behind CapabilityClipboard
	clip := r.vm.NewObject()
	clip.Set("readText", r.capMethod(CapabilityClipboard))
	clip.Set("writeText", r.capMethod(CapabilityClipboard))
	navObj.Set("clipboard", clip)
}

// setupNotificationAPI creates the Notification constructor gated behind
// CapabilityNotifications.
func (r *Runtime) setupNotificationAPI() {
	r.vm.Set("Notification", func(call goja.ConstructorCall) *goja.Object {
		if err := r.allowCapability(CapabilityNotifications); err != nil {
			panic(r.vm.NewTypeError(err.Error()))
		}
		panic(r.vm.NewTypeError("Notification API not yet implemented"))
	})
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
	r.attachWindowTimerAPIs(window)

	// Setup fetch API
	r.setupFetchAPI()

	// Setup window.open with popup blocking
	window.Set("open", func(call goja.FunctionCall) goja.Value {
		var urlStr, name string
		switch len(call.Arguments) {
		case 0:
			return goja.Null()
		case 1:
			urlStr = call.Arguments[0].String()
		default:
			urlStr = call.Arguments[0].String()
			name = call.Arguments[1].String()
		}

		// Enforce capability-based access control: deny window.open
		// when the navigation capability is not granted.
		if err := r.allowCapability(CapabilityNavigation); err != nil {
			r.jsConsoleWarn("window.open blocked: " + err.Error())
			return goja.Null()
		}

		// Block popups from remote origins (no user gesture tracking yet).
		if isRemoteOrigin(r.origin) {
			r.jsConsoleWarn("window.open blocked: popup from remote origin")
			return goja.Null()
		}

		if r.OnOpenWindow != nil {
			r.OnOpenWindow(urlStr, name)
		}
		// Return a minimal window proxy.
		proxy := r.vm.NewObject()
		proxy.Set("closed", false)
		proxy.Set("close", func(goja.FunctionCall) goja.Value {
			proxy.Set("closed", true)
			return goja.Undefined()
		})
		return r.vm.ToValue(proxy)
	})

	// Setup navigator with capability-gated APIs
	r.setupNavigatorAPI()

	// Setup Notification constructor
	r.setupNotificationAPI()

	// Install stubs for unsupported network/worker/PWA APIs. These
	// detect page usage and report it to the fallback layer; they
	// do NOT enforce capability policy — pages can call them, and
	// the detection is the signal we surface.
	r.setupWebSocketAPI()
	r.setupWorkerAPI()
	r.setupServiceWorkerAPI()

	r.vm.Set("window", window)
	r.vm.Set("self", window)
	r.vm.Set("top", window)
	r.vm.Set("parent", window)
	window.Set("window", window)
	window.Set("self", window)
	window.Set("top", window)
	window.Set("parent", window)
}

func (r *Runtime) attachWindowTimerAPIs(window *goja.Object) {
	for _, name := range []string{"setTimeout", "clearTimeout", "setInterval", "clearInterval"} {
		window.Set(name, r.vm.Get(name))
	}
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

	// Install a Go-level accessor property on location.href so that
	// both JS assignment (location.href = url) and Go-side Set()
	// trigger the OnNavigate callback. Uses Goja's native
	// DefineAccessorProperty, which Goja's Get/Set respect correctly.
	currentURL = "about:blank"
	getter := r.vm.ToValue(func() goja.Value {
		return r.vm.ToValue(currentURL)
	})
	setter := r.vm.ToValue(func(newURL string) {
		currentURL = newURL
		if r.OnNavigate != nil {
			r.OnNavigate(newURL)
		}
	})
	_ = location.DefineAccessorProperty("href", getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)

	// location.assign and location.replace — explicit method-based navigation.
	location.Set("assign", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		urlStr := call.Arguments[0].String()
		if r.OnNavigate != nil {
			r.OnNavigate(urlStr)
		}
		return goja.Undefined()
	})
	location.Set("replace", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		urlStr := call.Arguments[0].String()
		if r.OnNavigate != nil {
			r.OnNavigate(urlStr)
		}
		return goja.Undefined()
	})
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
// storageMethod wraps a localStorage/sessionStorage method with a
// CapabilityStorage check. When the capability is denied the method
// returns null/undefined. The check is performed on every call so that
// it works regardless of when a ScriptEnforcer is attached.
func (r *Runtime) storageMethod(fn func(goja.FunctionCall) goja.Value) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if err := r.allowCapability(CapabilityStorage); err != nil {
			return goja.Null()
		}
		return fn(call)
	}
}

func (r *Runtime) setupLocalStorageAPI() {
	localStorage := r.vm.NewObject()

	// getItem
	localStorage.Set("getItem", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}

		key := call.Arguments[0].String()
		value, exists := r.localStorageGet(key)
		if !exists {
			return goja.Null()
		}

		// The in-memory fallback retains its legacy version prefix.
		if r.localStorageAdapter == nil && strings.HasPrefix(value, "v1:") {
			return r.vm.ToValue(strings.TrimPrefix(value, "v1:"))
		}

		return r.vm.ToValue(value)
	}))

	// setItem - with validation
	localStorage.Set("setItem", r.storageMethod(func(call goja.FunctionCall) goja.Value {
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

		r.localStorageSet(key, value)

		return goja.Undefined()
	}))

	// removeItem
	localStorage.Set("removeItem", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		key := call.Arguments[0].String()
		r.localStorageRemove(key)

		return goja.Undefined()
	}))

	// clear - remove all items
	localStorage.Set("clear", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		r.localStorageClear()
		return goja.Undefined()
	}))

	// key - get key at index
	localStorage.Set("key", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}

		index := int(call.Arguments[0].ToInteger())
		keys := r.localStorageKeys()

		if index < 0 || index >= len(keys) {
			return goja.Null()
		}

		return r.vm.ToValue(keys[index])
	}))

	// length property
	localStorage.Set("length", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.localStorageKeys()))
	}))

	r.vm.Set("localStorage", localStorage)
}

func (r *Runtime) localStorageGet(key string) (string, bool) {
	if r.localStorageAdapter != nil {
		return r.localStorageAdapter.Get(r.origin, key)
	}
	value, ok := r.localStorage[key]
	return value, ok
}

func (r *Runtime) localStorageSet(key, value string) {
	if r.localStorageAdapter != nil {
		_ = r.localStorageAdapter.Set(r.origin, key, value)
		return
	}
	r.localStorage[key] = "v1:" + value
}

func (r *Runtime) localStorageRemove(key string) {
	if r.localStorageAdapter != nil {
		_ = r.localStorageAdapter.Remove(r.origin, key)
		return
	}
	delete(r.localStorage, key)
}

func (r *Runtime) localStorageClear() {
	if r.localStorageAdapter != nil {
		_ = r.localStorageAdapter.Clear(r.origin)
		return
	}
	r.localStorage = make(map[string]string)
}

func (r *Runtime) localStorageKeys() []string {
	if r.localStorageAdapter != nil {
		return r.localStorageAdapter.Keys(r.origin)
	}
	keys := make([]string, 0, len(r.localStorage))
	for key := range r.localStorage {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// setupSessionStorageAPI configures sessionStorage
func (r *Runtime) setupSessionStorageAPI() {
	sessionStorage := r.vm.NewObject()

	// getItem
	sessionStorage.Set("getItem", r.storageMethod(func(call goja.FunctionCall) goja.Value {
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
	}))

	// setItem - with session schema support
	sessionStorage.Set("setItem", r.storageMethod(func(call goja.FunctionCall) goja.Value {
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
	}))

	// removeItem
	sessionStorage.Set("removeItem", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		key := call.Arguments[0].String()
		delete(r.sessionStorage, key)

		return goja.Undefined()
	}))

	// clear - remove all items
	sessionStorage.Set("clear", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		r.sessionStorage = make(map[string]string)
		return goja.Undefined()
	}))

	// key - get key at index
	sessionStorage.Set("key", r.storageMethod(func(call goja.FunctionCall) goja.Value {
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
	}))

	// length property
	sessionStorage.Set("length", r.storageMethod(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.sessionStorage))
	}))

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
			select {
			case <-timer.Cancel:
				timer.mu.Unlock()
				return
			default:
				timer.mu.Unlock()
				if r.enqueueTask != nil {
					r.enqueueTask(func() {
						timer.mu.Lock()
						defer timer.mu.Unlock()
						select {
						case <-timer.Cancel:
							return
						default:
							callback(goja.Undefined()) //nolint:errcheck
							delete(r.timers, timerID)
						}
					})
				} else {
					timer.mu.Lock()
					defer timer.mu.Unlock()
					callback(goja.Undefined()) //nolint:errcheck
					delete(r.timers, timerID)
				}
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
					select {
					case <-timer.Cancel:
						timer.mu.Unlock()
						return
					default:
						timer.mu.Unlock()
						if r.enqueueTask != nil {
							r.enqueueTask(func() {
								timer.mu.Lock()
								defer timer.mu.Unlock()
								select {
								case <-timer.Cancel:
									return
								default:
									callback(goja.Undefined()) //nolint:errcheck
								}
							})
						} else {
							timer.mu.Lock()
							defer timer.mu.Unlock()
							callback(goja.Undefined()) //nolint:errcheck
						}
					}
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

// setupFetchAPI configures fetch API with real HTTP support when a fetcher is set
func (r *Runtime) setupFetchAPI() {
	r.vm.Set("fetch", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue(r.createRejectedPromise("fetch requires a URL"))
		}

		urlStr := call.Arguments[0].String()

		// Enforce capability-based access control: deny fetch when the
		// network capability is not granted.
		if err := r.allowCapability(CapabilityNetwork); err != nil {
			return r.vm.ToValue(r.createRejectedPromise(err.Error()))
		}

		// Enforce the default file-access policy: remote origins must not
		// fetch file:// URLs.
		if err := checkFileFetchAccess(r.origin, urlStr); err != nil {
			return r.vm.ToValue(r.createRejectedPromise(err.Error()))
		}

		// Create a promise-like object
		promise := r.vm.NewObject()
		var callbackMu sync.Mutex
		var catchCallback goja.Callable

		// then method
		promise.Set("then", func(thenCall goja.FunctionCall) goja.Value {
			if len(thenCall.Arguments) == 0 {
				return promise
			}

			onSuccess, ok := goja.AssertFunction(thenCall.Arguments[0])
			if !ok {
				return promise
			}

			go func() {
				if r.fetcher != nil {
					// Real HTTP fetch
					body, err := r.fetcher.Fetch(urlStr)
					if err != nil {
						callbackMu.Lock()
						cc := catchCallback
						callbackMu.Unlock()
						if cc != nil {
							if r.enqueueTask != nil {
								r.enqueueTask(func() {
									errObj := r.vm.NewObject()
									errObj.Set("message", err.Error())
									cc(goja.Undefined(), errObj) //nolint:errcheck
								})
							} else {
								errObj := r.vm.NewObject()
								errObj.Set("message", err.Error())
								cc(goja.Undefined(), errObj) //nolint:errcheck
							}
						}
						return
					}

					if r.enqueueTask != nil {
						r.enqueueTask(func() {
							response := r.createFetchResponse(urlStr, body)
							onSuccess(goja.Undefined(), response) //nolint:errcheck
						})
					} else {
						response := r.createFetchResponse(urlStr, body)
						onSuccess(goja.Undefined(), response) //nolint:errcheck
					}
				} else {
					// Mock fetch (no fetcher set – used in tests without a live server)
					if r.enqueueTask != nil {
						r.enqueueTask(func() {
							response := r.vm.NewObject()
							response.Set("ok", true)
							response.Set("status", 200)
							response.Set("statusText", "OK")
							response.Set("url", urlStr)

							// json method
							response.Set("json", func(jsonCall goja.FunctionCall) goja.Value {
								jsonPromise := r.vm.NewObject()
								jsonPromise.Set("then", func(jsonThenCall goja.FunctionCall) goja.Value {
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

							onSuccess(goja.Undefined(), response) //nolint:errcheck
						})
					} else {
						response := r.vm.NewObject()
						response.Set("ok", true)
						response.Set("status", 200)
						response.Set("statusText", "OK")
						response.Set("url", urlStr)

						// json method
						response.Set("json", func(jsonCall goja.FunctionCall) goja.Value {
							jsonPromise := r.vm.NewObject()
							jsonPromise.Set("then", func(jsonThenCall goja.FunctionCall) goja.Value {
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

						onSuccess(goja.Undefined(), response) //nolint:errcheck
					}
				}
			}()

			return promise
		})

		// catch method
		promise.Set("catch", func(catchCall goja.FunctionCall) goja.Value {
			if len(catchCall.Arguments) > 0 {
				if fn, ok := goja.AssertFunction(catchCall.Arguments[0]); ok {
					callbackMu.Lock()
					catchCallback = fn
					callbackMu.Unlock()
				}
			}
			return promise
		})

		return promise
	})
}

// createFetchResponse builds a JS response object from a real HTTP response body.
func (r *Runtime) createFetchResponse(urlStr, body string) *goja.Object {
	response := r.vm.NewObject()
	response.Set("ok", true)
	response.Set("status", 200)
	response.Set("statusText", "OK")
	response.Set("url", urlStr)

	bodyStr := body

	// text() method – returns a promise that resolves to the raw response string
	response.Set("text", func(textCall goja.FunctionCall) goja.Value {
		textPromise := r.vm.NewObject()
		textPromise.Set("then", func(thenCall goja.FunctionCall) goja.Value {
			if len(thenCall.Arguments) > 0 {
				if fn, ok := goja.AssertFunction(thenCall.Arguments[0]); ok {
					fn(goja.Undefined(), r.vm.ToValue(bodyStr)) //nolint:errcheck
				}
			}
			return textPromise
		})
		textPromise.Set("catch", func(catchCall goja.FunctionCall) goja.Value {
			return textPromise
		})
		return textPromise
	})

	// json() method – parses the response body as JSON
	response.Set("json", func(jsonCall goja.FunctionCall) goja.Value {
		jsonPromise := r.vm.NewObject()
		var jsonCatchCallback goja.Callable
		jsonPromise.Set("then", func(thenCall goja.FunctionCall) goja.Value {
			if len(thenCall.Arguments) > 0 {
				if fn, ok := goja.AssertFunction(thenCall.Arguments[0]); ok {
					var result interface{}
					if err := json.Unmarshal([]byte(bodyStr), &result); err == nil {
						fn(goja.Undefined(), r.vm.ToValue(result)) //nolint:errcheck
					} else if jsonCatchCallback != nil {
						errObj := r.vm.NewObject()
						errObj.Set("message", "JSON parse error: "+err.Error())
						jsonCatchCallback(goja.Undefined(), errObj) //nolint:errcheck
					}
				}
			}
			return jsonPromise
		})
		jsonPromise.Set("catch", func(catchCall goja.FunctionCall) goja.Value {
			if len(catchCall.Arguments) > 0 {
				if fn, ok := goja.AssertFunction(catchCall.Arguments[0]); ok {
					jsonCatchCallback = fn
				}
			}
			return jsonPromise
		})
		return jsonPromise
	})

	return response
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

// SetDOMMutationCallback sets a callback for when the JS DOM tree is mutated
func (r *Runtime) SetDOMMutationCallback(callback func(string)) {
	r.onDOMMutation = callback
}

func (r *Runtime) serializeJSDOMToCache() {
	serializeFunc, _ := goja.AssertFunction(r.vm.Get("window").ToObject(r.vm).Get("__serialize"))
	htmlVal, err := serializeFunc(goja.Undefined(), r.vm.Get("document"))
	if err == nil {
		r.htmlCache = htmlVal.String()
	}
}

func (r *Runtime) populateJSNode(goNode *html.Node, jsNode *goja.Object) {
	for c := goNode.FirstChild; c != nil; c = c.NextSibling {
		var jsChild goja.Value
		jsDoc := r.vm.Get("document").ToObject(r.vm)

		if c.Type == html.TextNode {
			createTextNode, _ := goja.AssertFunction(jsDoc.Get("createTextNode"))
			jsChild, _ = createTextNode(jsDoc, r.vm.ToValue(c.Data))
		} else if c.Type == html.ElementNode {
			createElement, _ := goja.AssertFunction(jsDoc.Get("createElement"))
			jsChild, _ = createElement(jsDoc, r.vm.ToValue(c.Data))

			jsChildObj := jsChild.ToObject(r.vm)
			for _, attr := range c.Attr {
				setAttribute, _ := goja.AssertFunction(jsChildObj.Get("setAttribute"))
				setAttribute(jsChildObj, r.vm.ToValue(attr.Key), r.vm.ToValue(attr.Val))
			}

			// Recurse
			r.populateJSNode(c, jsChildObj)
		}

		if jsChild != nil && jsChild != goja.Null() {
			appendChild, _ := goja.AssertFunction(jsNode.Get("appendChild"))
			appendChild(jsNode, jsChild)
		}
	}
}

func (r *Runtime) convertGoNodeToJS(n *html.Node) goja.Value {
	if n == nil {
		return goja.Null()
	}

	jsDoc := r.vm.Get("document").ToObject(r.vm)
	if n.Type == html.TextNode {
		createTextNode, _ := goja.AssertFunction(jsDoc.Get("createTextNode"))
		jsNode, _ := createTextNode(jsDoc, r.vm.ToValue(n.Data))
		return jsNode
	} else if n.Type == html.ElementNode {
		createElement, _ := goja.AssertFunction(jsDoc.Get("createElement"))
		jsNode, _ := createElement(jsDoc, r.vm.ToValue(n.Data))
		jsNodeObj := jsNode.ToObject(r.vm)

		for _, attr := range n.Attr {
			setAttribute, _ := goja.AssertFunction(jsNodeObj.Get("setAttribute"))
			setAttribute(jsNodeObj, r.vm.ToValue(attr.Key), r.vm.ToValue(attr.Val))
		}

		// Recursively populate children
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			childJS := r.convertGoNodeToJS(c)
			if childJS != nil && childJS != goja.Null() {
				appendChild, _ := goja.AssertFunction(jsNodeObj.Get("appendChild"))
				appendChild(jsNodeObj, childJS)
			}
		}
		return jsNode
	}
	return goja.Null()
}

// checkFileFetchAccess checks whether a fetch to targetURL should be
// allowed from the given origin (string like "https://example.com").
// It returns nil when allowed, or an error explaining why it was blocked.
func checkFileFetchAccess(origin, targetURL string) error {
	if origin == "" {
		return nil
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		return nil
	}
	// Only block file:// fetches from remote (non-file) origins.
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil
	}
	if target.Scheme != "file" {
		return nil
	}
	if originURL.Scheme == "file" {
		return nil
	}
	return errFetchFileAccessDenied
}

// errFetchFileAccessDenied is returned when a remote origin tries to fetch a
// file:// URL through the JS fetch() API.
var errFetchFileAccessDenied = fmt.Errorf("fetch: local file access denied from remote origin")

// isRemoteOrigin returns true if the origin is a remote (http/https) URL.
func isRemoteOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://") || strings.HasPrefix(origin, "https://")
}

// jsConsoleWarn logs a warning message to the JS console from Go code.
func (r *Runtime) jsConsoleWarn(msg string) {
	r.consoleMu.Lock()
	r.consoleMessages = append(r.consoleMessages, ConsoleMessage{
		Level:     "warn",
		Message:   msg,
		Timestamp: time.Now(),
	})
	r.consoleMu.Unlock()
	fmt.Println("[WARN] " + msg)
}

// ValueResult represents a serialized JavaScript value.
type ValueResult struct {
	Type  string
	Value interface{}
}

// SerializeValue converts a Goja value to a serializable result with size limits.
func (r *Runtime) SerializeValue(value goja.Value, maxBytes int) ValueResult {
	if value == nil || value.IsUndefined() {
		return ValueResult{Type: "undefined", Value: nil}
	}

	if value.IsNull() {
		return ValueResult{Type: "null", Value: nil}
	}

	if value.IsBool() {
		return ValueResult{Type: "boolean", Value: value.Bool()}
	}

	if value.IsNumber() {
		if f, err := value.ToFloat64(); err == nil {
			if f == float64(int64(f)) {
				return ValueResult{Type: "number", Value: int64(f)}
			}
			return ValueResult{Type: "number", Value: f}
		}
		return ValueResult{Type: "number", Value: value.ToInteger()}
	}

	if value.IsString() {
		s := value.String()
		if len(s) > maxBytes {
			s = s[:maxBytes] + "...[truncated]"
		}
		return ValueResult{Type: "string", Value: s}
	}

	// Objects and arrays
	if value.IsObject() {
		obj := value.ToObject(r.vm)
		if obj != nil && (obj.Class() == "Array" || obj.Class() == "Object") {
			exported := obj.Export()
			return ValueResult{Type: "object", Value: exported}
		}
		s := value.String()
		if len(s) > maxBytes {
			s = s[:maxBytes] + "...[truncated]"
		}
		return ValueResult{Type: "object", Value: s}
	}

	// Functions
	s := value.String()
	if len(s) > maxBytes {
		s = s[:maxBytes] + "...[truncated]"
	}
	return ValueResult{Type: "function", Value: s}
}

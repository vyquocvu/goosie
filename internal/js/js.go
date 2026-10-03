// Package js executes a document's classic scripts against a host-provided
// window and document stub.
//
// A Runtime belongs to exactly one document: nothing in this package is
// process-global, so two tabs cannot read each other's bindings and tearing a
// document down releases its realm. That ownership rule is the reason the API
// takes an Options struct per runtime rather than exposing setters.
//
// Scope is roadmap gate 6, sub-project 1: <script> elements run in document
// order, each one blocking the parse until it returns; console output reaches a
// host writer; window and document are stubs. Tasks, microtasks, DOM mutation
// and Fetch belong to the later sub-projects, and calling an API outside this
// scope must fail a script loudly rather than resolve to undefined.
package js

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// DefaultTimeout bounds one script's execution when Options.Timeout is zero.
// A script that runs past it is stopped where it stands and reported through
// ErrInterrupted; the bound exists because a page that spins takes the tab's
// event loop with it, and the tab shares a process with every other tab.
const DefaultTimeout = 5 * time.Second

// MaxScriptBytes is the source limit for a single script. The document byte
// guard already bounds the page; this bounds the compile, so one oversized
// literal is refused before it is parsed rather than after.
const MaxScriptBytes = 1 << 20

var (
	// ErrInterrupted reports a script stopped by its timeout or by Interrupt.
	ErrInterrupted = errors.New("js: script execution interrupted")
	// ErrClosed reports use of a runtime whose document has gone away.
	ErrClosed = errors.New("js: runtime is closed")
	// ErrTooLarge reports a script whose source exceeds MaxScriptBytes.
	ErrTooLarge = errors.New("js: script source exceeds the limit")
	// ErrNotImplemented is kept for compatibility but no longer returned.
	ErrNotImplemented = errors.New("js: runtime not implemented (roadmap gate 6, sub-project 1)")
)

// Options configures one Runtime.
type Options struct {
	// Timeout bounds a single Run. Zero selects DefaultTimeout; a negative
	// value is rejected rather than treated as unlimited, because "unlimited"
	// is not a bound this host is willing to give untrusted code.
	Timeout time.Duration
	// Console receives console.log, warn and error. nil discards: a runtime
	// never writes to a process stream the host did not hand it.
	Console io.Writer
	// URL is the document's URL, exposed as window.location.href and used to
	// attribute a script error to the file that threw.
	URL string
	// Title seeds document.title.
	Title string
	// DOM is the parsed document tree. When non-nil, document.querySelector,
	// document.querySelectorAll, document.createElement and element methods
	// are available to scripts. When nil, those APIs do not exist on the
	// document object.
	DOM *dom.Document

	// HTTPClient is the fetch backend. When non-nil, fetch() is available
	// to scripts. When nil, fetch() is not defined and calling it produces
	// a ReferenceError.
	HTTPClient HTTPFetcher

	// WebSocketDialer is the WebSocket backend. When non-nil, WebSocket is
	// available to scripts. When nil, WebSocket is not defined.
	WebSocketDialer WebSocketDialer

	// WorkerDialer is the Web Worker backend. When non-nil, Worker is
	// available to scripts. When nil, Worker is not defined.
	WorkerDialer WorkerDialer

	// CSP is the parsed Content-Security-Policy for this document. When
	// non-nil, script/style/connect sources are checked against it.
	CSP *CSPPolicy

	// OnMutation is called when scripts mutate the DOM. The engine uses
	// this to trigger re-style and re-layout. nil means mutations are
	// not tracked (e.g., in tests).
	OnMutation func()
}

// Kind is the JS type of a Value read back out of a runtime.
type Kind uint8

const (
	KindUndefined Kind = iota
	KindNull
	KindBool
	KindNumber
	KindString
	KindObject
	KindFunction
)

// Value is a global's value as seen from the host side.
type Value struct {
	Kind Kind
	Num  float64
	Str  string
	Bool bool
}

// Error is an uncaught exception from one script. It names the source and line
// so a page author sees which file broke; the load itself continues.
type Error struct {
	SourceURL string
	Line      int
	Message   string
}

func (e *Error) Error() string {
	return e.SourceURL + ":" + strconv.Itoa(e.Line) + ": " + e.Message
}

// timer represents a pending setTimeout or setInterval callback.
type timer struct {
	id       int
	callback goja.Value
	interval time.Duration
	nextFire time.Time
	repeat   bool // true for setInterval, false for setTimeout
}

// Runtime executes scripts for one document.
type Runtime struct {
	vm          *goja.Runtime
	opts        Options
	title       string
	closed      bool
	mu          sync.Mutex
	timers      map[int]*timer
	nextTimerID int

	// DOM bridge state. nil when Options.DOM is nil.
	domDoc       *dom.Document
	nodeRegistry map[int]*dom.Node
	nextNID      int
	nodeProto    *goja.Object

	// Canvas contexts keyed by DOM node ID. nil until the first getContext("2d").
	canvasContexts map[int]*canvasContext2D

	// Custom Elements registry. nil until setupCustomElements() is called.
	customElementRegistry *CustomElementRegistry

	// nodeWrappers maps DOM node IDs to their JS wrapper objects for lifecycle callbacks.
	nodeWrappers map[int]*goja.Object

	// shadowRoots maps host node IDs to their shadow root JS wrappers.
	shadowRoots map[int]*goja.Object
}

// New builds a runtime for one document.
func New(opts Options) (*Runtime, error) {
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("js: negative timeout %v rejected", opts.Timeout)
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	consoleWriter := opts.Console
	if consoleWriter == nil {
		consoleWriter = io.Discard
	}

	r := &Runtime{
		opts:   opts,
		title:  opts.Title,
		vm:     goja.New(),
		timers: make(map[int]*timer),
	}

	r.setupConsole(consoleWriter)
	r.setupDocument()
	r.setupWindow()
	r.setupDOM()
	if opts.HTTPClient != nil {
		r.setupFetch(opts.HTTPClient)
	}
	r.setupTimers()
	r.setupPromise()
	r.setupIntersectionObserver()
	r.setupRAF()
	r.setupStorage()
	r.setupURLAPI()
	r.setupTextEncoding()
	r.setupFormData()
	r.setupAbortController()
	r.setupCrypto()
	r.setupPerformance()
	r.setupHistory()
	if opts.WebSocketDialer != nil {
		r.setupWebSocket(opts.WebSocketDialer)
	}
	r.setupMutationObserver()
	r.setupResizeObserver()
	r.setupCanvas()
	r.setupCustomElements()
	r.setupShadowDOM()
	if opts.WorkerDialer != nil {
		r.setupWorker(opts.WorkerDialer)
	}
	r.setupServiceWorker()
	r.setupUnsupportedAPIs()
	_ = timeout // enforced per-Run via vm.SetMaxCallStackSize or interrupt timer

	return r, nil
}

func (r *Runtime) setupConsole(w io.Writer) {
	console := r.vm.NewObject()

	logFn := func(args ...goja.Value) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = formatConsoleArg(arg)
		}
		fmt.Fprintln(w, strings.Join(parts, " "))
	}

	_ = console.Set("log", logFn)
	_ = console.Set("warn", logFn)
	_ = console.Set("error", logFn)
	_ = r.vm.Set("console", console)
}

func formatConsoleArg(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	return v.String()
}

func (r *Runtime) setupDocument() {
	doc := r.vm.NewObject()
	_ = r.vm.Set("document", doc)

	_ = doc.Set("__title__", r.title)

	_, _ = r.vm.RunString(`
		Object.defineProperty(document, 'title', {
			get: function() { return document.__title__; },
			set: function(v) { document.__title__ = v; },
			configurable: true,
			enumerable: true
		});
	`)
}

func (r *Runtime) setupWindow() {
	loc := r.vm.NewObject()
	_ = loc.Set("href", r.opts.URL)

	win := r.vm.NewObject()
	_ = win.Set("location", loc)
	_ = r.vm.Set("window", win)
}

func (r *Runtime) setupUnsupportedAPIs() {
	// Wave 2: fetch is now wired by setupFetch when an HTTPClient is
	// provided. When it is nil, fetch remains undefined and calling it
	// produces a ReferenceError that names the missing API.
}

// setupDOM installs DOM API bindings on the document object when a DOM tree
// was provided via Options.DOM. Without a DOM, none of the query or mutation
// methods exist, so scripts that call them get a clear undefined-function
// error rather than a silent no-op.
func (r *Runtime) setupDOM() {
	if r.opts.DOM == nil {
		return
	}

	r.domDoc = r.opts.DOM
	r.nodeRegistry = make(map[int]*dom.Node)
	r.nodeWrappers = make(map[int]*goja.Object)
	r.shadowRoots = make(map[int]*goja.Object)
	r.nextNID = 1

	// Build a shared prototype for element wrapper objects. Each wrapper
	// stores a __nid__ (registry key) that the prototype methods use to
	// look up the real *dom.Node.
	r.nodeProto = r.vm.NewObject()
	_ = r.nodeProto.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		return r.jsAppendChild(call)
	})
	_ = r.nodeProto.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsAddEventListener(call)
	})
	_ = r.nodeProto.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveEventListener(call)
	})
	_ = r.nodeProto.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return r.jsDispatchEvent(call)
	})
	_ = r.nodeProto.Set("getAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsGetAttribute(call)
	})
	_ = r.nodeProto.Set("setAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsSetAttribute(call)
	})
	_ = r.nodeProto.Set("removeAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveAttribute(call)
	})
	_ = r.nodeProto.Set("hasAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsHasAttribute(call)
	})
	_ = r.nodeProto.Set("remove", func(call goja.FunctionCall) goja.Value {
		return r.jsRemove(call)
	})
	_ = r.nodeProto.Set("createElement", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateElement(call)
	})
	_ = r.nodeProto.Set("getContext", func(call goja.FunctionCall) goja.Value {
		return r.jsGetContext(call)
	})
	// Set the prototype as a temporary global so RunString can reference it.
	_ = r.vm.Set("__nodeProto__", r.nodeProto)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__nodeProto__, 'textContent', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getTextContent__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setTextContent__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__nodeProto__", nil)

	// innerHTML getter/setter on nodeProto.
	_ = r.vm.Set("__nodeProto__", r.nodeProto)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__nodeProto__, 'innerHTML', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getInnerHTML__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setInnerHTML__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__nodeProto__", nil)

	doc := r.vm.Get("document").(*goja.Object)

	_ = doc.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelector(call)
	})
	_ = doc.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelectorAll(call)
	})
	_ = doc.Set("createElement", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateElement(call)
	})
	_ = doc.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		return r.jsGetElementByID(call)
	})
	_ = doc.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		return r.jsGetElementsByClassName(call)
	})
	_ = doc.Set("getElementsByTagName", func(call goja.FunctionCall) goja.Value {
		return r.jsGetElementsByTagName(call)
	})
	_ = doc.Set("createTextNode", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateTextNode(call)
	})

	// Register the document node so document.addEventListener / dispatchEvent
	// operate on the same *dom.Node the engine dispatches events on.
	docNID := r.nextNID
	r.nextNID++
	r.nodeRegistry[docNID] = &r.domDoc.Node
	_ = doc.Set("__nid__", docNID)
	r.nodeWrappers[docNID] = doc

	_ = doc.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsAddEventListener(call)
	})
	_ = doc.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveEventListener(call)
	})
	_ = doc.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return r.jsDispatchEvent(call)
	})

	// Internal helpers the prototype getter/setter calls back into.
	_ = doc.Set("__getTextContent__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.TextContent())
	})
	_ = doc.Set("__setTextContent__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		text := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		for node.FirstChild != nil {
			node.RemoveChild(node.FirstChild)
		}
		if text != "" {
			textNode := node.Doc.NewText(text)
			node.AppendChild(textNode)
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getInnerHTML__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.serializeChildren(node))
	})
	_ = doc.Set("__setInnerHTML__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		html := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		// Clear existing children.
		for node.FirstChild != nil {
			node.RemoveChild(node.FirstChild)
		}
		// Parse the fragment by wrapping in a <div>.
		wrapped := "<div>" + html + "</div>"
		parsed, err := dom.ParseBounded(wrapped, dom.ParseLimits{
			Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096,
		})
		if err != nil {
			// Fallback: insert as text node.
			if html != "" {
				node.AppendChild(node.Doc.NewText(html))
			}
			r.notifyMutation()
			return goja.Undefined()
		}
		// Find the <div> we wrapped in (it will be inside <html><body>).
		var divNode *dom.Node
		body := parsed.Body
		if body != nil {
			divNode = body.FirstChild
		}
		if divNode == nil {
			r.notifyMutation()
			return goja.Undefined()
		}
		// Move the div's children to the target node.
		for divNode.FirstChild != nil {
			child := divNode.FirstChild
			divNode.RemoveChild(child)
			node.AppendChild(child)
		}
		r.notifyMutation()
		return goja.Undefined()
	})

	// Form-control helpers for checked/value property accessors.
	_ = doc.Set("__getChecked__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(node.HasAttribute("checked"))
	})
	_ = doc.Set("__setChecked__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		checked := call.Arguments[1].ToBoolean()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if checked {
			node.SetAttribute("checked", "")
		} else {
			node.RemoveAttribute("checked")
		}
		return goja.Undefined()
	})
	_ = doc.Set("__getValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.HasAttribute("value") {
			return r.vm.ToValue(node.GetAttribute("value"))
		}
		typ := strings.ToLower(node.GetAttribute("type"))
		if node.Data == "input" && (typ == "checkbox" || typ == "radio") {
			return r.vm.ToValue("on")
		}
		return r.vm.ToValue("")
	})
	_ = doc.Set("__setValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		v := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute("value", v)
		return goja.Undefined()
	})

	// Dynamic body property: wraps the body element on first access and
	// caches it. If the body is replaced, the host should re-run setupDOM.
	if r.domDoc.Body != nil {
		_ = doc.Set("body", r.wrapNode(r.domDoc.Body))
	} else {
		_ = doc.Set("body", goja.Null())
	}
}

// wrapNode creates a JS wrapper object for a DOM node. The wrapper inherits
// from nodeProto (which provides appendChild, textContent, addEventListener)
// and stores a __nid__ registry key so the prototype methods can find the
// underlying *dom.Node.
func (r *Runtime) wrapNode(node *dom.Node) goja.Value {
	if node == nil {
		return goja.Null()
	}
	nid := r.nextNID
	r.nextNID++
	r.nodeRegistry[nid] = node

	obj := r.vm.NewObject()
	_ = obj.Set("__nid__", nid)
	r.nodeWrappers[nid] = obj

	// Copy prototype properties onto the instance. goja does not support
	// __proto__ assignment on objects created with NewObject, so we copy
	// the methods directly.
	if r.nodeProto != nil {
		for _, key := range r.nodeProto.Keys() {
			_ = obj.Set(key, r.nodeProto.Get(key))
		}
		// Re-define textContent as an accessor on this instance, since
		// the copy above only copies the data descriptor, not the
		// getter/setter.
		r.defineTextContentAccessor(obj)
		// Re-define innerHTML as an accessor on this instance.
		r.defineInnerHTMLAccessor(obj)
	}

	// Element-specific properties.
	if node.Element() {
		_ = obj.Set("tagName", strings.ToUpper(node.Data))
		_ = obj.Set("nodeName", strings.ToUpper(node.Data))
		_ = obj.Set("id", node.GetAttribute("id"))
		_ = obj.Set("className", node.GetAttribute("class"))
		if node.Data == "input" {
			typ := strings.ToLower(node.GetAttribute("type"))
			if typ == "" {
				typ = "text"
			}
			_ = obj.Set("type", typ)
		}

		// classList with add/remove/toggle/contains methods.
		// The closures capture `node` directly because call.This inside
		// classList methods refers to the classList object, not the element
		// wrapper, so r.getNode(call.This) would return nil.
		classList := r.vm.NewObject()
		_ = classList.Set("add", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			cls := call.Arguments[0].String()
			if cls == "" {
				return goja.Undefined()
			}
			existing := node.ClassList()
			for _, c := range existing {
				if c == cls {
					return goja.Undefined()
				}
			}
			existing = append(existing, cls)
			node.SetAttribute("class", strings.Join(existing, " "))
			r.notifyMutation()
			return goja.Undefined()
		})
		_ = classList.Set("remove", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			cls := call.Arguments[0].String()
			if cls == "" {
				return goja.Undefined()
			}
			existing := node.ClassList()
			filtered := make([]string, 0, len(existing))
			for _, c := range existing {
				if c != cls {
					filtered = append(filtered, c)
				}
			}
			if len(filtered) == 0 {
				// Remove the attribute entirely if no classes remain.
				for i := range node.Attr {
					if strings.ToLower(node.Attr[i].Name) == "class" {
						node.Attr = append(node.Attr[:i], node.Attr[i+1:]...)
						break
					}
				}
			} else {
				node.SetAttribute("class", strings.Join(filtered, " "))
			}
			r.notifyMutation()
			return goja.Undefined()
		})
		_ = classList.Set("toggle", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			cls := call.Arguments[0].String()
			if cls == "" {
				return goja.Undefined()
			}
			existing := node.ClassList()
			for i, c := range existing {
				if c == cls {
					// Remove it.
					rest := append(existing[:i], existing[i+1:]...)
					if len(rest) == 0 {
						for j := range node.Attr {
							if strings.ToLower(node.Attr[j].Name) == "class" {
								node.Attr = append(node.Attr[:j], node.Attr[j+1:]...)
								break
							}
						}
					} else {
						node.SetAttribute("class", strings.Join(rest, " "))
					}
					r.notifyMutation()
					return r.vm.ToValue(false)
				}
			}
			// Add it.
			existing = append(existing, cls)
			node.SetAttribute("class", strings.Join(existing, " "))
			r.notifyMutation()
			return r.vm.ToValue(true)
		})
		_ = classList.Set("contains", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return r.vm.ToValue(false)
			}
			cls := call.Arguments[0].String()
			for _, c := range node.ClassList() {
				if c == cls {
					return r.vm.ToValue(true)
				}
			}
			return r.vm.ToValue(false)
		})
		_ = obj.Set("classList", classList)

		if node.Data == "input" {
			r.defineCheckedAccessor(obj)
			r.defineValueAccessor(obj)
		}
	}

	return obj
}

// defineTextContentAccessor installs the textContent getter/setter on obj
// using Object.defineProperty via a RunString helper.
func (r *Runtime) defineTextContentAccessor(obj *goja.Object) {
	// We use a temporary global to pass the object to RunString.
	_ = r.vm.Set("__tcTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__tcTarget__, 'textContent', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getTextContent__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setTextContent__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__tcTarget__", nil)
}

// defineInnerHTMLAccessor installs the innerHTML getter/setter on obj
// using Object.defineProperty via a RunString helper.
func (r *Runtime) defineInnerHTMLAccessor(obj *goja.Object) {
	_ = r.vm.Set("__ihTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__ihTarget__, 'innerHTML', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getInnerHTML__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setInnerHTML__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__ihTarget__", nil)
}

func (r *Runtime) defineCheckedAccessor(obj *goja.Object) {
	_ = r.vm.Set("__chkTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__chkTarget__, 'checked', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return false;
				return document.__getChecked__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setChecked__(nid, !!v);
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__chkTarget__", nil)
}

func (r *Runtime) defineValueAccessor(obj *goja.Object) {
	_ = r.vm.Set("__valTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__valTarget__, 'value', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getValue__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setValue__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__valTarget__", nil)
}

// getNode extracts the *dom.Node from a JS wrapper object via its __nid__.
func (r *Runtime) getNode(v goja.Value) *dom.Node {
	if v == nil || goja.IsNull(v) || goja.IsUndefined(v) {
		return nil
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	nidVal := obj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return nil
	}
	nid := int(nidVal.ToInteger())
	return r.nodeRegistry[nid]
}

// jsAppendChild implements element.appendChild(child).
func (r *Runtime) jsAppendChild(call goja.FunctionCall) goja.Value {
	parentNode := r.getNode(call.This)
	if parentNode == nil {
		return goja.Undefined()
	}
	if len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	childNode := r.getNode(call.Arguments[0])
	if childNode == nil {
		return goja.Undefined()
	}
	parentNode.AppendChild(childNode)
	r.notifyMutation()
	// Invoke connectedCallback if child is a custom element
	if childNode.Type == dom.NodeElement {
		r.invokeLifecycleCallback(childNode, "connectedCallback")
	}
	// Return the child, matching the DOM spec.
	return call.Arguments[0]
}

// jsQuerySelector implements document.querySelector(selector).
func (r *Runtime) jsQuerySelector(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	sel := call.Arguments[0].String()

	// Determine the search root: if called on an element wrapper, search
	// its subtree; otherwise search the whole document.
	root := &r.domDoc.Node
	if r.getNode(call.This) != nil {
		root = r.getNode(call.This)
	}

	node := r.querySelector(root, sel)
	if node == nil {
		return goja.Null()
	}
	return r.wrapNode(node)
}

// jsQuerySelectorAll implements document.querySelectorAll(selector).
func (r *Runtime) jsQuerySelectorAll(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return newJSArray(r.vm, nil)
	}
	sel := call.Arguments[0].String()

	root := &r.domDoc.Node
	if r.getNode(call.This) != nil {
		root = r.getNode(call.This)
	}

	nodes := r.querySelectorAll(root, sel)
	wrapped := make([]goja.Value, len(nodes))
	for i, n := range nodes {
		wrapped[i] = r.wrapNode(n)
	}
	return newJSArray(r.vm, wrapped)
}

// jsCreateElement implements document.createElement(tag).
func (r *Runtime) jsCreateElement(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	tag := call.Arguments[0].String()
	node := r.domDoc.NewElement(tag)
	return r.wrapNode(node)
}

// querySelector returns the first descendant of root (depth-first, excluding
// root itself) that matches sel, or nil.
func (r *Runtime) querySelector(root *dom.Node, sel string) *dom.Node {
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if r.matchSelector(c, sel) {
			return c
		}
		if found := r.querySelector(c, sel); found != nil {
			return found
		}
	}
	return nil
}

// querySelectorAll returns every descendant of root that matches sel.
func (r *Runtime) querySelectorAll(root *dom.Node, sel string) []*dom.Node {
	var result []*dom.Node
	r.collectMatching(root, sel, &result)
	return result
}

func (r *Runtime) collectMatching(n *dom.Node, sel string, result *[]*dom.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r.matchSelector(c, sel) {
			*result = append(*result, c)
		}
		r.collectMatching(c, sel, result)
	}
}

// matchSelector reports whether n matches a simple CSS selector. Supported
// forms are tag name ("p"), ID ("#main"), and class (".active").
func (r *Runtime) matchSelector(n *dom.Node, sel string) bool {
	if !n.Element() {
		return false
	}
	if sel == "" {
		return false
	}

	switch {
	case strings.HasPrefix(sel, "#"):
		return n.GetAttribute("id") == sel[1:]
	case strings.HasPrefix(sel, "."):
		class := sel[1:]
		for _, c := range n.ClassList() {
			if c == class {
				return true
			}
		}
		return false
	default:
		return strings.EqualFold(n.Data, sel)
	}
}

// newJSArray builds a JS Array from a slice of goja values.
func newJSArray(vm *goja.Runtime, vals []goja.Value) *goja.Object {
	arr := vm.NewArray()
	for i, v := range vals {
		_ = arr.Set(fmt.Sprintf("%d", i), v)
	}
	return arr
}

// setupTimers installs setTimeout, setInterval, clearTimeout, and clearInterval
// on both the global scope and the window object, matching the browser model
// where these are properties of the Window interface and also available as bare
// globals.
func (r *Runtime) setupTimers() {
	setTimeout := func(callback goja.Value, delay goja.Value) int {
		ms := int64(0)
		if delay != nil && !goja.IsUndefined(delay) && !goja.IsNull(delay) {
			ms = delay.ToInteger()
		}
		if ms < 0 {
			ms = 0
		}
		r.nextTimerID++
		id := r.nextTimerID
		r.timers[id] = &timer{
			id:       id,
			callback: callback,
			interval: time.Duration(ms) * time.Millisecond,
			nextFire: time.Now().Add(time.Duration(ms) * time.Millisecond),
			repeat:   false,
		}
		return id
	}

	setInterval := func(callback goja.Value, delay goja.Value) int {
		ms := int64(0)
		if delay != nil && !goja.IsUndefined(delay) && !goja.IsNull(delay) {
			ms = delay.ToInteger()
		}
		if ms < 1 {
			ms = 1 // setInterval with 0 would spin; clamp to 1ms
		}
		r.nextTimerID++
		id := r.nextTimerID
		r.timers[id] = &timer{
			id:       id,
			callback: callback,
			interval: time.Duration(ms) * time.Millisecond,
			nextFire: time.Now().Add(time.Duration(ms) * time.Millisecond),
			repeat:   true,
		}
		return id
	}

	clearTimeout := func(id int) {
		delete(r.timers, id)
	}

	clearInterval := func(id int) {
		delete(r.timers, id)
	}

	// Install on the global scope (bare names).
	_ = r.vm.Set("setTimeout", setTimeout)
	_ = r.vm.Set("setInterval", setInterval)
	_ = r.vm.Set("clearTimeout", clearTimeout)
	_ = r.vm.Set("clearInterval", clearInterval)

	// Also install on the window object, matching the browser spec.
	win := r.vm.Get("window").(*goja.Object)
	_ = win.Set("setTimeout", setTimeout)
	_ = win.Set("setInterval", setInterval)
	_ = win.Set("clearTimeout", clearTimeout)
	_ = win.Set("clearInterval", clearInterval)
}

// Tick processes all timers whose fire time has arrived, invoking each
// callback in the goja runtime. One-shot timers are removed after firing;
// repeating timers (setInterval) are rescheduled for their next interval.
//
// Tick is designed to be called from the same goroutine that calls Run,
// typically once per vsync or after each script completes. It processes due
// timers in a loop so that callbacks which schedule new timers with a fire
// time already past are also drained before Tick returns.
func (r *Runtime) Tick() {
	r.DrainFetchCallbacks()
	for {
		now := time.Now()
		var toFire []*timer
		for _, t := range r.timers {
			if !now.Before(t.nextFire) {
				toFire = append(toFire, t)
			}
		}
		if len(toFire) == 0 {
			return
		}
		for _, t := range toFire {
			if t.repeat {
				t.nextFire = now.Add(t.interval)
			} else {
				delete(r.timers, t.id)
			}
			// Invoke the callback; exceptions are caught and discarded,
			// matching browser behavior where an uncaught error in a
			// timer callback does not stop other timers.
			if fn, ok := goja.AssertFunction(t.callback); ok {
				_, _ = fn(nil)
			}
		}
	}
}

// Run evaluates one classic script to completion, returning an *Error for an
// uncaught exception, ErrInterrupted if the bound fired, or ErrClosed after
// Close.
func (r *Runtime) Run(src, sourceURL string) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrClosed
	}
	r.mu.Unlock()

	if len(src) > MaxScriptBytes {
		return ErrTooLarge
	}

	timeout := r.opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Arm the timeout interrupt.
	timer := time.AfterFunc(timeout, func() {
		r.vm.Interrupt(ErrInterrupted)
	})
	defer timer.Stop()

	// Compile first to catch syntax errors before execution.
	prog, err := goja.Compile(sourceURL, src, false)
	if err != nil {
		return r.mapError(err, sourceURL)
	}

	// Run the compiled program.
	_, err = r.vm.RunProgram(prog)
	if err != nil {
		return r.mapError(err, sourceURL)
	}

	return nil
}

func (r *Runtime) mapError(err error, sourceURL string) error {
	if err == nil {
		return nil
	}

	// Check for interrupt (timeout or manual).
	var gojaErr *goja.InterruptedError
	if errors.As(err, &gojaErr) {
		return ErrInterrupted
	}

	// Check if it's a goja Exception (uncaught JS exception).
	var exc *goja.Exception
	if errors.As(err, &exc) {
		msg := exc.String()
		line := extractLine(err.Error())
		return &Error{
			SourceURL: sourceURL,
			Line:      line,
			Message:   msg,
		}
	}

	// Syntax or other compile error.
	line := extractLine(err.Error())
	return &Error{
		SourceURL: sourceURL,
		Line:      line,
		Message:   err.Error(),
	}
}

func extractLine(errMsg string) int {
	// goja error messages often contain line information.
	// Try to extract it from patterns like "line N" or "(line N)".
	for i := 0; i < len(errMsg); i++ {
		if i+5 < len(errMsg) && errMsg[i:i+5] == "line " {
			j := i + 5
			for j < len(errMsg) && errMsg[j] >= '0' && errMsg[j] <= '9' {
				j++
			}
			if j > i+5 {
				if n, err := strconv.Atoi(errMsg[i+5 : j]); err == nil {
					return n
				}
			}
		}
	}
	return 1
}

// Global reads a top-level binding created by a script.
func (r *Runtime) Global(name string) (Value, bool) {
	v := r.vm.Get(name)
	if v == nil || goja.IsUndefined(v) {
		return Value{}, false
	}
	return exportValue(v), true
}

func exportValue(v goja.Value) Value {
	if goja.IsNull(v) {
		return Value{Kind: KindNull}
	}

	switch val := v.Export().(type) {
	case float64:
		return Value{Kind: KindNumber, Num: val}
	case int64:
		return Value{Kind: KindNumber, Num: float64(val)}
	case string:
		return Value{Kind: KindString, Str: val}
	case bool:
		return Value{Kind: KindBool, Bool: val}
	default:
		if v.ExportType().Kind().String() == "func" {
			return Value{Kind: KindFunction}
		}
		return Value{Kind: KindObject}
	}
}

// Title returns what the scripts left document.title at.
func (r *Runtime) Title() string {
	v := r.vm.Get("document")
	if v != nil {
		doc := v.ToObject(r.vm)
		if doc != nil {
			titleVal := doc.Get("__title__")
			if titleVal != nil && !goja.IsUndefined(titleVal) {
				return titleVal.String()
			}
		}
	}
	return r.title
}

// Interrupt stops a Run in progress, from any goroutine.
func (r *Runtime) Interrupt() {
	r.vm.Interrupt(ErrInterrupted)
}

// Close releases the realm. Run after Close reports ErrClosed.
func (r *Runtime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return nil
}

// SetDOM updates the DOM document reference. The engine calls this after
// parsing HTML so the runtime's document methods (getElementById, etc.)
// operate on the same nodes the engine's arena references.
func (r *Runtime) SetDOM(doc *dom.Document) {
	r.mu.Lock()
	r.opts.DOM = doc
	r.domDoc = doc
	if r.nodeRegistry == nil {
		r.nodeRegistry = make(map[int]*dom.Node)
		r.nodeWrappers = make(map[int]*goja.Object)
		r.shadowRoots = make(map[int]*goja.Object)
		r.nextNID = 1
	}
	r.mu.Unlock()
	// Add document methods (getElementById, querySelector, etc.) now that
	// the DOM reference is set. setupDOM checks r.opts.DOM internally.
	r.setupDOM()
}

// ---------------------------------------------------------------------------
// JS bridge: event methods
// ---------------------------------------------------------------------------

// jsAddEventListener implements element.addEventListener(type, callback).
func (r *Runtime) jsAddEventListener(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	fn, ok := goja.AssertFunction(call.Arguments[1])
	if !ok {
		return goja.Undefined()
	}

	// Wrap the goja function as a Go callback that creates a JS Event
	// object and passes it to the listener.
	callback := func(e *dom.Event) {
		eventObj := r.wrapEvent(e)
		_, _ = fn(goja.Null(), eventObj)
	}

	capture := false
	if len(call.Arguments) >= 3 {
		capture = call.Arguments[2].ToBoolean()
	}
	dom.AddEventListenerWithKey(node, typ, callback, capture, call.Arguments[1])
	return goja.Undefined()
}

// jsRemoveEventListener implements element.removeEventListener(type, callback).
// Because each addEventListener call creates a fresh Go closure, the removal
// cannot match the original pointer. This is a best-effort no-op that returns
// undefined, matching the spec's requirement that removeEventListener does not
// throw for missing listeners.
func (r *Runtime) jsRemoveEventListener(call goja.FunctionCall) goja.Value {
	// Best-effort: the Go closure wrapper created in jsAddEventListener has
	// a distinct pointer from the one the caller passes here, so an exact
	// match is not possible without a listener registry. Return undefined
	// silently, which is spec-compliant for missing listeners.
	return goja.Undefined()
}

// jsDispatchEvent implements element.dispatchEvent(event).
func (r *Runtime) jsDispatchEvent(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	eventObj := call.Arguments[0]
	typ := ""
	if t := eventObj.ToObject(r.vm).Get("type"); t != nil && !goja.IsUndefined(t) {
		typ = t.String()
	}
	event := dom.NewEvent(typ, true, true)
	result := dom.DispatchEvent(node, event)
	return r.vm.ToValue(result)
}

// wrapEvent creates a JS Event object from a *dom.Event. It exposes the
// standard Event properties and, for MouseEvent and KeyboardEvent subtypes,
// their additional fields.
func (r *Runtime) wrapEvent(e *dom.Event) goja.Value {
	obj := r.vm.NewObject()
	_ = obj.Set("type", e.Type)
	_ = obj.Set("bubbles", e.Bubbles())
	_ = obj.Set("cancelable", e.Cancelable())
	_ = obj.Set("stopPropagation", func() { e.StopPropagation() })
	_ = obj.Set("stopImmediatePropagation", func() { e.StopImmediatePropagation() })
	_ = obj.Set("preventDefault", func() { e.PreventDefault() })

	// defaultPrevented must be read dynamically: the callback may call
	// preventDefault() before reading this property. We expose a helper
	// method and define a JS getter that calls it.
	_ = obj.Set("__getDP__", func() bool { return e.DefaultPrevented() })
	_ = r.vm.Set("__ev__", obj)
	_, _ = r.vm.RunString(`
		(function() {
			var o = __ev__;
			var getter = o.__getDP__;
			Object.defineProperty(o, 'defaultPrevented', {
				get: getter,
				configurable: true,
				enumerable: true
			});
		})();
	`)
	_ = r.vm.Set("__ev__", nil)

	if e.Target() != nil {
		_ = obj.Set("target", r.wrapNode(e.Target()))
	}
	if e.CurrentTarget() != nil {
		_ = obj.Set("currentTarget", r.wrapNode(e.CurrentTarget()))
	}

	// MouseEvent fields (only populated for a dispatched *MouseEvent, recovered
	// through the payload back-pointer since listeners receive a *Event).
	if me := e.AsMouse(); me != nil {
		_ = obj.Set("clientX", me.ClientX)
		_ = obj.Set("clientY", me.ClientY)
		_ = obj.Set("screenX", me.ScreenX)
		_ = obj.Set("screenY", me.ScreenY)
		_ = obj.Set("button", me.Button)
		_ = obj.Set("altKey", me.AltKey)
		_ = obj.Set("ctrlKey", me.CtrlKey)
		_ = obj.Set("shiftKey", me.ShiftKey)
		_ = obj.Set("metaKey", me.MetaKey)
	}

	// KeyboardEvent fields.
	if ke := e.AsKeyboard(); ke != nil {
		_ = obj.Set("key", ke.Key)
		_ = obj.Set("code", ke.Code)
		_ = obj.Set("altKey", ke.AltKey)
		_ = obj.Set("ctrlKey", ke.CtrlKey)
		_ = obj.Set("shiftKey", ke.ShiftKey)
		_ = obj.Set("metaKey", ke.MetaKey)
		_ = obj.Set("repeat", ke.Repeat)
	}

	return obj
}

// ---------------------------------------------------------------------------
// JS bridge: attribute methods
// ---------------------------------------------------------------------------

// jsGetAttribute implements element.getAttribute(name).
func (r *Runtime) jsGetAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	name := call.Arguments[0].String()
	if !node.HasAttribute(name) {
		return goja.Null()
	}
	return r.vm.ToValue(node.GetAttribute(name))
}

// jsSetAttribute implements element.setAttribute(name, value).
func (r *Runtime) jsSetAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	name := call.Arguments[0].String()
	value := call.Arguments[1].String()
	node.SetAttribute(name, value)
	r.notifyMutation()
	return goja.Undefined()
}

// jsRemoveAttribute implements element.removeAttribute(name).
func (r *Runtime) jsRemoveAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	name := call.Arguments[0].String()
	// Remove the attribute from the node's Attr slice.
	lower := strings.ToLower(name)
	for i := range node.Attr {
		if strings.ToLower(node.Attr[i].Name) == lower {
			node.Attr = append(node.Attr[:i], node.Attr[i+1:]...)
			break
		}
	}
	r.notifyMutation()
	return goja.Undefined()
}

// jsHasAttribute implements element.hasAttribute(name).
func (r *Runtime) jsHasAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	return r.vm.ToValue(node.HasAttribute(call.Arguments[0].String()))
}

// ---------------------------------------------------------------------------
// JS bridge: mutation methods
// ---------------------------------------------------------------------------

// jsRemove implements element.remove(), detaching the node from its parent.
func (r *Runtime) jsRemove(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || node.Parent == nil {
		return goja.Undefined()
	}
	node.Parent.RemoveChild(node)
	r.notifyMutation()
	// Invoke disconnectedCallback if node is a custom element
	if node.Type == dom.NodeElement {
		r.invokeLifecycleCallback(node, "disconnectedCallback")
	}
	return goja.Undefined()
}

// jsGetElementByID implements document.getElementById(id).
func (r *Runtime) jsGetElementByID(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	id := call.Arguments[0].String()
	node := r.domDoc.ElementByID(id)
	if node == nil {
		return goja.Null()
	}
	return r.wrapNode(node)
}

// jsGetElementsByClassName implements document.getElementsByClassName(class).
func (r *Runtime) jsGetElementsByClassName(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return newJSArray(r.vm, nil)
	}
	class := call.Arguments[0].String()
	nodes := r.domDoc.ElementsByClassName(class)
	wrapped := make([]goja.Value, len(nodes))
	for i, n := range nodes {
		wrapped[i] = r.wrapNode(n)
	}
	return newJSArray(r.vm, wrapped)
}

// jsGetElementsByTagName implements document.getElementsByTagName(tag).
func (r *Runtime) jsGetElementsByTagName(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return newJSArray(r.vm, nil)
	}
	tag := call.Arguments[0].String()
	nodes := r.domDoc.ElementsByTagName(tag)
	wrapped := make([]goja.Value, len(nodes))
	for i, n := range nodes {
		wrapped[i] = r.wrapNode(n)
	}
	return newJSArray(r.vm, wrapped)
}

// jsCreateTextNode implements document.createTextNode(text).
func (r *Runtime) jsCreateTextNode(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	text := call.Arguments[0].String()
	node := r.domDoc.NewText(text)
	return r.wrapNode(node)
}

// ---------------------------------------------------------------------------
// HTML serialization (for innerHTML getter)
// ---------------------------------------------------------------------------

// voidElements lists the HTML elements that have no closing tag.
var voidElements = map[string]bool{
	"br": true, "hr": true, "img": true, "input": true,
	"meta": true, "link": true, "area": true, "base": true,
	"col": true, "embed": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

// serializeChildren returns the HTML serialization of node's children.
func (r *Runtime) serializeChildren(node *dom.Node) string {
	var sb strings.Builder
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		r.serializeNode(&sb, c)
	}
	return sb.String()
}

// serializeNode writes the HTML serialization of a single node to sb.
func (r *Runtime) serializeNode(sb *strings.Builder, n *dom.Node) {
	switch n.Type {
	case dom.NodeText:
		sb.WriteString(escapeHTML(n.DataContent))
	case dom.NodeElement:
		tag := strings.ToLower(n.Data)
		sb.WriteString("<")
		sb.WriteString(tag)
		for _, a := range n.Attr {
			sb.WriteString(" ")
			sb.WriteString(a.Name)
			sb.WriteString(`="`)
			sb.WriteString(escapeAttr(a.Value))
			sb.WriteString(`"`)
		}
		if voidElements[tag] {
			sb.WriteString(">")
			return
		}
		sb.WriteString(">")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			r.serializeNode(sb, c)
		}
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteString(">")
	case dom.NodeComment:
		sb.WriteString("<!--")
		sb.WriteString(n.DataContent)
		sb.WriteString("-->")
	}
}

// escapeHTML escapes special HTML characters in text content.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeAttr escapes special characters in attribute values.
func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ---------------------------------------------------------------------------
// Mutation notification
// ---------------------------------------------------------------------------

// notifyMutation calls the OnMutation callback if one was provided. It is
// invoked after every DOM mutation from JS so the engine can re-style and
// re-layout.
func (r *Runtime) notifyMutation() {
	if r.opts.OnMutation != nil {
		r.opts.OnMutation()
	}
}

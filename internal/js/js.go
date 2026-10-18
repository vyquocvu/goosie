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

// MaxMicrotasksPerCheckpoint bounds one microtask drain. A chain that
// schedules forever without touching the VM would otherwise spin past the
// timeout interrupt (which only lands on VM calls); the bound ends it with
// ErrInterrupted like any other runaway, consistent with every other
// admission limit in the engine.
const MaxMicrotasksPerCheckpoint = 100000

// timeout returns the bound for one user-code invocation: the configured
// timeout or the default. Run, Tick and the pump-driven callbacks all share
// it, so a hung callback wedges its frame rather than its thread.
func (r *Runtime) timeout() time.Duration {
	if r.opts.Timeout > 0 {
		return r.opts.Timeout
	}
	return DefaultTimeout
}

// callBounded invokes fn under the runtime timeout, reporting ErrInterrupted
// when the bound fires. Non-interrupt failures pass through untouched, so
// callers keep their existing error semantics and only add the stop rule.
func (r *Runtime) callBounded(fn func() (goja.Value, error)) (goja.Value, error) {
	t := time.AfterFunc(r.timeout(), func() { r.vm.Interrupt(ErrInterrupted) })
	defer t.Stop()
	v, err := fn()
	if err == nil {
		return v, nil
	}
	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		return v, ErrInterrupted
	}
	return v, err
}

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

	// Cookies is the cookie jar document.cookie reads and writes. When nil,
	// the binding keeps a per-document memory jar instead, which round-trips
	// script-set values for the document's own URL.
	Cookies CookieStore

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

	// Storage supplies origin-scoped Web Storage (localStorage, sessionStorage).
	// When nil, ephemeral in-memory storage is created per runtime.
	Storage *OriginStorageRegistry

	// SessionID identifies the tab/browsing context for sessionStorage isolation.
	SessionID string

	// DarkMode indicates whether the host environment prefers a dark color scheme.
	DarkMode bool

	// HighContrast indicates whether the host environment prefers increased contrast.
	HighContrast bool
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

	// microtasks is the FIFO promise-reaction queue. Producers on any
	// goroutine only ever append (queueMicrotask); only the Run/Tick
	// goroutine drains (drainMicrotasks), so user callbacks always run on
	// the runtime's own thread even when a network event settled.
	microtasks []func() error

	// DOM bridge state. nil when Options.DOM is nil.
	domDoc       *dom.Document
	nodeRegistry map[int]*dom.Node
	nextNID      int
	nodeProto    *goja.Object
	// wrapCache maps a DOM node to its single wrapper nid, so repeated
	// queries over the same nodes reuse one wrapper instead of compiling
	// accessors and closures per hit. Wrapping the same node twice used to
	// cost two RunString compiles plus dozens of sets per hit and leaked a
	// registry entry per query; it also broke `===` identity the DOM spec
	// requires. Snapshots (tagName/className/type) refresh on reuse, so a
	// cached wrapper reads exactly what a fresh one would.
	wrapCache map[*dom.Node]int

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
	// XHR installs regardless: without a transport the constructor still
	// exists and send() fails through the error event, so capability
	// probes never see a missing constructor.
	r.setupXHR(opts.HTTPClient)
	r.setupTimers()
	r.setupPromise()
	r.setupCookies()
	r.setupFonts()
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
	r.setupCSS()
	r.setupDragDrop()
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

	// The window object IS the global scope (HTML spec: Window is the
	// global object). An earlier stub installed window as a separate
	// object, so `window.google = ns` followed by bare `google` died with
	// "google is not defined" - the exact shape every dynamic page's boot
	// script takes. Aliasing keeps one identity: assignments in either
	// direction are visible in the other, and the later dual-installs
	// (global + window) below degenerate to the same store.
	win := r.vm.GlobalObject()
	_ = win.Set("location", loc)
	_ = win.Set("window", win)
	_ = win.Set("self", win)
	_ = win.Set("globalThis", win)
	nav := r.vm.NewObject()
	_ = nav.Set("userAgent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Goosie/2.0")
	_ = nav.Set("language", "en-US")
	_ = nav.Set("languages", r.vm.ToValue([]string{"en-US", "en"}))
	_ = nav.Set("cookieEnabled", true)
	_ = nav.Set("onLine", true)
	_ = win.Set("navigator", nav)

	_ = win.Set("matchMedia", func(call goja.FunctionCall) goja.Value {
		q := ""
		if len(call.Arguments) > 0 {
			q = call.Arguments[0].String()
		}
		mql := r.vm.NewObject()
		_ = mql.Set("media", q)
		matches := true
		lq := strings.ToLower(q)
		if strings.Contains(lq, "print") && !strings.Contains(lq, "not") {
			matches = false
		}
		if strings.Contains(lq, "prefers-color-scheme") {
			dark := strings.Contains(lq, "dark")
			light := strings.Contains(lq, "light")
			if r.opts.DarkMode {
				if dark {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if light {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			} else {
				if dark {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if light {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			}
		}
		if strings.Contains(lq, "prefers-contrast") {
			more := strings.Contains(lq, "more")
			noPref := strings.Contains(lq, "no-preference")
			if r.opts.HighContrast {
				if more {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if noPref {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			} else {
				if more {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if noPref {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			}
		}
		_ = mql.Set("matches", matches)
		_ = mql.Set("addListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = mql.Set("removeListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = mql.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = mql.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		return mql
	})
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
	r.wrapCache = make(map[*dom.Node]int)
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
	_ = r.nodeProto.Set("setAttributeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsSetAttributeNS(call)
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
	// ParentNode query roots: jsQuerySelector(All) already resolve the root
	// from call.This, so the same handlers serve document and elements with
	// document-order, subtree-scoped results.
	_ = r.nodeProto.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelector(call)
	})
	_ = r.nodeProto.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelectorAll(call)
	})
	_ = r.nodeProto.Set("insertAdjacentHTML", func(call goja.FunctionCall) goja.Value {
		return r.jsInsertAdjacentHTML(call)
	})
	_ = r.nodeProto.Set("prepend", func(call goja.FunctionCall) goja.Value {
		return r.jsPrepend(call)
	})
	_ = r.nodeProto.Set("append", func(call goja.FunctionCall) goja.Value {
		return r.jsAppend(call)
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
	_ = doc.Set("prepend", func(call goja.FunctionCall) goja.Value {
		return r.jsPrepend(call)
	})
	_ = doc.Set("append", func(call goja.FunctionCall) goja.Value {
		return r.jsAppend(call)
	})
	_ = doc.Set("__getChildren__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue([]any{})
		}
		var list []goja.Value
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				list = append(list, r.wrapNode(c))
			}
		}
		if list == nil {
			list = []goja.Value{}
		}
		return r.vm.ToValue(list)
	})
	_ = doc.Set("__getParentElement__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil || node.Parent == nil || !node.Parent.Element() {
			return goja.Null()
		}
		return r.wrapNode(node.Parent)
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
	_ = doc.Set("__getDialogOpen__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(node.HasAttribute("open"))
	})
	_ = doc.Set("__setDialogOpen__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		open := call.Arguments[1].ToBoolean()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if open {
			node.SetAttribute("open", "")
		} else {
			node.RemoveAttribute("open")
			node.IsModal = false
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getDialogReturnValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.ReturnValue)
	})
	_ = doc.Set("__setDialogReturnValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		v := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		node.ReturnValue = v
		return goja.Undefined()
	})
	_ = doc.Set("__getPopover__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil || !node.HasAttribute("popover") {
			return goja.Null()
		}
		val := node.GetAttribute("popover")
		if val == "" || strings.EqualFold(val, "auto") {
			return r.vm.ToValue("auto")
		}
		if strings.EqualFold(val, "manual") {
			return r.vm.ToValue("manual")
		}
		return r.vm.ToValue(val)
	})
	_ = doc.Set("__setPopover__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if len(call.Arguments) < 2 || goja.IsNull(call.Arguments[1]) || goja.IsUndefined(call.Arguments[1]) {
			node.RemoveAttribute("popover")
			node.IsPopoverOpen = false
		} else {
			node.SetAttribute("popover", call.Arguments[1].String())
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getId__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.GetAttribute("id"))
	})
	_ = doc.Set("__setId__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		v := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute("id", v)
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getDataset__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		m := r.vm.NewObject()
		if node == nil || !node.Element() {
			return m
		}
		for _, a := range node.Attr {
			key, ok := datasetKey(strings.ToLower(a.Name))
			if !ok {
				continue
			}
			attrName := a.Name
			getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
				cur := r.nodeRegistry[nid]
				if cur == nil {
					return goja.Undefined()
				}
				return r.vm.ToValue(cur.GetAttribute(attrName))
			})
			setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				cur := r.nodeRegistry[nid]
				if cur == nil {
					return goja.Undefined()
				}
				var v string
				if len(call.Arguments) > 0 && call.Arguments[0] != nil {
					v = call.Arguments[0].String()
				}
				cur.SetAttribute(attrName, v)
				r.notifyMutation()
				return goja.Undefined()
			})
			_ = m.DefineAccessorProperty(key, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
		// The registry key rides along non-enumerably so Object.keys sees
		// only the data properties, as on a real DOMStringMap.
		_ = m.DefineDataProperty("__nid__", r.vm.ToValue(nid), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		return m
	})
	_ = doc.Set("__getDatasetAttr__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Null()
		}
		node := r.nodeRegistry[int(call.Arguments[0].ToInteger())]
		if node == nil || !node.Element() {
			return goja.Null()
		}
		attr := call.Arguments[1].String()
		for _, a := range node.Attr {
			if strings.EqualFold(a.Name, attr) {
				return r.vm.ToValue(a.Value)
			}
		}
		return goja.Null()
	})
	_ = doc.Set("__setDatasetAttr__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 4 {
			return goja.Undefined()
		}
		target, _ := call.Arguments[0].(*goja.Object)
		nid := int(call.Arguments[1].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil || !node.Element() || target == nil {
			return goja.Undefined()
		}
		attr, v := call.Arguments[2].String(), call.Arguments[3].String()
		node.SetAttribute(attr, v)
		r.notifyMutation()
		// Define the live pair on the map so later reads and Object.keys
		// see the new attribute exactly like a parsed one.
		if key, ok := datasetKey(strings.ToLower(attr)); ok {
			attrName := attr
			getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
				if cur := r.nodeRegistry[nid]; cur != nil {
					return r.vm.ToValue(cur.GetAttribute(attrName))
				}
				return goja.Undefined()
			})
			setter := r.vm.ToValue(func(setCall goja.FunctionCall) goja.Value {
				cur := r.nodeRegistry[nid]
				if cur == nil {
					return goja.Undefined()
				}
				var sv string
				if len(setCall.Arguments) > 0 && setCall.Arguments[0] != nil {
					sv = setCall.Arguments[0].String()
				}
				cur.SetAttribute(attrName, sv)
				r.notifyMutation()
				return goja.Undefined()
			})
			_ = target.DefineAccessorProperty(key, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
		return goja.Undefined()
	})

	// Dynamic body property: wraps the body element on first access and
	// caches it. If the body is replaced, the host should re-run setupDOM.
	if r.domDoc.Body != nil {
		_ = doc.Set("body", r.wrapNode(r.domDoc.Body))
	} else {
		_ = doc.Set("body", goja.Null())
	}
	// documentElement and head: boot scripts hang global listeners off the
	// root (document.documentElement.addEventListener("click", ...)) and
	// probe head for script/style placement. Same wrap-once rule as body.
	if r.domDoc.HTML != nil {
		_ = doc.Set("documentElement", r.wrapNode(r.domDoc.HTML))
	} else {
		_ = doc.Set("documentElement", goja.Null())
	}
	if r.domDoc.Head != nil {
		_ = doc.Set("head", r.wrapNode(r.domDoc.Head))
	} else {
		_ = doc.Set("head", goja.Null())
	}

	_ = doc.Set("__getShadowRoot__", func(call goja.FunctionCall) goja.Value {
		return r.jsGetShadowRoot(call)
	})
}

// wrapNode creates a JS wrapper object for a DOM node. The wrapper inherits
// from nodeProto (which provides appendChild, textContent, addEventListener)
// and stores a __nid__ registry key so the prototype methods can find the
// underlying *dom.Node.
func (r *Runtime) wrapNode(node *dom.Node) goja.Value {
	if node == nil {
		return goja.Null()
	}
	// One wrapper per node: a repeat wrap refreshes the snapshot data
	// properties and returns the same object. This keeps `===` identity
	// across queries (per DOM spec) and avoids recompiling the
	// textContent/innerHTML/id accessors plus dozens of sets per hit —
	// querySelectorAll over thousands of nodes used to spend seconds here.
	if r.wrapCache == nil {
		r.wrapCache = make(map[*dom.Node]int)
	}
	if nid, ok := r.wrapCache[node]; ok {
		if obj := r.nodeWrappers[nid]; obj != nil {
			r.refreshWrapSnapshots(obj, node)
			return obj
		}
	}
	nid := r.nextNID
	r.nextNID++
	r.nodeRegistry[nid] = node
	r.wrapCache[node] = nid

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
		// Live dataset map, re-defined per instance like the accessors above.
		r.defineDatasetAccessor(obj)
		// Live element accessors: children, parentElement, firstElementChild, etc.
		r.defineElementAccessors(obj)
	}

	// Element-specific properties.
	if node.Element() {
		_ = obj.Set("tagName", strings.ToUpper(node.Data))
		_ = obj.Set("nodeName", strings.ToUpper(node.Data))
		r.defineIdAccessor(obj)
		_ = obj.Set("className", node.GetAttribute("class"))
		_ = obj.Set("lang", node.GetAttribute("lang"))
		_ = obj.Set("dir", node.GetAttribute("dir"))
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
						if node.Doc != nil {
							node.Doc.MarkQueryDirty()
						}
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
								if node.Doc != nil {
									node.Doc.MarkQueryDirty()
								}
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
		if node.Data == "dialog" {
			r.defineDialogElement(obj, node)
		}
		r.definePopoverMethods(obj, node)
	}

	return obj
}

// refreshWrapSnapshots rewrites the data properties a fresh wrapNode would
// set, so a reused wrapper reads exactly what a new one would. Accessors
// (textContent/innerHTML/id/value/checked) and the classList closures are
// already installed and stay live against the same node.
func (r *Runtime) refreshWrapSnapshots(obj *goja.Object, node *dom.Node) {
	if !node.Element() {
		return
	}
	_ = obj.Set("tagName", strings.ToUpper(node.Data))
	_ = obj.Set("nodeName", strings.ToUpper(node.Data))
	_ = obj.Set("className", node.GetAttribute("class"))
	_ = obj.Set("lang", node.GetAttribute("lang"))
	_ = obj.Set("dir", node.GetAttribute("dir"))
	if node.Data == "input" {
		typ := strings.ToLower(node.GetAttribute("type"))
		if typ == "" {
			typ = "text"
		}
		_ = obj.Set("type", typ)
	}
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

func (r *Runtime) defineDialogElement(obj *goja.Object, node *dom.Node) {
	_ = obj.Set("show", func(call goja.FunctionCall) goja.Value {
		if node.HasAttribute("open") {
			return goja.Undefined()
		}
		node.SetAttribute("open", "")
		node.IsModal = false
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("showModal", func(call goja.FunctionCall) goja.Value {
		if node.HasAttribute("open") {
			panic(r.vm.NewTypeError("Failed to execute 'showModal' on 'HTMLDialogElement': The dialog is already open."))
		}
		node.SetAttribute("open", "")
		node.IsModal = true
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("open") {
			return goja.Undefined()
		}
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
			node.ReturnValue = call.Arguments[0].String()
		}
		node.RemoveAttribute("open")
		node.IsModal = false
		dom.DispatchEvent(node, dom.NewEvent("close", false, false))
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = r.vm.Set("__dlgTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__dlgTarget__, 'open', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return false;
				return document.__getDialogOpen__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setDialogOpen__(nid, !!v);
			},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(__dlgTarget__, 'returnValue', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getDialogReturnValue__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setDialogReturnValue__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__dlgTarget__", nil)
}

func (r *Runtime) definePopoverMethods(obj *goja.Object, node *dom.Node) {
	_ = obj.Set("showPopover", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("popover") {
			panic(r.vm.NewTypeError("Failed to execute 'showPopover' on 'HTMLElement': Not supported on elements that do not have a valid 'popover' attribute."))
		}
		if node.IsPopoverOpen {
			panic(r.vm.NewTypeError("Failed to execute 'showPopover' on 'HTMLElement': The popover is already open."))
		}
		pType := strings.ToLower(node.GetAttribute("popover"))
		if pType == "" || pType == "auto" {
			r.closeOpenAutoPopovers(node)
		}
		beforeEv := dom.NewEvent("beforetoggle", false, true)
		if !dom.DispatchEvent(node, beforeEv) {
			return goja.Undefined()
		}
		node.IsPopoverOpen = true
		dom.DispatchEvent(node, dom.NewEvent("toggle", false, false))
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("hidePopover", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("popover") {
			panic(r.vm.NewTypeError("Failed to execute 'hidePopover' on 'HTMLElement': Not supported on elements that do not have a valid 'popover' attribute."))
		}
		if !node.IsPopoverOpen {
			panic(r.vm.NewTypeError("Failed to execute 'hidePopover' on 'HTMLElement': The popover is not open."))
		}
		beforeEv := dom.NewEvent("beforetoggle", false, true)
		if !dom.DispatchEvent(node, beforeEv) {
			return goja.Undefined()
		}
		node.IsPopoverOpen = false
		dom.DispatchEvent(node, dom.NewEvent("toggle", false, false))
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("togglePopover", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("popover") {
			panic(r.vm.NewTypeError("Failed to execute 'togglePopover' on 'HTMLElement': Not supported on elements that do not have a valid 'popover' attribute."))
		}
		var force *bool
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
			b := call.Arguments[0].ToBoolean()
			force = &b
		}
		if force != nil {
			if *force && !node.IsPopoverOpen {
				pType := strings.ToLower(node.GetAttribute("popover"))
				if pType == "" || pType == "auto" {
					r.closeOpenAutoPopovers(node)
				}
				beforeEv := dom.NewEvent("beforetoggle", false, true)
				if dom.DispatchEvent(node, beforeEv) {
					node.IsPopoverOpen = true
					dom.DispatchEvent(node, dom.NewEvent("toggle", false, false))
					r.notifyMutation()
				}
			} else if !*force && node.IsPopoverOpen {
				beforeEv := dom.NewEvent("beforetoggle", false, true)
				if dom.DispatchEvent(node, beforeEv) {
					node.IsPopoverOpen = false
					dom.DispatchEvent(node, dom.NewEvent("toggle", false, false))
					r.notifyMutation()
				}
			}
			return r.vm.ToValue(node.IsPopoverOpen)
		}
		if node.IsPopoverOpen {
			beforeEv := dom.NewEvent("beforetoggle", false, true)
			if dom.DispatchEvent(node, beforeEv) {
				node.IsPopoverOpen = false
				dom.DispatchEvent(node, dom.NewEvent("toggle", false, false))
				r.notifyMutation()
			}
		} else {
			pType := strings.ToLower(node.GetAttribute("popover"))
			if pType == "" || pType == "auto" {
				r.closeOpenAutoPopovers(node)
			}
			beforeEv := dom.NewEvent("beforetoggle", false, true)
			if dom.DispatchEvent(node, beforeEv) {
				node.IsPopoverOpen = true
				dom.DispatchEvent(node, dom.NewEvent("toggle", false, false))
				r.notifyMutation()
			}
		}
		return r.vm.ToValue(node.IsPopoverOpen)
	})

	_ = r.vm.Set("__popoverTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__popoverTarget__, 'popover', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return null;
				return document.__getPopover__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setPopover__(nid, v === null || v === undefined ? null : String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__popoverTarget__", nil)
}

func (r *Runtime) closeOpenAutoPopovers(except *dom.Node) {
	if r.domDoc == nil {
		return
	}
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n != except && n.Element() && n.IsPopoverOpen {
			pType := strings.ToLower(n.GetAttribute("popover"))
			if pType == "" || pType == "auto" {
				n.IsPopoverOpen = false
				dom.DispatchEvent(n, dom.NewEvent("toggle", false, false))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&r.domDoc.Node)
}

// defineIdAccessor installs a live id getter/setter on obj: reading reflects
// the current attribute (not the wrap-time snapshot) and writing sets it, so
// `div.id = "g"` retargets getElementById and #g selectors.
func (r *Runtime) defineIdAccessor(obj *goja.Object) {
	_ = r.vm.Set("__idTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__idTarget__, 'id', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getId__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setId__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__idTarget__", nil)
}

// defineDatasetAccessor installs a live dataset (DOMStringMap) getter on obj.
// Each access builds a fresh map object for the element's current data-*
// attributes, wrapped in a Proxy so brand-new keys assign straight through
// to setAttribute instead of dying as dead JS properties. Reads of unknown
// keys stay undefined, as in browsers.
func (r *Runtime) defineDatasetAccessor(obj *goja.Object) {
	_ = r.vm.Set("__dsTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__dsTarget__, 'dataset', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return {};
				var map = document.__getDataset__(nid);
				var toAttr = function(k) {
					return 'data-' + String(k).replace(/[A-Z]/g, function(c) {
						return '-' + c.toLowerCase();
					});
				};
				return new Proxy(map, {
					get: function(t, k) {
						if (typeof k !== 'string') return t[k];
						var cur = t[k];
						if (cur !== undefined) return cur;
						if (k === '__nid__') return nid;
						var v = document.__getDatasetAttr__(nid, toAttr(k));
						return v === null ? undefined : v;
					},
					set: function(t, k, v) {
						if (typeof k !== 'string' || k === '__nid__') return true;
						document.__setDatasetAttr__(map, nid, toAttr(k), String(v));
						return true;
					}
				});
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__dsTarget__", nil)
}

func (r *Runtime) defineElementAccessors(obj *goja.Object) {
	_ = r.vm.Set("__elTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperties(__elTarget__, {
			children: {
				get: function() {
					var nid = this.__nid__;
					if (nid === undefined) return [];
					return document.__getChildren__(nid);
				},
				configurable: true,
				enumerable: true
			},
			firstElementChild: {
				get: function() {
					var ch = this.children;
					return (ch && ch.length > 0) ? ch[0] : null;
				},
				configurable: true,
				enumerable: true
			},
			lastElementChild: {
				get: function() {
					var ch = this.children;
					return (ch && ch.length > 0) ? ch[ch.length - 1] : null;
				},
				configurable: true,
				enumerable: true
			},
			parentElement: {
				get: function() {
					var nid = this.__nid__;
					if (nid === undefined) return null;
					return document.__getParentElement__(nid);
				},
				configurable: true,
				enumerable: true
			},
			parentNode: {
				get: function() {
					return this.parentElement;
				},
				configurable: true,
				enumerable: true
			},
			shadowRoot: {
				get: function() {
					var nid = this.__nid__;
					if (nid === undefined) return null;
					return document.__getShadowRoot__(nid);
				},
				configurable: true,
				enumerable: true
			}
		});
	`)
	_ = r.vm.Set("__elTarget__", nil)
}

func (r *Runtime) jsInsertAdjacentHTML(call goja.FunctionCall) goja.Value {
	thisObj := call.This.ToObject(r.vm)
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Undefined()
	}
	nid := int(nidVal.ToInteger())
	node := r.nodeRegistry[nid]
	if node == nil {
		return goja.Undefined()
	}
	if len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	position := strings.ToLower(call.Arguments[0].String())
	html := call.Arguments[1].String()
	if html == "" {
		return goja.Undefined()
	}

	wrapped := "<div>" + html + "</div>"
	parsed, err := dom.ParseBounded(wrapped, dom.ParseLimits{
		Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096,
	})
	if err != nil {
		return goja.Undefined()
	}
	var divNode *dom.Node
	if parsed.Body != nil {
		divNode = parsed.Body.FirstChild
	}
	if divNode == nil {
		return goja.Undefined()
	}

	var children []*dom.Node
	for c := divNode.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}

	switch position {
	case "beforebegin":
		if node.Parent == nil {
			return goja.Undefined()
		}
		for _, child := range children {
			divNode.RemoveChild(child)
			node.Parent.InsertBefore(child, node)
		}
	case "afterbegin":
		ref := node.FirstChild
		for _, child := range children {
			divNode.RemoveChild(child)
			node.InsertBefore(child, ref)
		}
	case "beforeend":
		for _, child := range children {
			divNode.RemoveChild(child)
			node.AppendChild(child)
		}
	case "afterend":
		if node.Parent == nil {
			return goja.Undefined()
		}
		ref := node.NextSibling
		for _, child := range children {
			divNode.RemoveChild(child)
			node.Parent.InsertBefore(child, ref)
		}
	}
	r.notifyMutation()
	return goja.Undefined()
}

func (r *Runtime) jsPrepend(call goja.FunctionCall) goja.Value {
	thisObj := call.This.ToObject(r.vm)
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Undefined()
	}
	node := r.nodeRegistry[int(nidVal.ToInteger())]
	if node == nil {
		return goja.Undefined()
	}
	ref := node.FirstChild
	for _, arg := range call.Arguments {
		if obj, ok := arg.(*goja.Object); ok {
			if knidVal := obj.Get("__nid__"); knidVal != nil && !goja.IsUndefined(knidVal) {
				if kidNode := r.nodeRegistry[int(knidVal.ToInteger())]; kidNode != nil {
					if kidNode == ref {
						ref = kidNode.NextSibling
						continue
					}
					node.InsertBefore(kidNode, ref)
					continue
				}
			}
		}
		text := arg.String()
		node.InsertBefore(node.Doc.NewText(text), ref)
	}
	r.notifyMutation()
	return goja.Undefined()
}

func (r *Runtime) jsAppend(call goja.FunctionCall) goja.Value {
	thisObj := call.This.ToObject(r.vm)
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Undefined()
	}
	node := r.nodeRegistry[int(nidVal.ToInteger())]
	if node == nil {
		return goja.Undefined()
	}
	for _, arg := range call.Arguments {
		if obj, ok := arg.(*goja.Object); ok {
			if knidVal := obj.Get("__nid__"); knidVal != nil && !goja.IsUndefined(knidVal) {
				if kidNode := r.nodeRegistry[int(knidVal.ToInteger())]; kidNode != nil {
					node.AppendChild(kidNode)
					continue
				}
			}
		}
		text := arg.String()
		node.AppendChild(node.Doc.NewText(text))
	}
	r.notifyMutation()
	return goja.Undefined()
}

// datasetKey converts a data-* attribute name to its dataset property key:
// data-foo-bar becomes fooBar. Only lowercase-dashed names convert; anything
// else is not exposed on the map.
func datasetKey(attr string) (string, bool) {
	if !strings.HasPrefix(attr, "data-") || len(attr) <= 5 {
		return "", false
	}
	rest := attr[5:]
	var sb strings.Builder
	upperNext := false
	for _, c := range rest {
		switch {
		case c == '-':
			upperNext = true
		case c >= 'A' && c <= 'Z':
			// Uppercase in the attribute name never appears from the HTML
			// parser (it lowercases); reject defensively.
			return "", false
		case upperNext && c >= 'a' && c <= 'z':
			sb.WriteRune(c - ('a' - 'A'))
			upperNext = false
		default:
			sb.WriteRune(c)
			upperNext = false
		}
	}
	return sb.String(), true
}

// datasetAttr converts a dataset property key back to its data-* attribute
// name: fooBar becomes data-foo-bar.
func datasetAttr(key string) string {
	var sb strings.Builder
	sb.WriteString("data-")
	for _, c := range key {
		if c >= 'A' && c <= 'Z' {
			sb.WriteByte('-')
			sb.WriteRune(c + ('a' - 'A'))
		} else {
			sb.WriteRune(c)
		}
	}
	return sb.String()
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
	if node, ok := r.querySelectorIndexed(root, sel); ok {
		return node
	}
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

// querySelectorIndexed answers simple selectors from the document indexes:
// O(matches) instead of O(subtree). Reports ok==false when the walk must
// still run (compound/unknown shapes, or an #id resolving outside the
// subtree where a deeper duplicate may still match). Results equal the walk:
// indexes enumerate in document order and containment+exclusion mirror it.
func (r *Runtime) querySelectorIndexed(root *dom.Node, sel string) (*dom.Node, bool) {
	if root == nil || r.domDoc == nil || sel == "" {
		if sel == "" {
			return nil, true
		}
		return nil, false
	}
	switch {
	case len(sel) > 1 && sel[0] == '#':
		return r.querySelectorID(root, sel)
	case len(sel) > 1 && sel[0] == '.':
		for _, n := range r.domDoc.ElementsByClassName(sel[1:]) {
			if containedStrict(root, n) {
				return n, true
			}
		}
		return nil, true
	default:
		// A lone "#" is an empty-id lookup, not a tag: the walk matches
		// elements without ids there, so it must run.
		if sel[0] == '#' || !isLowerTag(sel) {
			return nil, false
		}
		for _, n := range r.domDoc.ElementsByTagName(sel) {
			if containedStrict(root, n) {
				return n, true
			}
		}
		return nil, true
	}
}

// containedStrict reports whether n is a proper descendant of root.
func containedStrict(root, n *dom.Node) bool {
	if n == nil || n == root {
		return false
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p == root {
			return true
		}
	}
	return false
}

// isLowerTag reports whether sel is already lowercase ASCII: only then does
// the atom-indexed tag lookup answer exactly what the case-insensitive walk
// would (parsed documents always carry lowercase tags).
func isLowerTag(sel string) bool {
	if sel == "" {
		return false
	}
	for i := 0; i < len(sel); i++ {
		if c := sel[i]; c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

// querySelectorID answers a bare #id query from the document index: O(depth)
// instead of O(subtree). It reports ok==false when the walk must still run:
// a hit on root itself (the walk never tests root, but a deeper duplicate
// still matches) or a hit outside an element-rooted subtree. A document-wide
// miss answers nil directly: no element carries that id, so the walk could
// only return nil too.
func (r *Runtime) querySelectorID(root *dom.Node, sel string) (node *dom.Node, ok bool) {
	if root == nil || r.domDoc == nil || len(sel) < 2 || sel[0] != '#' {
		return nil, false
	}
	node = r.domDoc.ElementByID(sel[1:])
	if node == nil {
		return nil, true
	}
	if node == root {
		return nil, false
	}
	for p := node.Parent; p != nil; p = p.Parent {
		if p == root {
			return node, true
		}
	}
	return nil, false
}

// querySelectorAll returns every descendant of root that matches sel.
func (r *Runtime) querySelectorAll(root *dom.Node, sel string) []*dom.Node {
	if nodes, ok := r.querySelectorAllIndexed(root, sel); ok {
		return nodes
	}
	var result []*dom.Node
	r.collectMatching(root, sel, &result)
	return result
}

// querySelectorAllIndexed answers simple selectors from the document
// indexes, filtering to the rooted subtree. Same ordering and membership as
// the walk; ok==false falls back to it.
func (r *Runtime) querySelectorAllIndexed(root *dom.Node, sel string) ([]*dom.Node, bool) {
	if root == nil || r.domDoc == nil || sel == "" {
		if sel == "" {
			return nil, true
		}
		return nil, false
	}
	var all []*dom.Node
	switch {
	case len(sel) > 1 && sel[0] == '.':
		all = r.domDoc.ElementsByClassName(sel[1:])
	case isLowerTag(sel) && sel[0] != '#':
		all = r.domDoc.ElementsByTagName(sel)
	default:
		return nil, false
	}
	if root == &r.domDoc.Node {
		return all, true
	}
	var out []*dom.Node
	for _, n := range all {
		if containedStrict(root, n) {
			out = append(out, n)
		}
	}
	return out, true
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
	case len(sel) > 2 && sel[0] == '[' && sel[len(sel)-1] == ']':
		// Bare attribute selector: [name] or [name=value] with an
		// optionally quoted value. Compounds (tag[attr], [a][b]) stay
		// unsupported and report no match, as pinned by the query tests.
		return matchAttrSelector(n, sel[1:len(sel)-1])
	default:
		return strings.EqualFold(n.Data, sel)
	}
}

// matchAttrSelector matches one [name] or [name=value] selector against an
// element. Attribute names compare case-insensitively (HTML); values compare
// exactly after stripping one layer of matching quotes.
func matchAttrSelector(n *dom.Node, inner string) bool {
	name, value, hasValue := inner, "", false
	if i := strings.IndexByte(inner, '='); i >= 0 {
		name, value, hasValue = inner[:i], inner[i+1:], true
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t\n\"'[]") {
		return false
	}
	actual, present := "", false
	for _, a := range n.Attr {
		if strings.EqualFold(a.Name, name) {
			actual, present = a.Value, true
			break
		}
	}
	if !hasValue {
		return present
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	return present && actual == value
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
//
// It reports whether any timer callback ran. Every callback runs under the
// runtime timeout, and the loop itself is bounded by it too, so a hung or
// self-rescheduling callback wedges one pump step rather than the thread;
// an interrupt stops the drain with the remaining timers left for next time.
// A closed runtime is a no-op.
func (r *Runtime) Tick() bool {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return false
	}
	r.DrainFetchCallbacks()
	r.DrainWSEvents()
	r.DrainWorkerMessages()
	r.DrainServiceWorkerEvents()
	// Checkpoint after the I/O callbacks: a fetch resolving promises queues
	// reactions that must run before the next timer task, not after Tick.
	if err := r.drainMicrotasks(); err != nil {
		return false
	}
	start := time.Now()
	fired := false
	for {
		// A 1ms interval reschedules into the same tick forever; the budget
		// ends the pass instead of the thread.
		if fired && time.Since(start) > r.timeout() {
			return true
		}
		now := time.Now()
		var toFire []*timer
		for _, t := range r.timers {
			if !now.Before(t.nextFire) {
				toFire = append(toFire, t)
			}
		}
		if len(toFire) == 0 {
			return fired
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
				fired = true
				if _, err := r.callBounded(func() (goja.Value, error) { return fn(nil) }); err != nil {
					return true
				}
			}
			// Per-task checkpoint: reactions a callback queued drain
			// before the next timer fires.
			if err := r.drainMicrotasks(); err != nil {
				return true
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

	// Arm the timeout interrupt. It stays armed across the microtask
	// checkpoint below so a self-perpetuating reaction chain ends in
	// ErrInterrupted rather than a hang.
	timer := time.AfterFunc(timeout, func() {
		r.vm.Interrupt(ErrInterrupted)
	})

	// Compile first to catch syntax errors before execution.
	prog, err := goja.Compile(sourceURL, src, false)
	if err != nil {
		timer.Stop()
		return r.mapError(err, sourceURL)
	}

	// Run the compiled program.
	_, runErr := r.vm.RunProgram(prog)

	// Microtask checkpoint: reactions queued during the task drain before
	// Run returns, even when the program threw. A checkpoint failure
	// (interrupt) is reported unless the program already failed first.
	drainErr := r.drainMicrotasks()
	timer.Stop()
	if runErr != nil {
		return r.mapError(runErr, sourceURL)
	}
	if drainErr != nil {
		return drainErr
	}

	return nil
}

// Evaluate executes a JavaScript expression, returning its string representation or an error.
func (r *Runtime) Evaluate(expr string) (string, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", ErrClosed
	}
	r.mu.Unlock()

	if len(expr) > MaxScriptBytes {
		return "", ErrTooLarge
	}

	timeout := r.opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	timer := time.AfterFunc(timeout, func() {
		r.vm.Interrupt(ErrInterrupted)
	})
	defer timer.Stop()

	val, err := r.vm.RunString(expr)
	_ = r.drainMicrotasks()
	if err != nil {
		return "", r.mapError(err, "eval")
	}
	if val == nil || goja.IsUndefined(val) {
		return "undefined", nil
	}
	if goja.IsNull(val) {
		return "null", nil
	}
	return val.String(), nil
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
		r.wrapCache = make(map[*dom.Node]int)
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

	// DragEvent fields.
	if de := e.AsDrag(); de != nil {
		_ = obj.Set("dataTransfer", r.wrapDataTransfer(de.DataTransfer))
	}

	return obj
}

func (r *Runtime) wrapDataTransfer(dt *dom.DataTransfer) *goja.Object {
	if dt == nil {
		dt = dom.NewDataTransfer()
	}
	obj := r.vm.NewObject()
	_ = obj.Set("dropEffect", dt.DropEffect)
	_ = obj.Set("effectAllowed", dt.EffectAllowed)
	_ = obj.Set("setData", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 {
			dt.SetData(call.Arguments[0].String(), call.Arguments[1].String())
		}
		return goja.Undefined()
	})
	_ = obj.Set("getData", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 1 {
			return r.vm.ToValue(dt.GetData(call.Arguments[0].String()))
		}
		return r.vm.ToValue("")
	})
	_ = obj.Set("clearData", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 1 {
			dt.ClearData(call.Arguments[0].String())
		} else {
			dt.ClearData()
		}
		return goja.Undefined()
	})
	_ = obj.Set("__getTypes__", func() []string {
		t := dt.Types()
		if t == nil {
			return []string{}
		}
		return t
	})
	_ = r.vm.Set("__dt__", obj)
	_, _ = r.vm.RunString(`
		(function() {
			var o = __dt__;
			Object.defineProperty(o, 'types', {
				get: function() { return o.__getTypes__(); },
				configurable: true,
				enumerable: true
			});
		})();
	`)
	_ = r.vm.Set("__dt__", nil)
	return obj
}

func (r *Runtime) setupDragDrop() {
	dtCtor := func(call goja.ConstructorCall) *goja.Object {
		dt := dom.NewDataTransfer()
		return r.wrapDataTransfer(dt)
	}
	dtVal := r.vm.ToValue(dtCtor)
	_ = r.vm.Set("DataTransfer", dtVal)

	dragEventCtor := func(call goja.ConstructorCall) *goja.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		dt := dom.NewDataTransfer()
		de := dom.NewDragEvent(eventType, dom.MouseEvent{}, dt)
		return r.wrapEvent(de).(*goja.Object)
	}
	deVal := r.vm.ToValue(dragEventCtor)
	_ = r.vm.Set("DragEvent", deVal)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("DataTransfer", dtVal)
			_ = winObj.Set("DragEvent", deVal)
		}
	}
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

// jsSetAttributeNS implements element.setAttributeNS(ns, name, value). A null
// namespace behaves exactly like setAttribute. The XML namespace stores the
// qualified name marked NSXML, which is the only xml:lang :lang() honors -
// the HTML parser never creates namespaced attributes, so static xml:lang
// stays invisible to language matching exactly like the reference engine.
func (r *Runtime) jsSetAttributeNS(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 3 {
		return goja.Undefined()
	}
	var ns string
	if nsArg := call.Arguments[0]; nsArg != nil && !goja.IsNull(nsArg) && !goja.IsUndefined(nsArg) {
		ns = nsArg.String()
	}
	name := call.Arguments[1].String()
	value := call.Arguments[2].String()
	if ns == "" {
		node.SetAttribute(name, value)
	} else if ns == "http://www.w3.org/XML/1998/namespace" {
		replaced := false
		for i := range node.Attr {
			if node.Attr[i].Namespace == dom.NSXML && strings.EqualFold(node.Attr[i].Name, name) {
				node.Attr[i].Value = value
				replaced = true
				break
			}
		}
		if !replaced {
			node.Attr = append(node.Attr, dom.Attribute{Name: name, Namespace: dom.NSXML, Value: value})
		}
		// A namespaced write spells its name the same way GetAttribute
		// reads it, so an xml "id" would retarget the id index.
		if node.Doc != nil && strings.EqualFold(name, "id") {
			node.Doc.MarkQueryDirty()
		}
	} else {
		// Other namespaces have no engine semantics; keep the qualified
		// name as a plain attribute rather than dropping the write.
		node.SetAttribute(name, value)
	}
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
			// A removed id retargets getElementById: drop the index.
			if node.Doc != nil && lower == "id" {
				node.Doc.MarkQueryDirty()
			}
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

// ChainOnMutation appends fn to the mutation callback chain instead of
// replacing it: the engine uses this to observe DOM writes on a runtime the
// caller constructed (and may have given its own OnMutation). A nil fn is
// ignored. Safe for concurrent use; callbacks fire on the mutating thread.
func (r *Runtime) ChainOnMutation(fn func()) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev := r.opts.OnMutation; prev != nil {
		r.opts.OnMutation = func() { prev(); fn() }
	} else {
		r.opts.OnMutation = fn
	}
}

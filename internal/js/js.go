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

// Runtime executes scripts for one document.
type Runtime struct {
	vm     *goja.Runtime
	opts   Options
	title  string
	closed bool
	mu     sync.Mutex
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
		opts:  opts,
		title: opts.Title,
		vm:    goja.New(),
	}

	r.setupConsole(consoleWriter)
	r.setupDocument()
	r.setupWindow()
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
	// setTimeout on window
	_, _ = r.vm.RunString(`
		window.setTimeout = function() { throw new Error("setTimeout is not implemented"); };
	`)

	// document.querySelector
	_, _ = r.vm.RunString(`
		document.querySelector = function() { throw new Error("querySelector is not implemented"); };
	`)

	// fetch on window
	_, _ = r.vm.RunString(`
		window.fetch = function() { throw new Error("fetch is not implemented"); };
	`)
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

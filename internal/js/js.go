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
	"io"
	"strconv"
	"time"
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
	// ErrNotImplemented stands in for the runtime itself. Every method returns
	// it, so the contract tests fail as assertions with a reason rather than as
	// build errors.
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
type Runtime struct{}

// New builds a runtime for one document.
func New(opts Options) (*Runtime, error) { return nil, ErrNotImplemented }

// Run evaluates one classic script to completion, returning an *Error for an
// uncaught exception, ErrInterrupted if the bound fired, or ErrClosed after
// Close.
func (r *Runtime) Run(src, sourceURL string) error { return ErrNotImplemented }

// Global reads a top-level binding created by a script.
func (r *Runtime) Global(name string) (Value, bool) { return Value{}, false }

// Title returns what the scripts left document.title at.
func (r *Runtime) Title() string { return "" }

// Interrupt stops a Run in progress, from any goroutine.
func (r *Runtime) Interrupt() {}

// Close releases the realm. Run after Close reports ErrClosed.
func (r *Runtime) Close() error { return ErrNotImplemented }

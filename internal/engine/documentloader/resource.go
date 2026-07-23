// Package documentloader coordinates document-level subresource lifecycle.
//
// Resource discovery, URL resolution, CSP gating, and navigation-scoped
// scheduling live here. The renderer and JS runtime consume this package's
// outputs; they do not perform their own network or scheduling work.
package documentloader

import (
	"errors"
	"fmt"
)

// ResourceKind identifies the type of a subresource discovered during
// document parsing.
type ResourceKind uint8

const (
	// KindCSS represents <link rel="stylesheet" href>.
	KindCSS ResourceKind = iota + 1
	// KindScript represents <script src> or inline <script>.
	KindScript
	// KindImage represents <img src> and (M7) CSS url(...) images.
	KindImage
	// KindFont represents @font-face src: url(...) discovered inside a
	// stylesheet. The coordinator fetches the bytes and reports them
	// via Callbacks.OnFont.
	KindFont
)

// Default to zero so callers can omit ScriptMode for classic scripts;
// the ResourceKind is always set explicitly by the caller.

// String returns a stable, human-readable label for logging and metrics.
func (k ResourceKind) String() string {
	switch k {
	case KindCSS:
		return "css"
	case KindScript:
		return "script"
	case KindImage:
		return "image"
	case KindFont:
		return "font"
	default:
		return fmt.Sprintf("unknown_kind_%d", uint8(k))
	}
}

// ScriptMode describes how a discovered script should be executed. M1
// supports only ScriptModeClassic; defer/async/module are accepted but
// reported via unsupported-feature hooks in later milestones.
type ScriptMode uint8

const (
	// ScriptModeClassic is the zero value and a parser-blocking classic
	// script. M1 handles this mode. Callers can omit ScriptMode for
	// classic scripts; non-zero values select other modes.
	ScriptModeClassic ScriptMode = iota
	// ScriptModeAsync executes when ready without preserving order.
	// M5 will add support.
	ScriptModeAsync
	// ScriptModeDefer executes in document order after parsing.
	// M5 will add support.
	ScriptModeDefer
	// ScriptModeModule is an ES module. Out of scope; reported via the
	// engine fallback layer.
	ScriptModeModule
)

// String returns a stable label for the script mode.
func (m ScriptMode) String() string {
	switch m {
	case ScriptModeClassic:
		return "classic"
	case ScriptModeAsync:
		return "async"
	case ScriptModeDefer:
		return "defer"
	case ScriptModeModule:
		return "module"
	default:
		return fmt.Sprintf("unknown_mode_%d", uint8(m))
	}
}

// IsUnsupported reports whether the script mode is out of scope for the
// current milestone (M1: only classic is supported).
func (m ScriptMode) IsUnsupported() bool {
	return m != ScriptModeClassic
}

// Resource describes one subresource discovered during document parsing.
// Position is the document-order index assigned by the streaming parser
// (or by the caller if the resource originates outside the parser).
//
// The shape intentionally carries more fields than dom.Resource today;
// M2 will extend dom.Resource and bridge the two.
type Resource struct {
	Kind       ResourceKind
	URL        string // raw reference as it appears in HTML
	Position   int    // document order; ties broken by Position
	ScriptMode ScriptMode
	// Inline is true for <script>...</script> with no src attribute.
	// When true, URL is empty and Source carries the script body.
	Inline bool
	// Source carries the body for inline scripts. External scripts leave
	// this empty; the coordinator populates Source after fetch.
	Source []byte
	// Integrity and CrossOrigin are accepted for forward compatibility
	// with SRI / CORS-aware policies (M5+). M1 does not act on them.
	Integrity   string
	CrossOrigin string
}

// CSSResult is the coordinator's output for one successfully fetched
// stylesheet. Source is the raw bytes returned by the fetcher. Position
// preserves document order so callers can merge rules in source order
// even when responses complete out of order.
type CSSResult struct {
	URL      string
	Resolved string
	Source   []byte
	Position int
}

// ScriptResult is the coordinator's output for one script (inline or
// external). Inline scripts are emitted as soon as HandleResource is
// called with Inline=true; external scripts are emitted after the fetch
// completes.
type ScriptResult struct {
	URL      string
	Resolved string
	Source   []byte
	Inline   bool
	Mode     ScriptMode
	Position int
}

// ImageResult is the coordinator's output for one image load. M1 emits
// the result; downstream caching/decoding is the caller's responsibility.
type ImageResult struct {
	URL      string
	Resolved string
	Source   []byte
	Position int
}

// FontResult is the coordinator's output for one font load (M7). Fonts
// are discovered via @font-face src: url(...) inside stylesheets. The
// raw bytes are returned; decoding to glyphs is the renderer's job.
type FontResult struct {
	URL      string
	Resolved string
	Source   []byte
	Position int
}

// LifecycleEvent identifies coarse-grained lifecycle transitions the
// coordinator reports via Callbacks.OnLifecycle.
//
// Order of emission for one navigation:
//  1. EventDOMContentLoaded — after HandleDocumentEnd drains classic
//     and deferred scripts in source order. Async scripts may still be
//     in flight at this point.
//  2. EventLoad — after DOMContentLoaded AND every async script has
//     finished executing.
//  3. EventDocumentEnd — final cleanup signal. Currently emitted at
//     the same time as EventLoad; kept for backward compatibility with
//     the M1 contract.
type LifecycleEvent uint8

const (
	// EventDOMContentLoaded fires after HandleDocumentEnd drains
	// classic and deferred scripts in source order. This matches the
	// HTML spec: DOMContentLoaded fires after the document is parsed
	// and defer scripts execute.
	EventDOMContentLoaded LifecycleEvent = iota + 1
	// EventLoad fires after DOMContentLoaded and every async script
	// has finished executing.
	EventLoad
	// EventDocumentEnd is the final cleanup signal. Kept for backward
	// compatibility with M1 callers; emits at the same time as
	// EventLoad in M5.
	EventDocumentEnd
)

// String returns a stable label for the lifecycle event.
func (e LifecycleEvent) String() string {
	switch e {
	case EventDOMContentLoaded:
		return "dom_content_loaded"
	case EventLoad:
		return "load"
	case EventDocumentEnd:
		return "document_end"
	default:
		return fmt.Sprintf("unknown_event_%d", uint8(e))
	}
}

// ErrResourceSkipped indicates that the coordinator intentionally did not
// fetch a resource (CSP block, unsupported mode, invalid URL, or inactive
// navigation). It is reported via Callbacks.OnError so callers can record
// a metric without treating it as a network failure.
var ErrResourceSkipped = errors.New("documentloader: resource skipped")

// SkippedError wraps ErrResourceSkipped with a reason. Callers can use
// errors.Is(err, ErrResourceSkipped) to detect any skip.
type SkippedError struct {
	Reason string
}

func (e *SkippedError) Error() string {
	if e.Reason == "" {
		return "documentloader: resource skipped"
	}
	return "documentloader: resource skipped: " + e.Reason
}

// Unwrap lets callers use errors.Is(err, ErrResourceSkipped).
func (e *SkippedError) Unwrap() error { return ErrResourceSkipped }

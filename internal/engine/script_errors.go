package engine

import (
	"errors"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/js"
)

// A document's own exceptions are evidence, not noise, but they arrive from
// untrusted input: a page can throw as often as it can be served and each
// message can be as long as its author likes. Both are therefore bounded here,
// and the total count is kept separately so a capped window never reads as "the
// page only broke once".
const (
	// MaxScriptErrors is how many distinct exceptions one session retains.
	MaxScriptErrors = 8
	// maxScriptErrorMessage truncates one message. The source and line ride
	// along uncut, so a page author can still find the file that broke.
	maxScriptErrorMessage = 200
)

// ScriptError is one uncaught exception from a document script. It is a value,
// not an error: the load continued past it, which is the behaviour browsers
// have and the reason a broken ad tag does not take down a page.
type ScriptError struct {
	Source  string
	Line    int
	Message string
	// TimedOut marks the exception as the host's deadline stopping user code,
	// not a script that threw. The chrome turns it into the unresponsive-script
	// prompt instead of a console line.
	TimedOut bool
}

func (e ScriptError) String() string {
	return e.Source + ":" + strconv.Itoa(e.Line) + ": " + e.Message
}

// recordScriptError keeps one discarded exception. It never changes control
// flow: callers carry on running the rest of the document either way.
func (s *Session) recordScriptError(err error) {
	if err == nil {
		return
	}
	var je *js.Error
	source, line, message := "script", 0, err.Error()
	if errors.As(err, &je) {
		source, line, message = je.SourceURL, je.Line, je.Message
	}
	if source == "" {
		source = "script"
	}
	// The runtime flags its own deadline, and clearing it here attributes the
	// stop to this error exactly once. A host Interrupt leaves the flag clear.
	timedOut := false
	if errors.Is(err, js.ErrInterrupted) {
		timedOut = s.jsRT.TakeTimedOut()
	}
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	s.scriptErrTotal++
	if timedOut {
		s.scriptTimeoutTotal++
	}
	if len(s.scriptErrs) >= MaxScriptErrors {
		return
	}
	// One error must stay one line. goja appends a stack trace to these
	// messages, and the text is the page's own: left alone it would print
	// several report lines per error and let a document forge lines that look
	// like the browser's diagnostics. Whitespace runs collapse to single
	// spaces, then the length cap applies.
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > maxScriptErrorMessage {
		// Byte truncation of untrusted text can split a UTF-8 sequence and put
		// invalid bytes on the report, so the tail is cleaned after the cut.
		message = strings.ToValidUTF8(message[:maxScriptErrorMessage], "") + "..."
	}
	s.scriptErrs = append(s.scriptErrs, ScriptError{Source: source, Line: line, Message: message, TimedOut: timedOut})
}

// ScriptErrors returns the retained exceptions, in the order the document
// threw them, oldest first. The window is capped at MaxScriptErrors;
// ScriptErrorCount reports how many there really were.
func (s *Session) ScriptErrors() []ScriptError {
	if s == nil {
		return nil
	}
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	if len(s.scriptErrs) == 0 {
		return nil
	}
	out := make([]ScriptError, len(s.scriptErrs))
	copy(out, s.scriptErrs)
	return out
}

// ScriptErrorCount is the total number of uncaught script errors the document
// produced, including the ones past the retained window.
func (s *Session) ScriptErrorCount() int {
	if s == nil {
		return 0
	}
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	return s.scriptErrTotal
}

// ScriptTimeoutCount is how many times the host deadline stopped user code in
// this document. A non-zero count is what the chrome turns into the
// unresponsive-script prompt.
func (s *Session) ScriptTimeoutCount() int {
	if s == nil {
		return 0
	}
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	return s.scriptTimeoutTotal
}

// HasScriptTimeout reports whether the document ran past the script deadline.
func (s *Session) HasScriptTimeout() bool {
	return s.ScriptTimeoutCount() > 0
}

// noteScriptTimeout attributes a deadline stop that no Run caller reported.
// Timer, animation-frame and event callbacks discard their errors on purpose,
// so the runtime's flag is the only evidence they ran too long.
func (s *Session) noteScriptTimeout() {
	if s == nil || s.jsRT == nil || !s.jsRT.TimedOut() {
		return
	}
	s.recordScriptError(js.ErrInterrupted)
}

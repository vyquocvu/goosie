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
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	s.scriptErrTotal++
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
	s.scriptErrs = append(s.scriptErrs, ScriptError{Source: source, Line: line, Message: message})
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

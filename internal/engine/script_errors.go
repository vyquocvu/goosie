package engine

import (
	"errors"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/js"
)

const (
	MaxScriptErrors       = 8
	maxScriptErrorMessage = 200
)

type ScriptError struct {
	Source   string
	Line     int
	Message  string
	TimedOut bool
}

func (e ScriptError) String() string {
	return e.Source + ":" + strconv.Itoa(e.Line) + ": " + e.Message
}

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
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > maxScriptErrorMessage {
		message = strings.ToValidUTF8(message[:maxScriptErrorMessage], "") + "..."
	}
	s.scriptErrs = append(s.scriptErrs, ScriptError{Source: source, Line: line, Message: message, TimedOut: timedOut})
}

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

func (s *Session) ScriptErrorCount() int {
	if s == nil {
		return 0
	}
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	return s.scriptErrTotal
}

func (s *Session) ScriptTimeoutCount() int {
	if s == nil {
		return 0
	}
	s.scriptErrMu.Lock()
	defer s.scriptErrMu.Unlock()
	return s.scriptTimeoutTotal
}

func (s *Session) HasScriptTimeout() bool {
	return s.ScriptTimeoutCount() > 0
}

func (s *Session) noteScriptTimeout() {
	if s == nil || s.jsRT == nil || !s.jsRT.TimedOut() {
		return
	}
	s.recordScriptError(js.ErrInterrupted)
}

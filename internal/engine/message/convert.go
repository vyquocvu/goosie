package message

import (
	"strconv"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/session"
)

// Event converts a session Event into a versioned Message.
// Only value-type fields cross the boundary—no pointers, no slices of
// pointers, no UI objects. The caller retains ownership of the original
// Event; the returned Message is a fresh, independent value.
func Event(ev session.Event, ts time.Time) *Message {
	m := &Message{Version: Version, Time: ts}

	navID := uint64(ev.NavID)

	switch ev.Type {
	case session.EventStateChange:
		m.Navigation = &Navigation{
			NavID: navID,
			URL:   ev.URL,
			State: ConvertState(ev.State),
		}
	case session.EventTitleChange:
		m.Title = &Title{
			NavID: navID,
			Title: ev.Title,
		}
	case session.EventURLChange:
		m.URL = &URL{
			NavID: navID,
			URL:   ev.URL,
		}
	case session.EventFirstPaint:
		m.Paint = &FirstPaint{
			NavID: navID,
		}
	case session.EventProgress:
		m.Progress = &Progress{
			NavID:    navID,
			Progress: ev.Progress,
		}
	case session.EventError:
		msg := ""
		if ev.Err != nil {
			msg = ev.Err.Error()
		}
		m.Error = &Error{
			NavID:   navID,
			Message: msg,
		}
	case session.EventSecuritySummary:
		src := ev.SecuritySummary
		m.Security = &SecuritySummary{
			URL:    src.URL,
			Scheme: src.Scheme,
			Secure: src.Secure,
			Error:  src.Error,
		}
	case session.EventDownload:
		src := ev.Download
		m.Download = &Download{
			URL:          src.URL,
			Status:       string(src.Status),
			BytesWritten: src.BytesWritten,
			Error:        src.Error,
		}
	}

	return m
}

// ConvertState maps a session.State to the corresponding message NavState.
func ConvertState(s session.State) NavState {
	switch s {
	case session.StateCreated:
		return StateCreated
	case session.StateNavigating:
		return StateNavigating
	case session.StateParsing:
		return StateParsing
	case session.StateInteractive:
		return StateInteractive
	case session.StateComplete:
		return StateComplete
	case session.StateCancelled:
		return StateCancelled
	case session.StateFailed:
		return StateFailed
	case session.StateClosed:
		return StateClosed
	default:
		return NavState(s)
	}
}

// ParseUint64 is a helper for callers that need to parse a uint64 navID
// from a string representation (e.g. from JSON or logs).
func ParseUint64(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

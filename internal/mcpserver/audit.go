package mcpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// AuditEvent represents a single audit log entry.
// Audit logs are emitted to stderr only and never include
// sensitive page content (passwords, typed values, cookies).
type AuditEvent struct {
	Timestamp time.Time         `json:"ts"`
	Type      string            `json:"type"`
	ContextID string            `json:"contextId,omitempty"`
	Tool      string            `json:"tool,omitempty"`
	Outcome   string            `json:"outcome"` // success, error, denied
	ErrorCode string            `json:"errorCode,omitempty"`
	Duration  int64             `json:"durationMs,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AuditLogger writes structured audit events to a writer (stderr by default).
// Events are written as single-line JSON for easy ingestion by log aggregators.
type AuditLogger struct {
	mu     sync.Mutex
	writer io.Writer
	closed bool
}

// NewAuditLogger creates a logger that writes to stderr.
func NewAuditLogger() *AuditLogger {
	return &AuditLogger{writer: os.Stderr}
}

// NewAuditLoggerTo creates a logger that writes to a custom writer (for testing).
func NewAuditLoggerTo(w io.Writer) *AuditLogger {
	return &AuditLogger{writer: w}
}

// Log writes an audit event to the writer.
func (a *AuditLogger) Log(event AuditEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = a.writer.Write(data)
}

// LogToolCall records a tool invocation outcome.
func (a *AuditLogger) LogToolCall(contextID, tool, outcome, errorCode string, duration time.Duration, metadata map[string]string) {
	a.Log(AuditEvent{
		Type:      "tool_call",
		ContextID: contextID,
		Tool:      tool,
		Outcome:   outcome,
		ErrorCode: errorCode,
		Duration:  duration.Milliseconds(),
		Metadata:  metadata,
	})
}

// LogContextAction records context lifecycle events.
func (a *AuditLogger) LogContextAction(contextID, action, outcome string) {
	a.Log(AuditEvent{
		Type:      "context_" + action,
		ContextID: contextID,
		Outcome:   outcome,
	})
}

// LogServerEvent records server-level events (startup, shutdown, errors).
func (a *AuditLogger) LogServerEvent(eventType, outcome string, metadata map[string]string) {
	a.Log(AuditEvent{
		Type:     eventType,
		Outcome:  outcome,
		Metadata: metadata,
	})
}

// Close disables further audit logging.
func (a *AuditLogger) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
}

// Sensitive keys are never recorded in audit logs.
var sensitiveKeys = map[string]bool{
	"password":    true,
	"passwd":      true,
	"secret":      true,
	"token":       true,
	"apikey":      true,
	"api_key":     true,
	"authorization": true,
	"cookie":      true,
	"set-cookie":  true,
	"typed":       true,
	"value":       true, // for password fields
}

// SanitizeMetadata removes sensitive fields from metadata before logging.
func SanitizeMetadata(meta map[string]string) map[string]string {
	if meta == nil {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		if sensitiveKeys[k] {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}

// Truncate returns a string truncated to n bytes, appending an indicator.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 4 {
		return s[:n]
	}
	return s[:n-4] + "..."
}

// SafeError returns an error message safe for protocol responses.
// It removes paths, stack traces, and credential strings.
func SafeError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	// Remove anything after a newline (often stacks)
	if idx := indexNewline(s); idx >= 0 {
		s = s[:idx]
	}
	return Truncate(s, 512)
}

func indexNewline(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return i
		}
	}
	return -1
}

// RedactURL strips userinfo and embeds credentials from a URL string
// before logging or returning to a client.
func RedactURL(raw string) string {
	if raw == "" {
		return raw
	}
	// Find scheme separator
	schemeEnd := -1
	for i := 0; i < len(raw)-2; i++ {
		if raw[i] == ':' && raw[i+1] == '/' && raw[i+2] == '/' {
			schemeEnd = i + 3
			break
		}
	}
	if schemeEnd < 0 {
		return raw
	}
	// Find authority end (next /, ?, #)
	authEnd := len(raw)
	for i := schemeEnd; i < len(raw); i++ {
		if raw[i] == '/' || raw[i] == '?' || raw[i] == '#' {
			authEnd = i
			break
		}
	}
	authority := raw[schemeEnd:authEnd]
	// Strip userinfo
	if atIdx := indexAt(authority); atIdx >= 0 {
		authority = authority[atIdx+1:]
	}
	return raw[:schemeEnd] + authority + raw[authEnd:]
}

func indexAt(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return i
		}
	}
	return -1
}

// FormatSize returns a human-readable size string.
func FormatSize(bytes int) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

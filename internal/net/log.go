package net

import (
	"sync"
	"time"
)

// RequestLogEntry records one network request made by the browser service.
type RequestLogEntry struct {
	Method      string
	URL         string
	Status      int
	ContentType string
	Bytes       int64
	CacheHit    bool
	Error       string
	StartedAt   time.Time
	Duration    time.Duration
}

// RequestLog stores request log entries safely across concurrent fetches.
type RequestLog struct {
	mu      sync.Mutex
	entries []RequestLogEntry
}

func NewRequestLog() *RequestLog {
	return &RequestLog{entries: make([]RequestLogEntry, 0)}
}

func (l *RequestLog) Add(entry RequestLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *RequestLog) Entries() []RequestLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := make([]RequestLogEntry, len(l.entries))
	copy(entries, l.entries)
	return entries
}

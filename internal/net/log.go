package net

import (
	"sync"
	"time"
)

// TimingPhaseName represents a named phase in an HTTP request lifecycle.
type TimingPhaseName string

const (
	PhaseDNS     TimingPhaseName = "DNS"
	PhaseConnect TimingPhaseName = "Connect"
	PhaseTLS     TimingPhaseName = "TLS"
	PhaseRequest TimingPhaseName = "Request"
	PhaseResponse TimingPhaseName = "Response"
	PhaseDownload TimingPhaseName = "Download"
)

// TimingPhase records the duration of one phase of an HTTP request.
type TimingPhase struct {
	Name     TimingPhaseName `json:"name"`
	Duration time.Duration   `json:"duration"`
}

// RequestLogEntry records one network request made by the browser service.
type RequestLogEntry struct {
	Method        string
	URL           string
	Status        int
	ContentType   string
	Bytes         int64
	CacheHit      bool
	Error         string
	StartedAt     time.Time
	Duration      time.Duration
	TimingPhases  []TimingPhase
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

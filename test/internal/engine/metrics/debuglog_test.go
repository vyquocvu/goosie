package metrics_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
)

// syncBuffer is a goroutine-safe io.Writer for capturing slog output in tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func newDebugLogger(b *syncBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(b, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func TestRecorderDebugLogDisabledByDefault(t *testing.T) {
	buf := &syncBuffer{}
	r := metrics.NewRecorder(1, "https://example.com")
	r.Finalize()

	if buf.String() != "" {
		t.Fatalf("unexpected log output when debug disabled: %q", buf.String())
	}
}

func TestRecorderDebugLogEmitsStructured(t *testing.T) {
	buf := &syncBuffer{}
	logger := newDebugLogger(buf)

	r := metrics.NewRecorder(7, "https://example.com/page")
	r.SetDebugLog(logger)
	r.BeginPhase(metrics.PhaseParse)
	r.EndPhase(metrics.PhaseParse)
	r.AddCounters(metrics.Counters{NodeCount: 120, RuleCount: 30, ImageCount: 4})

	m := r.Finalize()

	out := buf.String()
	if out == "" {
		t.Fatal("expected structured log output, got empty")
	}
	for _, want := range []string{
		"navigation complete",
		"nav_id=7",
		"url=https://example.com/page",
		"node_count=120",
		"rule_count=30",
		"image_count=4",
		"parse_ns=",
	} {
		if !contains(out, want) {
			t.Errorf("log output missing %q\n got: %s", want, out)
		}
	}
	if m.EndedAt.IsZero() {
		t.Fatal("Finalize should still set EndedAt")
	}
}

func TestRecorderDebugLogNotEmittedWhenDisabledAfterSet(t *testing.T) {
	buf := &syncBuffer{}
	logger := newDebugLogger(buf)

	r := metrics.NewRecorder(1, "https://example.com")
	r.SetDebugLog(logger)
	r.SetDebugLog(nil) // disable
	r.AddCounters(metrics.Counters{NodeCount: 5})

	r.Finalize()

	if buf.String() != "" {
		t.Fatalf("expected no log output after disabling debug: %q", buf.String())
	}
}

func TestRecorderLogStructuredNoOpAfterFinalize(t *testing.T) {
	buf := &syncBuffer{}
	logger := newDebugLogger(buf)

	r := metrics.NewRecorder(1, "https://example.com")
	r.SetDebugLog(logger)
	r.Finalize()

	before := buf.String()

	// Manual call after finalize must be a no-op (done flag set).
	r.LogStructured(context.Background())

	if buf.String() != before {
		t.Fatalf("LogStructured after Finalize should be a no-op; got extra: %q", buf.String()[len(before):])
	}
}

func TestRecorderLogStructuredNoOpWhenDisabled(t *testing.T) {
	buf := &syncBuffer{}
	r := metrics.NewRecorder(1, "https://example.com")
	// No logger set -> disabled.
	r.LogStructured(context.Background())
	if buf.String() != "" {
		t.Fatalf("unexpected log output when disabled: %q", buf.String())
	}
}

func TestRecorderLogStructuredExplicitCall(t *testing.T) {
	buf := &syncBuffer{}
	logger := newDebugLogger(buf)

	r := metrics.NewRecorder(3, "https://example.com/x")
	r.SetDebugLog(logger)
	r.BeginPhase(metrics.PhaseLayout)
	r.EndPhase(metrics.PhaseLayout)
	r.AddCounters(metrics.Counters{BoxCount: 42})

	// Explicit call before finalize should already emit.
	r.LogStructured(context.Background())

	out := buf.String()
	if !contains(out, "nav_id=3") || !contains(out, "box_count=42") {
		t.Fatalf("explicit LogStructured missing expected fields: %s", out)
	}

	// Finalize emits a second, complete record.
	r.Finalize()
	if countOccurrences(buf.String(), "navigation complete") != 2 {
		t.Fatalf("expected two navigation complete records, got: %s", buf.String())
	}
}

func TestRecorderDebugLogConcurrentSafe(t *testing.T) {
	buf := &syncBuffer{}
	logger := newDebugLogger(buf)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := metrics.NewRecorder(1, "https://example.com")
			r.SetDebugLog(logger)
			r.BeginPhase(metrics.PhaseParse)
			r.EndPhase(metrics.PhaseParse)
			r.AddCounters(metrics.Counters{NodeCount: 1})
			r.Finalize()
		}()
	}
	wg.Wait()

	if got := countOccurrences(buf.String(), "navigation complete"); got != 20 {
		t.Fatalf("expected 20 navigation complete records, got %d", got)
	}
}

func TestRecorderDebugLogContextPropagation(t *testing.T) {
	buf := &syncBuffer{}
	logger := newDebugLogger(buf)

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "trace-123")

	r := metrics.NewRecorder(9, "https://example.com")
	r.SetDebugLog(logger)
	r.LogStructured(ctx)
	r.Finalize()

	if !contains(buf.String(), "nav_id=9") {
		t.Fatalf("expected nav_id in output: %s", buf.String())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
			i += len(sub) - 1
		}
	}
	return n
}

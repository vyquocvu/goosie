package js

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DefaultScriptPolicy
// ---------------------------------------------------------------------------

func TestDefaultScriptPolicy(t *testing.T) {
	p := DefaultScriptPolicy()
	if p.MaxSteps != 1_000_000 {
		t.Errorf("MaxSteps = %d, want 1000000", p.MaxSteps)
	}
	if p.MaxExecutionTime != 10*time.Second {
		t.Errorf("MaxExecutionTime = %v, want 10s", p.MaxExecutionTime)
	}
	if p.MaxTimers != 128 {
		t.Errorf("MaxTimers = %d, want 128", p.MaxTimers)
	}
	if p.MaxTaskQueueSize != 256 {
		t.Errorf("MaxTaskQueueSize = %d, want 256", p.MaxTaskQueueSize)
	}
	if p.Mode != DocumentModeFull {
		t.Errorf("Mode = %d, want Full", p.Mode)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — step limit
// ---------------------------------------------------------------------------

func TestScriptEnforcer_StepLimit(t *testing.T) {
	policy := ScriptPolicy{MaxSteps: 100}
	e := NewScriptEnforcer(policy)
	e.Start()

	if err := e.AddSteps(50); err != nil {
		t.Errorf("50 steps should be within limit: %v", err)
	}
	if err := e.AddSteps(51); err != ErrScriptStepLimit {
		t.Errorf("101 steps should trigger ErrScriptStepLimit, got %v", err)
	}
	if !e.IsAborted() {
		t.Error("enforcer should be aborted")
	}
	if e.AbortError() != ErrScriptStepLimit {
		t.Errorf("AbortError = %v, want ErrScriptStepLimit", e.AbortError())
	}
}

func TestScriptEnforcer_NoStepLimit(t *testing.T) {
	policy := ScriptPolicy{MaxSteps: 0} // no limit
	e := NewScriptEnforcer(policy)
	e.Start()

	if err := e.AddSteps(1_000_000); err != nil {
		t.Errorf("no step limit should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — time limit
// ---------------------------------------------------------------------------

func TestScriptEnforcer_TimeLimit(t *testing.T) {
	policy := ScriptPolicy{MaxSteps: 0, MaxExecutionTime: time.Millisecond}
	e := NewScriptEnforcer(policy)
	e.Start()

	time.Sleep(5 * time.Millisecond)
	if err := e.AddSteps(1); err != ErrScriptTimeout {
		t.Errorf("should trigger ErrScriptTimeout, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — timer limits
// ---------------------------------------------------------------------------

func TestScriptEnforcer_TimerLimit(t *testing.T) {
	policy := ScriptPolicy{MaxTimers: 2}
	e := NewScriptEnforcer(policy)

	if err := e.AcquireTimer(); err != nil {
		t.Fatal("first timer should succeed")
	}
	if err := e.AcquireTimer(); err != nil {
		t.Fatal("second timer should succeed")
	}
	if err := e.AcquireTimer(); err != ErrTimerLimit {
		t.Errorf("third timer should fail: %v", err)
	}

	e.ReleaseTimer()
	if err := e.AcquireTimer(); err != nil {
		t.Errorf("after release, timer should succeed: %v", err)
	}
}

func TestScriptEnforcer_ActiveTimers(t *testing.T) {
	policy := ScriptPolicy{MaxTimers: 10}
	e := NewScriptEnforcer(policy)

	e.AcquireTimer()
	e.AcquireTimer()
	if e.ActiveTimers() != 2 {
		t.Errorf("ActiveTimers = %d, want 2", e.ActiveTimers())
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — task queue limit
// ---------------------------------------------------------------------------

func TestScriptEnforcer_TaskQueueLimit(t *testing.T) {
	policy := ScriptPolicy{MaxTaskQueueSize: 2}
	e := NewScriptEnforcer(policy)

	if err := e.AcquireTaskSlot(); err != nil {
		t.Fatal("first task should succeed")
	}
	if err := e.AcquireTaskSlot(); err != nil {
		t.Fatal("second task should succeed")
	}
	if err := e.AcquireTaskSlot(); err != ErrTaskQueueLimit {
		t.Errorf("third task should fail: %v", err)
	}

	e.ReleaseTaskSlot()
	if err := e.AcquireTaskSlot(); err != nil {
		t.Errorf("after release, task should succeed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — document mode
// ---------------------------------------------------------------------------

func TestScriptEnforcer_AllowScript_Full(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{Mode: DocumentModeFull})
	if err := e.AllowScript(""); err != nil {
		t.Errorf("inline script should be allowed: %v", err)
	}
	if err := e.AllowScript("https://example.com/app.js"); err != nil {
		t.Errorf("remote script should be allowed in Full mode: %v", err)
	}
}

func TestScriptEnforcer_AllowScript_InlineOnly(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{Mode: DocumentModeInlineOnly})
	if err := e.AllowScript(""); err != nil {
		t.Errorf("inline script should be allowed: %v", err)
	}
	if err := e.AllowScript("https://example.com/app.js"); err != ErrRemoteScriptBlocked {
		t.Errorf("remote script should be blocked: %v", err)
	}
}

func TestScriptEnforcer_AllowScript_NoScript(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{Mode: DocumentModeNoScript})
	if err := e.AllowScript(""); err != ErrRemoteScriptBlocked {
		t.Errorf("all scripts should be blocked: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — API permissions
// ---------------------------------------------------------------------------

func TestScriptEnforcer_CheckAPIPermission_Default(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{DefaultAPIPermission: APIAllowed})
	if err := e.CheckAPIPermission("https://example.com", "fetch"); err != nil {
		t.Errorf("default allowed should pass: %v", err)
	}
}

func TestScriptEnforcer_CheckAPIPermission_Denied(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{DefaultAPIPermission: APIDenied})
	if err := e.CheckAPIPermission("https://example.com", "fetch"); err != ErrAPIPermissionDenied {
		t.Errorf("default denied should fail: %v", err)
	}
}

func TestScriptEnforcer_CheckAPIPermission_OriginSpecific(t *testing.T) {
	policy := ScriptPolicy{
		DefaultAPIPermission: APIAllowed,
		OriginPermissions: map[string]map[string]APIPermission{
			"https://evil.com": {
				"fetch":        APIDenied,
				"localStorage": APIDenied,
			},
		},
	}
	e := NewScriptEnforcer(policy)

	// Evil origin should be denied.
	if err := e.CheckAPIPermission("https://evil.com", "fetch"); err != ErrAPIPermissionDenied {
		t.Errorf("evil.com fetch should be denied: %v", err)
	}

	// Good origin should be allowed (default).
	if err := e.CheckAPIPermission("https://good.com", "fetch"); err != nil {
		t.Errorf("good.com fetch should be allowed: %v", err)
	}

	// Evil origin, different API (not listed) — falls back to default (allowed).
	if err := e.CheckAPIPermission("https://evil.com", "console"); err != nil {
		t.Errorf("evil.com console should use default (allowed): %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — context cancellation
// ---------------------------------------------------------------------------

func TestScriptEnforcer_ContextCancelledOnAbort(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{MaxSteps: 10})
	e.Start()

	select {
	case <-e.Context().Done():
		t.Fatal("context should not be done before abort")
	default:
	}

	e.AddSteps(11) // triggers abort

	select {
	case <-e.Context().Done():
		// expected
	default:
		t.Fatal("context should be done after abort")
	}
}

func TestScriptEnforcer_Stop(t *testing.T) {
	e := NewScriptEnforcer(DefaultScriptPolicy())
	e.Stop()

	select {
	case <-e.Context().Done():
		// expected
	default:
		t.Fatal("context should be done after Stop")
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — Steps counter
// ---------------------------------------------------------------------------

func TestScriptEnforcer_StepsCounter(t *testing.T) {
	e := NewScriptEnforcer(ScriptPolicy{MaxSteps: 0})
	e.Start()

	e.AddSteps(10)
	e.AddSteps(20)
	if e.Steps() != 30 {
		t.Errorf("Steps = %d, want 30", e.Steps())
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkScriptEnforcer_AddSteps(b *testing.B) {
	e := NewScriptEnforcer(ScriptPolicy{MaxSteps: 0}) // no limit
	e.Start()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.AddSteps(1)
	}
}

func BenchmarkScriptEnforcer_CheckAPIPermission(b *testing.B) {
	e := NewScriptEnforcer(ScriptPolicy{DefaultAPIPermission: APIAllowed})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.CheckAPIPermission("https://example.com", "fetch")
	}
}

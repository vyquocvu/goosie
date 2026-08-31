package js_test

import (
	"testing"
	"time"

	js "github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// DefaultScriptPolicy
// ---------------------------------------------------------------------------

func TestDefaultScriptPolicy(t *testing.T) {
	p := js.DefaultScriptPolicy()
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
	if p.Mode != js.DocumentModeFull {
		t.Errorf("Mode = %d, want Full", p.Mode)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — step limit
// ---------------------------------------------------------------------------

func TestScriptEnforcer_StepLimit(t *testing.T) {
	policy := js.ScriptPolicy{MaxSteps: 100}
	e := js.NewScriptEnforcer(policy)
	e.Start()

	if err := e.AddSteps(50); err != nil {
		t.Errorf("50 steps should be within limit: %v", err)
	}
	if err := e.AddSteps(51); err != js.ErrScriptStepLimit {
		t.Errorf("101 steps should trigger js.ErrScriptStepLimit, got %v", err)
	}
	if !e.IsAborted() {
		t.Error("enforcer should be aborted")
	}
	if e.AbortError() != js.ErrScriptStepLimit {
		t.Errorf("AbortError = %v, want js.ErrScriptStepLimit", e.AbortError())
	}
}

func TestScriptEnforcer_NoStepLimit(t *testing.T) {
	policy := js.ScriptPolicy{MaxSteps: 0} // no limit
	e := js.NewScriptEnforcer(policy)
	e.Start()

	if err := e.AddSteps(1_000_000); err != nil {
		t.Errorf("no step limit should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — time limit
// ---------------------------------------------------------------------------

func TestScriptEnforcer_TimeLimit(t *testing.T) {
	policy := js.ScriptPolicy{MaxSteps: 0, MaxExecutionTime: time.Millisecond}
	e := js.NewScriptEnforcer(policy)
	e.Start()

	time.Sleep(5 * time.Millisecond)
	if err := e.AddSteps(1); err != js.ErrScriptTimeout {
		t.Errorf("should trigger js.ErrScriptTimeout, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — timer limits
// ---------------------------------------------------------------------------

func TestScriptEnforcer_TimerLimit(t *testing.T) {
	policy := js.ScriptPolicy{MaxTimers: 2}
	e := js.NewScriptEnforcer(policy)

	if err := e.AcquireTimer(); err != nil {
		t.Fatal("first timer should succeed")
	}
	if err := e.AcquireTimer(); err != nil {
		t.Fatal("second timer should succeed")
	}
	if err := e.AcquireTimer(); err != js.ErrTimerLimit {
		t.Errorf("third timer should fail: %v", err)
	}

	e.ReleaseTimer()
	if err := e.AcquireTimer(); err != nil {
		t.Errorf("after release, timer should succeed: %v", err)
	}
}

func TestScriptEnforcer_ActiveTimers(t *testing.T) {
	policy := js.ScriptPolicy{MaxTimers: 10}
	e := js.NewScriptEnforcer(policy)

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
	policy := js.ScriptPolicy{MaxTaskQueueSize: 2}
	e := js.NewScriptEnforcer(policy)

	if err := e.AcquireTaskSlot(); err != nil {
		t.Fatal("first task should succeed")
	}
	if err := e.AcquireTaskSlot(); err != nil {
		t.Fatal("second task should succeed")
	}
	if err := e.AcquireTaskSlot(); err != js.ErrTaskQueueLimit {
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
	e := js.NewScriptEnforcer(js.ScriptPolicy{Mode: js.DocumentModeFull})
	if err := e.AllowScript(""); err != nil {
		t.Errorf("inline script should be allowed: %v", err)
	}
	if err := e.AllowScript("https://example.com/app.js"); err != nil {
		t.Errorf("remote script should be allowed in Full mode: %v", err)
	}
}

func TestScriptEnforcer_AllowScript_InlineOnly(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{Mode: js.DocumentModeInlineOnly})
	if err := e.AllowScript(""); err != nil {
		t.Errorf("inline script should be allowed: %v", err)
	}
	if err := e.AllowScript("https://example.com/app.js"); err != js.ErrRemoteScriptBlocked {
		t.Errorf("remote script should be blocked: %v", err)
	}
}

func TestScriptEnforcer_AllowScript_NoScript(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{Mode: js.DocumentModeNoScript})
	if err := e.AllowScript(""); err != js.ErrRemoteScriptBlocked {
		t.Errorf("all scripts should be blocked: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — API permissions
// ---------------------------------------------------------------------------

func TestScriptEnforcer_CheckAPIPermission_Default(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIAllowed})
	if err := e.CheckAPIPermission("https://example.com", "fetch"); err != nil {
		t.Errorf("default allowed should pass: %v", err)
	}
}

func TestScriptEnforcer_CheckAPIPermission_Denied(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIDenied})
	if err := e.CheckAPIPermission("https://example.com", "fetch"); err != js.ErrAPIPermissionDenied {
		t.Errorf("default denied should fail: %v", err)
	}
}

func TestScriptEnforcer_CheckAPIPermission_WildcardOrigin(t *testing.T) {
	policy := js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*": {
				"network": js.APIAllowed,
				"storage": js.APIAllowed,
			},
		},
	}
	e := js.NewScriptEnforcer(policy)

	// Wildcard-allowed APIs should pass for any origin.
	if err := e.CheckAPIPermission("https://example.com", "network"); err != nil {
		t.Errorf("network should be allowed via wildcard: %v", err)
	}
	if err := e.CheckAPIPermission("file:///page.html", "storage"); err != nil {
		t.Errorf("storage should be allowed via wildcard: %v", err)
	}

	// API not in wildcard should fall back to default (denied).
	if err := e.CheckAPIPermission("https://example.com", "geolocation"); err != js.ErrAPIPermissionDenied {
		t.Errorf("geolocation should be denied: %v", err)
	}

	// Exact-origin match should override wildcard.
	policy.OriginPermissions["https://example.com"] = map[string]js.APIPermission{"network": js.APIDenied}
	if err := e.CheckAPIPermission("https://example.com", "network"); err != js.ErrAPIPermissionDenied {
		t.Errorf("exact origin should override wildcard: %v", err)
	}
}

func TestScriptEnforcer_CheckAPIPermission_OriginSpecific(t *testing.T) {
	policy := js.ScriptPolicy{
		DefaultAPIPermission: js.APIAllowed,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"https://evil.com": {
				"fetch":        js.APIDenied,
				"localStorage": js.APIDenied,
			},
		},
	}
	e := js.NewScriptEnforcer(policy)

	// Evil origin should be denied.
	if err := e.CheckAPIPermission("https://evil.com", "fetch"); err != js.ErrAPIPermissionDenied {
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
	e := js.NewScriptEnforcer(js.ScriptPolicy{MaxSteps: 10})
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
	e := js.NewScriptEnforcer(js.DefaultScriptPolicy())
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
	e := js.NewScriptEnforcer(js.ScriptPolicy{MaxSteps: 0})
	e.Start()

	e.AddSteps(10)
	e.AddSteps(20)
	if e.Steps() != 30 {
		t.Errorf("Steps = %d, want 30", e.Steps())
	}
}

// ---------------------------------------------------------------------------
// PermissionDecisions — audit trail
// ---------------------------------------------------------------------------

func TestPermissionDecisions_RecordsAllowed(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIAllowed})
	if err := e.CheckAPIPermission("https://example.com", "fetch"); err != nil {
		t.Fatal(err)
	}
	decs := e.PermissionDecisions()
	if len(decs) != 1 {
		t.Fatalf("got %d decisions, want 1", len(decs))
	}
	if decs[0].Capability != "fetch" {
		t.Errorf("capability = %q, want fetch", decs[0].Capability)
	}
	if !decs[0].Allowed {
		t.Error("decision should be Allowed")
	}
	if decs[0].MatchRule != "default" {
		t.Errorf("matchRule = %q, want default", decs[0].MatchRule)
	}
}

func TestPermissionDecisions_RecordsDenied(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIDenied})
	_ = e.CheckAPIPermission("https://example.com", "fetch")
	decs := e.PermissionDecisions()
	if len(decs) != 1 {
		t.Fatalf("got %d decisions, want 1", len(decs))
	}
	if decs[0].Allowed {
		t.Error("decision should be Denied")
	}
	if decs[0].MatchRule != "default" {
		t.Errorf("matchRule = %q, want default", decs[0].MatchRule)
	}
}

func TestPermissionDecisions_MatchRule(t *testing.T) {
	policy := js.ScriptPolicy{
		DefaultAPIPermission: js.APIDenied,
		OriginPermissions: map[string]map[string]js.APIPermission{
			"*":                        {"storage": js.APIAllowed},
			"https://exact.com":        {"storage": js.APIAllowed},
			"https://exact-denied.com": {"network": js.APIDenied},
		},
	}
	e := js.NewScriptEnforcer(policy)

	_ = e.CheckAPIPermission("https://exact.com", "storage")        // exact (allowed)
	_ = e.CheckAPIPermission("https://wild.com", "storage")         // wildcard (allowed)
	_ = e.CheckAPIPermission("https://other.com", "network")        // wildcard not listed → default (denied)
	_ = e.CheckAPIPermission("https://exact-denied.com", "network") // exact (denied)

	decs := e.PermissionDecisions()
	if len(decs) != 4 {
		t.Fatalf("got %d decisions, want 4", len(decs))
	}

	tests := []struct {
		allowed bool
		rule    string
	}{
		{true, "exact"},
		{true, "wildcard"},
		{false, "default"},
		{false, "exact"},
	}
	for i, tt := range tests {
		if decs[i].Allowed != tt.allowed {
			t.Errorf("decision %d: Allowed = %v, want %v", i, decs[i].Allowed, tt.allowed)
		}
		if decs[i].MatchRule != tt.rule {
			t.Errorf("decision %d: MatchRule = %q, want %q", i, decs[i].MatchRule, tt.rule)
		}
	}
}

func TestPermissionDecisions_ReturnsAndClearsBuffer(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIAllowed})
	_ = e.CheckAPIPermission("https://example.com", "fetch")

	first := e.PermissionDecisions()
	if len(first) != 1 {
		t.Fatalf("first call: got %d, want 1", len(first))
	}

	second := e.PermissionDecisions()
	if len(second) != 0 {
		t.Fatalf("second call (after clear): got %d, want 0", len(second))
	}
}

func TestPermissionDecisions_BoundedBuffer(t *testing.T) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIAllowed})
	for i := 0; i < js.MaxPermissionDecisions+10; i++ {
		e.CheckAPIPermission("test", "allow")
	}
	decs := e.PermissionDecisions()
	if len(decs) > js.MaxPermissionDecisions {
		t.Fatalf("buffer exceeded: got %d, max %d", len(decs), js.MaxPermissionDecisions)
	}
	if len(decs) != js.MaxPermissionDecisions {
		t.Fatalf("expected exactly %d decisions (overflow trim), got %d", js.MaxPermissionDecisions, len(decs))
	}
}

func BenchmarkScriptEnforcer_AddSteps(b *testing.B) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{MaxSteps: 0}) // no limit
	e.Start()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.AddSteps(1)
	}
}

func BenchmarkScriptEnforcer_CheckAPIPermission(b *testing.B) {
	e := js.NewScriptEnforcer(js.ScriptPolicy{DefaultAPIPermission: js.APIAllowed})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.CheckAPIPermission("https://example.com", "fetch")
	}
}

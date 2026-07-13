package js

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrScriptTimeout is returned when a script exceeds its execution budget.
	ErrScriptTimeout = errors.New("js: script execution timeout")

	// ErrScriptStepLimit is returned when a script exceeds its step budget.
	ErrScriptStepLimit = errors.New("js: script step limit exceeded")

	// ErrTimerLimit is returned when the timer count exceeds the policy limit.
	ErrTimerLimit = errors.New("js: timer limit exceeded")

	// ErrTaskQueueLimit is returned when the task queue exceeds the policy limit.
	ErrTaskQueueLimit = errors.New("js: task queue limit exceeded")

	// ErrRemoteScriptBlocked is returned when remote scripts are disabled.
	ErrRemoteScriptBlocked = errors.New("js: remote scripts disabled")

	// ErrAPIPermissionDenied is returned when an API call is not permitted.
	ErrAPIPermissionDenied = errors.New("js: API permission denied")
)

// ---------------------------------------------------------------------------
// DocumentMode — controls script loading behavior
// ---------------------------------------------------------------------------

// DocumentMode controls how scripts are loaded and executed.
type DocumentMode uint8

const (
	// DocumentModeFull allows all scripts (inline and remote).
	DocumentModeFull DocumentMode = iota
	// DocumentModeInlineOnly allows only inline scripts, blocking remote.
	DocumentModeInlineOnly
	// DocumentModeNoScript disables all script execution.
	DocumentModeNoScript
)

// ---------------------------------------------------------------------------
// Capability names — used with ScriptPolicy.OriginPermissions and
// CheckAPIPermission. Each constant identifies a browser API capability
// that can be individually allowed or denied.
// ---------------------------------------------------------------------------

const (
	CapabilityNetwork       = "network"       // fetch(), XMLHttpRequest, WebSocket
	CapabilityStorage       = "storage"       // localStorage, sessionStorage
	CapabilityNavigation    = "navigation"    // window.open, location navigation
	CapabilityClipboard     = "clipboard"     // navigator.clipboard read/write
	CapabilityFile          = "file"          // File System Access API
	CapabilityNotifications = "notifications" // Notification API
	CapabilityGeolocation   = "geolocation"   // Geolocation API
)

// ---------------------------------------------------------------------------
// APIPermission — per-origin API access control
// ---------------------------------------------------------------------------

// APIPermission defines the access level for a specific API.
type APIPermission uint8

const (
	// APIAllowed allows the API call.
	APIAllowed APIPermission = iota
	// APIDenied denies the API call.
	APIDenied
	// APIPrompt would prompt the user (treated as denied for now).
	APIPrompt
)

// ---------------------------------------------------------------------------
// ScriptPolicy — configurable limits and controls
// ---------------------------------------------------------------------------

// ScriptPolicy configures execution limits and security controls.
type ScriptPolicy struct {
	// MaxSteps is the maximum number of VM steps before interruption.
	// Zero means no limit.
	MaxSteps uint64

	// MaxExecutionTime is the maximum wall-clock time for script execution.
	// Zero means no limit.
	MaxExecutionTime time.Duration

	// MaxTimers is the maximum number of active timers.
	MaxTimers int

	// MaxTaskQueueSize is the maximum number of pending tasks.
	MaxTaskQueueSize int

	// Mode controls script loading behavior.
	Mode DocumentMode

	// DefaultAPIPermission is the default permission for APIs not
	// explicitly listed in OriginPermissions.
	DefaultAPIPermission APIPermission

	// OriginPermissions maps origin → API name → permission.
	OriginPermissions map[string]map[string]APIPermission
}

// DefaultScriptPolicy returns permissive defaults (allows everything).
func DefaultScriptPolicy() ScriptPolicy {
	return ScriptPolicy{
		MaxSteps:             1_000_000,
		MaxExecutionTime:     10 * time.Second,
		MaxTimers:            128,
		MaxTaskQueueSize:     256,
		Mode:                 DocumentModeFull,
		DefaultAPIPermission: APIAllowed,
	}
}

// DefaultSecurePolicy returns secure defaults — it denies capabilities that
// the engine does not yet implement or that should require explicit user
// consent, while allowing the core browser APIs (network, storage,
// navigation) across all origins.
func DefaultSecurePolicy() ScriptPolicy {
	return ScriptPolicy{
		MaxSteps:             1_000_000,
		MaxExecutionTime:     10 * time.Second,
		MaxTimers:            128,
		MaxTaskQueueSize:     256,
		Mode:                 DocumentModeFull,
		DefaultAPIPermission: APIDenied,
		OriginPermissions: map[string]map[string]APIPermission{
			"*": {
				CapabilityNetwork:    APIAllowed,
				CapabilityStorage:    APIAllowed,
				CapabilityNavigation: APIAllowed,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// ScriptEnforcer — runtime enforcement of ScriptPolicy
// ---------------------------------------------------------------------------

// PermissionDecision records a single API capability check result.
type PermissionDecision struct {
	Origin     string    `json:"origin"`
	Capability string    `json:"capability"`
	Allowed    bool      `json:"allowed"`
	MatchRule  string    `json:"matchRule"` // "exact", "wildcard", or "default"
	Timestamp  time.Time `json:"timestamp"`
}

// maxPermissionDecisions limits the audit trail size to prevent unbounded
// memory growth.
const maxPermissionDecisions = 1024

// ScriptEnforcer enforces a ScriptPolicy at runtime. It tracks step count,
// execution time, and provides interruption signals.
type ScriptEnforcer struct {
	mu       sync.Mutex
	policy   ScriptPolicy
	ctx      context.Context
	cancel   context.CancelFunc
	steps    atomic.Uint64
	started  time.Time
	aborted  atomic.Bool
	abortErr error

	// Timer tracking.
	activeTimers atomic.Int64

	// Task queue tracking.
	pendingTasks atomic.Int64

	// Audit trail of permission decisions (bounded ring buffer).
	decisions []PermissionDecision
}

// NewScriptEnforcer creates an enforcer with the given policy.
func NewScriptEnforcer(policy ScriptPolicy) *ScriptEnforcer {
	ctx, cancel := context.WithCancel(context.Background())
	return &ScriptEnforcer{
		policy: policy,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins tracking execution time.
func (e *ScriptEnforcer) Start() {
	e.mu.Lock()
	e.started = time.Now()
	e.mu.Unlock()
	e.steps.Store(0)
	e.aborted.Store(false)
	e.abortErr = nil
}

// AddSteps records step execution and checks limits.
// Returns an error if a limit is exceeded.
func (e *ScriptEnforcer) AddSteps(n uint64) error {
	if e.aborted.Load() {
		return e.abortErr
	}

	total := e.steps.Add(n)

	// Check step limit.
	if e.policy.MaxSteps > 0 && total > e.policy.MaxSteps {
		e.abort(ErrScriptStepLimit)
		return ErrScriptStepLimit
	}

	// Check time limit.
	if e.policy.MaxExecutionTime > 0 {
		e.mu.Lock()
		elapsed := time.Since(e.started)
		e.mu.Unlock()
		if elapsed > e.policy.MaxExecutionTime {
			e.abort(ErrScriptTimeout)
			return ErrScriptTimeout
		}
	}

	return nil
}

// Steps returns the current step count.
func (e *ScriptEnforcer) Steps() uint64 {
	return e.steps.Load()
}

// abort signals the enforcer to stop.
func (e *ScriptEnforcer) abort(err error) {
	e.aborted.Store(true)
	e.abortErr = err
	e.cancel()
}

// IsAborted reports whether execution has been interrupted.
func (e *ScriptEnforcer) IsAborted() bool {
	return e.aborted.Load()
}

// AbortError returns the error that caused the abort, or nil.
func (e *ScriptEnforcer) AbortError() error {
	return e.abortErr
}

// Context returns the enforcer's context (cancelled on abort).
func (e *ScriptEnforcer) Context() context.Context {
	return e.ctx
}

// Stop halts the enforcer and releases resources.
func (e *ScriptEnforcer) Stop() {
	e.cancel()
}

// ---------------------------------------------------------------------------
// Timer and task queue limit enforcement
// ---------------------------------------------------------------------------

// AcquireTimer checks if a new timer can be created. Returns error if limit reached.
func (e *ScriptEnforcer) AcquireTimer() error {
	if e.policy.MaxTimers > 0 && e.activeTimers.Load() >= int64(e.policy.MaxTimers) {
		return ErrTimerLimit
	}
	e.activeTimers.Add(1)
	return nil
}

// ReleaseTimer records that a timer has been cleared or fired.
func (e *ScriptEnforcer) ReleaseTimer() {
	e.activeTimers.Add(-1)
}

// ActiveTimers returns the number of active timers.
func (e *ScriptEnforcer) ActiveTimers() int64 {
	return e.activeTimers.Load()
}

// AcquireTaskSlot checks if a new task can be enqueued. Returns error if limit reached.
func (e *ScriptEnforcer) AcquireTaskSlot() error {
	if e.policy.MaxTaskQueueSize > 0 && e.pendingTasks.Load() >= int64(e.policy.MaxTaskQueueSize) {
		return ErrTaskQueueLimit
	}
	e.pendingTasks.Add(1)
	return nil
}

// ReleaseTaskSlot records that a task has been executed.
func (e *ScriptEnforcer) ReleaseTaskSlot() {
	e.pendingTasks.Add(-1)
}

// PendingTasks returns the number of pending tasks.
func (e *ScriptEnforcer) PendingTasks() int64 {
	return e.pendingTasks.Load()
}

// ---------------------------------------------------------------------------
// Script mode and API permission checks
// ---------------------------------------------------------------------------

// AllowScript reports whether a script with the given source should execute.
// Empty src means inline script.
func (e *ScriptEnforcer) AllowScript(src string) error {
	switch e.policy.Mode {
	case DocumentModeNoScript:
		return ErrRemoteScriptBlocked
	case DocumentModeInlineOnly:
		if src != "" {
			return ErrRemoteScriptBlocked
		}
	}
	return nil
}

// CheckAPIPermission checks if the given origin can access the named API.
func (e *ScriptEnforcer) CheckAPIPermission(origin, api string) error {
	allowed, rule := e.checkPermission(origin, api)
	r := PermissionDecision{
		Origin:     origin,
		Capability: api,
		Allowed:    allowed,
		MatchRule:  rule,
		Timestamp:  time.Now(),
	}

	e.mu.Lock()
	if len(e.decisions) >= maxPermissionDecisions {
		e.decisions = e.decisions[1:]
	}
	e.decisions = append(e.decisions, r)
	e.mu.Unlock()

	if !allowed {
		return ErrAPIPermissionDenied
	}
	return nil
}

// checkPermission returns whether the API is allowed and which rule matched.
func (e *ScriptEnforcer) checkPermission(origin, api string) (allowed bool, matchRule string) {
	// Check exact origin match first (most specific).
	if perms, ok := e.policy.OriginPermissions[origin]; ok {
		if perm, ok := perms[api]; ok {
			return perm == APIAllowed, "exact"
		}
	}

	// Check wildcard origin ("*") as a fallback for global defaults.
	if perms, ok := e.policy.OriginPermissions["*"]; ok {
		if perm, ok := perms[api]; ok {
			return perm == APIAllowed, "wildcard"
		}
	}

	// Fall back to default.
	return e.policy.DefaultAPIPermission == APIAllowed, "default"
}

// PermissionDecisions returns all recorded permission decisions and clears
// the internal buffer.
func (e *ScriptEnforcer) PermissionDecisions() []PermissionDecision {
	e.mu.Lock()
	out := e.decisions
	e.decisions = nil
	e.mu.Unlock()
	return out
}

// Policy returns the current script policy.
func (e *ScriptEnforcer) Policy() ScriptPolicy {
	return e.policy
}

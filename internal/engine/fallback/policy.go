// Package fallback provides the compatibility fallback decision layer.
//
// M12.1: A Policy evaluates fallback triggers (user request, detected
// unsupported features, site allowlist, failure threshold) and produces
// a ShouldFallback decision. The package is engine-side only — it has
// no Fyne, UI, or platform dependencies.
package fallback

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

// Decision describes why fallback was triggered.
type Decision int

const (
	DecisionNone               Decision = iota
	DecisionUserRequested               // user explicitly requested compatibility mode
	DecisionUnsupportedFeature          // parser detected an unsupported element or API
	DecisionAllowlist                   // origin is on the compatibility allowlist
	DecisionFailureThreshold            // consecutive render/script failures exceeded threshold
)

func (d Decision) String() string {
	switch d {
	case DecisionUserRequested:
		return "user-requested"
	case DecisionUnsupportedFeature:
		return "unsupported-feature"
	case DecisionAllowlist:
		return "allowlist"
	case DecisionFailureThreshold:
		return "failure-threshold"
	default:
		return fmt.Sprintf("Decision(%d)", int(d))
	}
}

// Policy controls when the engine should fall back to a compatibility backend.
// It is safe for concurrent use.
type Policy struct {
	mu sync.Mutex

	UserRequested    bool     // user explicitly requested compatibility mode (M12.1 item 5)
	Allowlist        []string // origin hosts that always use compatibility (M12.1 item 6)
	FailureThreshold int      // fallback after N consecutive failures (M12.1 item 7)

	detected            []dom.UnsupportedFeature
	consecutiveFailures int
	lastDecision        Decision
}

// NewPolicy creates a fallback policy with the given configuration.
func NewPolicy(userRequested bool, allowlist []string, failureThreshold int) *Policy {
	if allowlist == nil {
		allowlist = []string{}
	}
	return &Policy{
		UserRequested:    userRequested,
		Allowlist:        allowlist,
		FailureThreshold: failureThreshold,
	}
}

// Record registers a detected unsupported feature from the parser.
func (p *Policy) Record(feature dom.UnsupportedFeature) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.detected = append(p.detected, feature)
}

// RecordFailure increments the consecutive failure counter.
func (p *Policy) RecordFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures++
}

// ResetFailure resets the consecutive failure counter on a successful render.
func (p *Policy) ResetFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures = 0
}

// ShouldFallback returns true when the policy determines fallback is needed.
// Triggers are evaluated in order: user request, allowlist, detected features,
// failure threshold. The first match wins.
func (p *Policy) ShouldFallback(originHost string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.UserRequested {
		p.lastDecision = DecisionUserRequested
		return true
	}
	for _, allowed := range p.Allowlist {
		if strings.EqualFold(originHost, allowed) {
			p.lastDecision = DecisionAllowlist
			return true
		}
	}
	if len(p.detected) > 0 {
		p.lastDecision = DecisionUnsupportedFeature
		return true
	}
	if p.FailureThreshold > 0 && p.consecutiveFailures >= p.FailureThreshold {
		p.lastDecision = DecisionFailureThreshold
		return true
	}
	p.lastDecision = DecisionNone
	return false
}

// LastDecision returns the reason for the most recent ShouldFallback call.
func (p *Policy) LastDecision() Decision {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastDecision
}

// Reset clears detected features and failure counters for a new navigation.
func (p *Policy) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.detected = p.detected[:0]
	p.consecutiveFailures = 0
	p.lastDecision = DecisionNone
}

// Detected returns a copy of the recorded unsupported features for diagnostics.
func (p *Policy) Detected() []dom.UnsupportedFeature {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]dom.UnsupportedFeature, len(p.detected))
	copy(result, p.detected)
	return result
}

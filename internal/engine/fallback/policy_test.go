package fallback

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// TestPolicyNoFallbackByDefault
// ---------------------------------------------------------------------------

func TestPolicyNoFallbackByDefault(t *testing.T) {
	p := NewPolicy(false, nil, 0)
	assert.False(t, p.ShouldFallback("example.com"), "empty policy should not trigger fallback")
	assert.Equal(t, DecisionNone, p.LastDecision())
}

// ---------------------------------------------------------------------------
// TestPolicyUserRequested
// ---------------------------------------------------------------------------

func TestPolicyUserRequested(t *testing.T) {
	p := NewPolicy(true, nil, 0)
	assert.True(t, p.ShouldFallback("example.com"), "user request should trigger fallback")
	assert.Equal(t, DecisionUserRequested, p.LastDecision())
}

// ---------------------------------------------------------------------------
// TestPolicyDetectedFeature
// ---------------------------------------------------------------------------

func TestPolicyDetectedFeature(t *testing.T) {
	p := NewPolicy(false, nil, 0)
	p.Record(dom.UnsupportedFeature{Kind: dom.FeatureCanvas})
	assert.True(t, p.ShouldFallback("example.com"), "detected feature should trigger fallback")
	assert.Equal(t, DecisionUnsupportedFeature, p.LastDecision())
}

// ---------------------------------------------------------------------------
// TestPolicyAllowlist
// ---------------------------------------------------------------------------

func TestPolicyAllowlist(t *testing.T) {
	p := NewPolicy(false, []string{"heavy-site.example.com", "app.example.com"}, 0)
	assert.True(t, p.ShouldFallback("heavy-site.example.com"), "allowlisted origin should trigger fallback")
	assert.Equal(t, DecisionAllowlist, p.LastDecision())
}

// ---------------------------------------------------------------------------
// TestPolicyAllowlistCaseInsensitive
// ---------------------------------------------------------------------------

func TestPolicyAllowlistCaseInsensitive(t *testing.T) {
	p := NewPolicy(false, []string{"Heavy-Site.Example.COM"}, 0)
	assert.True(t, p.ShouldFallback("heavy-site.example.com"), "allowlist should be case-insensitive")
}

// ---------------------------------------------------------------------------
// TestPolicyAllowlistNoMatch
// ---------------------------------------------------------------------------

func TestPolicyAllowlistNoMatch(t *testing.T) {
	p := NewPolicy(false, []string{"allowed.example.com"}, 0)
	assert.False(t, p.ShouldFallback("other.example.com"), "non-allowlisted origin should not trigger")
}

// ---------------------------------------------------------------------------
// TestPolicyFailureThresholdExceeded
// ---------------------------------------------------------------------------

func TestPolicyFailureThresholdExceeded(t *testing.T) {
	p := NewPolicy(false, nil, 3)
	p.RecordFailure()
	p.RecordFailure()
	p.RecordFailure()
	assert.True(t, p.ShouldFallback("example.com"), "3 failures should meet threshold of 3")
	assert.Equal(t, DecisionFailureThreshold, p.LastDecision())
}

// ---------------------------------------------------------------------------
// TestPolicyFailureThresholdNotReached
// ---------------------------------------------------------------------------

func TestPolicyFailureThresholdNotReached(t *testing.T) {
	p := NewPolicy(false, nil, 5)
	p.RecordFailure()
	p.RecordFailure()
	assert.False(t, p.ShouldFallback("example.com"), "2 failures should not meet threshold of 5")
}

// ---------------------------------------------------------------------------
// TestPolicyResetClearsStateAndFeatures
// ---------------------------------------------------------------------------

func TestPolicyResetClearsState(t *testing.T) {
	p := NewPolicy(false, nil, 2)
	p.Record(dom.UnsupportedFeature{Kind: dom.FeatureVideo})
	p.RecordFailure()
	p.RecordFailure()
	assert.True(t, p.ShouldFallback("example.com"), "should trigger before reset")

	p.Reset()
	assert.False(t, p.ShouldFallback("example.com"), "should not trigger after reset")
	assert.Equal(t, DecisionNone, p.LastDecision())
	assert.Empty(t, p.Detected(), "detected features should be empty after reset")
}

// ---------------------------------------------------------------------------
// TestPolicyResetFailure
// ---------------------------------------------------------------------------

func TestPolicyResetFailure(t *testing.T) {
	p := NewPolicy(false, nil, 2)
	p.RecordFailure()
	p.ResetFailure()
	p.RecordFailure()
	assert.False(t, p.ShouldFallback("example.com"), "reset should clear failure count")
}

// ---------------------------------------------------------------------------
// TestPolicyMultipleFeatures
// ---------------------------------------------------------------------------

func TestPolicyMultipleFeatures(t *testing.T) {
	p := NewPolicy(false, nil, 0)
	p.Record(dom.UnsupportedFeature{Kind: dom.FeatureCanvas})
	p.Record(dom.UnsupportedFeature{Kind: dom.FeatureVideo})
	p.Record(dom.UnsupportedFeature{Kind: dom.FeatureESModule})
	assert.True(t, p.ShouldFallback("example.com"))

	detected := p.Detected()
	assert.Len(t, detected, 3, "should have recorded 3 features")
}

// ---------------------------------------------------------------------------
// TestPolicyConcurrent
// ---------------------------------------------------------------------------

func TestPolicyConcurrent(t *testing.T) {
	p := NewPolicy(false, nil, 0)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Record(dom.UnsupportedFeature{Kind: dom.FeatureCanvas})
			p.ShouldFallback("example.com")
			p.LastDecision()
			p.Detected()
		}()
	}
	wg.Wait()

	detected := p.Detected()
	assert.Len(t, detected, 20, "all 20 concurrent records should be stored")
}

// ---------------------------------------------------------------------------
// TestPolicyUserRequestedOverridesAllowlist
// ---------------------------------------------------------------------------

func TestPolicyUserRequestedTakesPriority(t *testing.T) {
	p := NewPolicy(true, []string{"trusted.example.com"}, 0)
	// UserRequested=true should trigger even for allowlisted origins.
	assert.True(t, p.ShouldFallback("trusted.example.com"))
	assert.Equal(t, DecisionUserRequested, p.LastDecision(),
		"user request should take priority over allowlist")
}

// ---------------------------------------------------------------------------
// TestPolicyAllowlistWinsOverDetected
// ---------------------------------------------------------------------------

func TestPolicyAllowlistWinsOverDetected(t *testing.T) {
	p := NewPolicy(false, []string{"trusted.example.com"}, 0)
	p.Record(dom.UnsupportedFeature{Kind: dom.FeatureIframe})
	// Allowlist is checked before detected features — both trigger fallback
	// but the decision reason is allowlist.
	assert.True(t, p.ShouldFallback("trusted.example.com"))
	assert.Equal(t, DecisionAllowlist, p.LastDecision(),
		"allowlist should take priority over detected features")
}

// ---------------------------------------------------------------------------
// TestPolicyDecisionString
// ---------------------------------------------------------------------------

func TestPolicyDecisionString(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{DecisionNone, "Decision(0)"},
		{DecisionUserRequested, "user-requested"},
		{DecisionUnsupportedFeature, "unsupported-feature"},
		{DecisionAllowlist, "allowlist"},
		{DecisionFailureThreshold, "failure-threshold"},
		{Decision(99), "Decision(99)"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.d.String(), "String() for Decision(%d)", tc.d)
	}
}

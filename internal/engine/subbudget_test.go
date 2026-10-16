package engine

import (
	"testing"
)

// TestSubBudgetAccountsOnce pins the shared total: charges accumulate and
// the first charge past the cap fails without spending.
func TestSubBudgetAccountsOnce(t *testing.T) {
	b := &subBudget{}
	if !b.take(maxSubresourceBytes - 1) {
		t.Fatal("near-cap charge refused")
	}
	if b.take(2) {
		t.Error("over-cap charge accepted")
	}
	if b.used != maxSubresourceBytes-1 {
		t.Errorf("used = %d after refused charge, want %d (no spend on failure)", b.used, maxSubresourceBytes-1)
	}
	var nilBudget *subBudget
	if !nilBudget.take(1 << 30) {
		t.Error("nil budget must admit (paths without a session)")
	}
}

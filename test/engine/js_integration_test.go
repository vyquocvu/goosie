package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

// TestJSMutationSinkNotNil verifies that JSMutationSink always returns a
// non-nil function, regardless of whether an invalidation was supplied.
func TestJSMutationSinkNotNil(t *testing.T) {
	sess, err := engine.NewSession("<html><body></body></html>", nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sink := sess.JSMutationSink()
	if sink == nil {
		t.Fatal("JSMutationSink returned nil")
	}
	// Should not panic when called without an invalidation (no-op).
	sink()
}

// TestWithInvalidationOption verifies that WithInvalidation wires the
// invalidation so that JSMutationSink records DOMMutation into it.
func TestWithInvalidationOption(t *testing.T) {
	var inv engine.Invalidation

	sess, err := engine.NewSession("<html><body></body></html>", nil, 800,
		engine.WithInvalidation(&inv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sink := sess.JSMutationSink()
	sink() // simulate a JS mutation

	plan := inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons&engine.DOMMutation == 0 {
		t.Error("WithInvalidation did not cause DOMMutation to be recorded")
	}

	// After reset, the invalidation should be clean.
	inv.Reset()
	plan = inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons != 0 {
		t.Errorf("after Reset, reasons = %v, want 0", plan.Reasons)
	}
}

// TestJSMutationSinkWithoutInvalidationIsNoop verifies that calling the
// mutation sink without an invalidation does not panic and does not record
// any reasons.
func TestJSMutationSinkWithoutInvalidationIsNoop(t *testing.T) {
	sess, err := engine.NewSession("<html><body></body></html>", nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sink := sess.JSMutationSink()
	// Should not panic.
	sink()
	sink()
	sink()
}

// TestJSMutationSinkRecordsDOMMutationReason verifies that each call to the
// mutation sink records exactly the DOMMutation reason.
func TestJSMutationSinkRecordsDOMMutationReason(t *testing.T) {
	var inv engine.Invalidation

	sess, err := engine.NewSession("<html><body></body></html>", nil, 800,
		engine.WithInvalidation(&inv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sink := sess.JSMutationSink()

	// Call the sink multiple times; DOMMutation should be recorded.
	sink()
	sink()
	sink()

	plan := inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons&engine.DOMMutation == 0 {
		t.Error("expected DOMMutation in reasons")
	}
	// No other reasons should be present.
	if plan.Reasons != engine.DOMMutation {
		t.Errorf("reasons = %v, want only DOMMutation (%v)", plan.Reasons, engine.DOMMutation)
	}
}

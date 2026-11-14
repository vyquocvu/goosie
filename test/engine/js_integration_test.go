package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

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
	sink()
}

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
	if sess.HasPendingDOMMutation() {
		t.Fatal("new invalidation unexpectedly contains a DOM mutation")
	}
	sink()
	if !sess.HasPendingDOMMutation() {
		t.Fatal("pending DOM mutation was not observable after the mutation sink")
	}

	plan := inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons&engine.DOMMutation == 0 {
		t.Error("WithInvalidation did not cause DOMMutation to be recorded")
	}

	inv.Reset()
	if sess.HasPendingDOMMutation() {
		t.Fatal("Reset left a pending DOM mutation")
	}
	plan = inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons != 0 {
		t.Errorf("after Reset, reasons = %v, want 0", plan.Reasons)
	}
}

func TestJSMutationSinkWithoutInvalidationIsNoop(t *testing.T) {
	sess, err := engine.NewSession("<html><body></body></html>", nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sink := sess.JSMutationSink()
	sink()
	sink()
	sink()
}

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

	sink()
	sink()
	sink()

	plan := inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons&engine.DOMMutation == 0 {
		t.Error("expected DOMMutation in reasons")
	}
	if plan.Reasons != engine.DOMMutation {
		t.Errorf("reasons = %v, want only DOMMutation (%v)", plan.Reasons, engine.DOMMutation)
	}
}

func TestPendingDOMMutationTracksExactReasonAndReset(t *testing.T) {
	var inv engine.Invalidation
	sess, err := engine.NewSession("<html><body></body></html>", nil, 800,
		engine.WithInvalidation(&inv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	inv.Add(engine.Resize, frame.RectF{}, 0)
	if sess.HasPendingDOMMutation() {
		t.Fatal("resize was mistaken for a pending DOM mutation")
	}
	inv.Reset()
	sess.JSMutationSink()()
	if !sess.HasPendingDOMMutation() {
		t.Fatal("DOM mutation was not recorded")
	}
	inv.Reset()
	if sess.HasPendingDOMMutation() {
		t.Fatal("Reset did not clear pending DOM mutation")
	}
}

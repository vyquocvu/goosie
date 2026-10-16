package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

func TestRefreshScrollOnlyIsNoop(t *testing.T) {
	sess, err := engine.NewSession("<html><body><div>test</div></body></html>", nil, 512)
	if err != nil {
		t.Fatal(err)
	}

	// Scroll-only plan should be a no-op.
	plan := engine.Plan{
		Viewport: frame.Viewport{Offset: frame.Point{Y: 100}, Size: frame.Size{W: 512, H: 512}},
		Reasons:  engine.Scroll,
	}

	// Capture the original arena pointer to verify it's unchanged.
	origArena := sess.Arena

	done := sess.Refresh(plan, nil, 512)
	if done {
		t.Fatal("scroll-only plan should not do work")
	}
	if sess.Arena != origArena {
		t.Fatal("scroll-only plan should not rebuild arena")
	}
}

func TestRefreshFullDocRebuildsPipeline(t *testing.T) {
	sess, err := engine.NewSession("<html><body><div>test</div></body></html>", nil, 512)
	if err != nil {
		t.Fatal(err)
	}

	origArena := sess.Arena

	plan := engine.Plan{
		FullDoc: true,
		Reasons: engine.Resize,
	}

	done := sess.Refresh(plan, nil, 512)
	if !done {
		t.Fatal("full-doc plan should do work")
	}
	if sess.Arena == origArena {
		t.Fatal("full-doc plan should rebuild arena")
	}
}

func TestRefreshPartialRebuildsPipeline(t *testing.T) {
	sess, err := engine.NewSession("<html><body><div>test</div></body></html>", nil, 512)
	if err != nil {
		t.Fatal(err)
	}

	origArena := sess.Arena

	plan := engine.Plan{
		Subtree:       []uint32{1},
		LayoutObjects: 1,
		Reasons:       engine.Hover,
	}

	done := sess.Refresh(plan, nil, 512)
	if !done {
		t.Fatal("partial plan with subtrees should do work")
	}
	if sess.Arena == origArena {
		t.Fatal("partial plan should rebuild arena")
	}
}

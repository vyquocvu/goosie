package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

// TestSessionHasAnimationController verifies that a session with @keyframes
// animations creates a non-nil AnimationController and TickAnimations reports
// active while the animation is running.
func TestSessionHasAnimationController(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
@keyframes fadeIn { from { opacity: 0 } to { opacity: 1 } }
div { animation: fadeIn 2s linear }
</style>
</head><body>
<div id="box" style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	if sess.Animations == nil {
		t.Fatal("session.Animations is nil, want non-nil for document with @keyframes animation")
	}

	// TickAnimations at the start should report active.
	active := sess.TickAnimations(time.Now())
	if !active {
		t.Error("TickAnimations() = false at start, want true")
	}
}

// TestAnimationTickReason verifies that AnimationTick is a valid invalidation
// reason distinct from other reasons.
func TestAnimationTickReason(t *testing.T) {
	// AnimationTick must be a distinct value from every other reason.
	reasons := []engine.Reason{
		engine.Scroll,
		engine.Hover,
		engine.StyleChange,
		engine.ImageLoad,
		engine.DOMMutation,
		engine.Resize,
		engine.StylesheetChange,
		engine.AnimationTick,
	}
	seen := make(map[engine.Reason]bool)
	for _, r := range reasons {
		if seen[r] {
			t.Errorf("AnimationTick or another reason has a duplicate value: %d", r)
		}
		seen[r] = true
	}

	// Verify it can be used in an Invalidation batch.
	var inv engine.Invalidation
	inv.Add(engine.AnimationTick, frame.RectF{}, 0)
	plan := inv.Resolve(frame.Viewport{}, nil)
	if plan.Reasons&engine.AnimationTick == 0 {
		t.Error("AnimationTick not present in plan.Reasons after Add")
	}
}

// TestTransformInComputedStyle verifies that an element with a CSS transform
// declaration has the Transform field populated in its computed style.
func TestTransformInComputedStyle(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
div { transform: translate(10px, 20px) }
</style>
</head><body>
<div id="box" style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	el := sess.Doc.ElementByID("box")
	if el == nil {
		t.Fatal("div#box not found")
	}
	st := sess.Styles[el.ID]
	if st == nil {
		t.Fatal("div#box has no computed style")
	}

	if len(st.Transform) == 0 {
		t.Fatal("Transform is empty, want at least one transform function")
	}
	fn := st.Transform[0]
	if fn.Name != "translate" {
		t.Errorf("Transform[0].Name = %q, want %q", fn.Name, "translate")
	}
	if len(fn.Args) < 2 {
		t.Fatalf("Transform[0].Args has %d elements, want >= 2", len(fn.Args))
	}
	if fn.Args[0] != 10 {
		t.Errorf("translate X = %v, want 10", fn.Args[0])
	}
	if fn.Args[1] != 20 {
		t.Errorf("translate Y = %v, want 20", fn.Args[1])
	}
}

// TestAnimationPropertiesInComputedStyle verifies that the animation shorthand
// populates all animation fields in the computed style.
func TestAnimationPropertiesInComputedStyle(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
@keyframes fade { from { opacity: 0 } to { opacity: 1 } }
div { animation: fade 1s ease }
</style>
</head><body>
<div id="box" style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	el := sess.Doc.ElementByID("box")
	if el == nil {
		t.Fatal("div#box not found")
	}
	st := sess.Styles[el.ID]
	if st == nil {
		t.Fatal("div#box has no computed style")
	}

	if st.AnimationName != "fade" {
		t.Errorf("AnimationName = %q, want %q", st.AnimationName, "fade")
	}
	if st.AnimationDuration != time.Second {
		t.Errorf("AnimationDuration = %v, want %v", st.AnimationDuration, time.Second)
	}
	if st.AnimationTiming != css.TimingEase {
		t.Errorf("AnimationTiming = %v, want TimingEase", st.AnimationTiming)
	}
}

// TestAnimationLonghandProperties verifies that individual animation longhand
// properties are parsed correctly.
func TestAnimationLonghandProperties(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
@keyframes slide { from { transform: translateX(0) } to { transform: translateX(100px) } }
div {
	animation-name: slide;
	animation-duration: 500ms;
	animation-timing-function: linear;
	animation-delay: 200ms;
	animation-iteration-count: 3;
	animation-direction: alternate;
	animation-fill-mode: forwards;
}
</style>
</head><body>
<div id="box" style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	el := sess.Doc.ElementByID("box")
	if el == nil {
		t.Fatal("div#box not found")
	}
	st := sess.Styles[el.ID]
	if st == nil {
		t.Fatal("div#box has no computed style")
	}

	if st.AnimationName != "slide" {
		t.Errorf("AnimationName = %q, want %q", st.AnimationName, "slide")
	}
	if st.AnimationDuration != 500*time.Millisecond {
		t.Errorf("AnimationDuration = %v, want %v", st.AnimationDuration, 500*time.Millisecond)
	}
	if st.AnimationTiming != css.TimingLinear {
		t.Errorf("AnimationTiming = %v, want TimingLinear", st.AnimationTiming)
	}
	if st.AnimationDelay != 200*time.Millisecond {
		t.Errorf("AnimationDelay = %v, want %v", st.AnimationDelay, 200*time.Millisecond)
	}
	if st.AnimationIterCount != 3 {
		t.Errorf("AnimationIterCount = %d, want 3", st.AnimationIterCount)
	}
	if st.AnimationDirection != css.AnimAlternate {
		t.Errorf("AnimationDirection = %v, want AnimAlternate", st.AnimationDirection)
	}
	if st.AnimationFillMode != css.FillForwards {
		t.Errorf("AnimationFillMode = %v, want FillForwards", st.AnimationFillMode)
	}
}

// TestKeyframesInStylesheet verifies that a stylesheet with a @keyframes rule
// has its Keyframes field populated.
func TestKeyframesInStylesheet(t *testing.T) {
	sheet := css.Parse(`
@keyframes fadeIn {
	from { opacity: 0 }
	to { opacity: 1 }
}
`)
	if len(sheet.Keyframes) != 1 {
		t.Fatalf("Keyframes count = %d, want 1", len(sheet.Keyframes))
	}
	kf := sheet.Keyframes[0]
	if kf.Name != "fadeIn" {
		t.Errorf("Keyframes[0].Name = %q, want %q", kf.Name, "fadeIn")
	}
	if len(kf.Stops) != 2 {
		t.Fatalf("Keyframes[0].Stops count = %d, want 2", len(kf.Stops))
	}
	if kf.Stops[0].Offset != 0 {
		t.Errorf("Stops[0].Offset = %v, want 0", kf.Stops[0].Offset)
	}
	if kf.Stops[1].Offset != 1 {
		t.Errorf("Stops[1].Offset = %v, want 1", kf.Stops[1].Offset)
	}
	if kf.Stops[0].Declarations["opacity"] != "0" {
		t.Errorf("from opacity = %q, want %q", kf.Stops[0].Declarations["opacity"], "0")
	}
	if kf.Stops[1].Declarations["opacity"] != "1" {
		t.Errorf("to opacity = %q, want %q", kf.Stops[1].Declarations["opacity"], "1")
	}
}

// TestAnimationInfiniteIteration verifies that animation-iteration-count: infinite
// is parsed as 0 (the sentinel for infinite).
func TestAnimationInfiniteIteration(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
@keyframes pulse { from { opacity: 0.5 } to { opacity: 1 } }
div { animation: pulse 1s ease infinite }
</style>
</head><body>
<div id="box" style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	el := sess.Doc.ElementByID("box")
	if el == nil {
		t.Fatal("div#box not found")
	}
	st := sess.Styles[el.ID]
	if st == nil {
		t.Fatal("div#box has no computed style")
	}

	if st.AnimationIterCount != 0 {
		t.Errorf("AnimationIterCount = %d, want 0 (infinite)", st.AnimationIterCount)
	}
}

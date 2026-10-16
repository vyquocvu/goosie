package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// simpleKeyframes returns a two-stop keyframe: from { property: fromVal } to { property: toVal }.
func simpleKeyframes(property, fromVal, toVal string) []css.KeyframeStop {
	return []css.KeyframeStop{
		{Offset: 0.0, Declarations: map[string]string{property: fromVal}},
		{Offset: 1.0, Declarations: map[string]string{property: toVal}},
	}
}

// TestAnimationControllerAdd verifies that adding an animation makes HasActive true.
func TestAnimationControllerAdd(t *testing.T) {
	var ac engine.AnimationController
	now := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, now)

	if !ac.HasActive() {
		t.Error("HasActive() = false after Add, want true")
	}
}

// TestAnimationControllerUpdate verifies that advancing time changes the offset.
func TestAnimationControllerUpdate(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	// Advance to halfway.
	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)

	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false, want true")
	}
	// At 50%, opacity should be approximately 0.5.
	if !strings.Contains(val, "0.5") {
		t.Errorf("opacity at 500ms = %q, want value containing '0.5'", val)
	}
}

// TestAnimationControllerCompletes verifies that after the full duration, the
// animation is marked as finished.
func TestAnimationControllerCompletes(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	// Advance past the duration.
	end := start.Add(time.Second + time.Millisecond)
	ac.Update(end)

	// Without fill-forwards, HasActive should be false after completion.
	// (FillNone means the animation doesn't hold after finishing.)
	// But the animation is finished, not removed. HasActive checks finished + fill.
	// With FillNone, a finished animation is not active.
	if ac.HasActive() {
		t.Error("HasActive() = true after completion with FillNone, want false")
	}
}

// TestAnimationControllerInfiniteLoop verifies that iterCount=0 never finishes.
func TestAnimationControllerInfiniteLoop(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "pulse", kf, time.Second, 0, css.TimingLinear, 0, engine.AnimNormal, engine.FillNone, start)

	// Advance well past one iteration.
	far := start.Add(10 * time.Second)
	ac.Update(far)

	if !ac.HasActive() {
		t.Error("HasActive() = false for infinite animation, want true")
	}
}

// TestAnimationControllerDelay verifies that the animation doesn't start until
// after the delay period.
func TestAnimationControllerDelay(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 500*time.Millisecond, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	// During delay: animation is active but hasn't started interpolating.
	during := start.Add(250 * time.Millisecond)
	ac.Update(during)
	if !ac.HasActive() {
		t.Error("HasActive() = false during delay, want true")
	}

	// Right after delay starts, offset should be at the beginning.
	afterDelay := start.Add(500 * time.Millisecond)
	ac.Update(afterDelay)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false after delay")
	}
	// At the start of the animation, opacity should be 0.
	if val != "0" {
		t.Errorf("opacity right after delay = %q, want '0'", val)
	}
}

// TestAnimationControllerRemoveCompleted verifies that finished animations
// without fill-forwards are removed.
func TestAnimationControllerRemoveCompleted(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	// Complete the animation.
	end := start.Add(time.Second + time.Millisecond)
	ac.Update(end)
	ac.RemoveCompleted()

	if ac.HasActive() {
		t.Error("HasActive() = true after RemoveCompleted, want false")
	}
}

// TestAnimationControllerReplace verifies that adding an animation with the
// same name on the same node replaces the existing one.
func TestAnimationControllerReplace(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf1 := simpleKeyframes("opacity", "0", "1")
	kf2 := simpleKeyframes("opacity", "0", "0.5")

	ac.Add(dom.NodeID(1), "fadeIn", kf1, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)
	ac.Add(dom.NodeID(1), "fadeIn", kf2, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	// At 500ms, the replaced animation should interpolate to 0.25 (half of 0.5).
	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)

	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false")
	}
	// Should reflect kf2 (0 to 0.5), at 50% = 0.25.
	if !strings.Contains(val, "0.25") {
		t.Errorf("opacity after replace at 500ms = %q, want value containing '0.25'", val)
	}
}

// TestAnimationGetInterpolatedValue verifies that interpolated values are
// computed correctly at various offsets.
func TestAnimationGetInterpolatedValue(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	tests := []struct {
		name   string
		offset time.Duration
		want   string
	}{
		{"start", 0, "0"},
		{"quarter", 250 * time.Millisecond, "0.25"},
		{"half", 500 * time.Millisecond, "0.5"},
		{"three_quarter", 750 * time.Millisecond, "0.75"},
		{"end", time.Second, "1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			at := start.Add(tc.offset)
			ac.Update(at)
			val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
			if !ok {
				t.Fatal("GetInterpolatedValue returned false")
			}
			if val != tc.want {
				t.Errorf("opacity at %v = %q, want %q", tc.offset, val, tc.want)
			}
		})
	}
}

// TestAnimationMaxConcurrent verifies that adding beyond MaxConcurrentAnimations
// is bounded.
func TestAnimationMaxConcurrent(t *testing.T) {
	var ac engine.AnimationController
	now := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")

	// Add more than MaxConcurrentAnimations.
	for i := 0; i < engine.MaxConcurrentAnimations+5; i++ {
		ac.Add(dom.NodeID(i), "anim", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, now)
	}

	// The controller should not exceed MaxConcurrentAnimations.
	// We can't directly inspect the count, but we can verify HasActive is true
	// and that it doesn't panic.
	if !ac.HasActive() {
		t.Error("HasActive() = false after adding many animations, want true")
	}
}

// TestAnimationDirectionReverse verifies that the animation plays backward.
func TestAnimationDirectionReverse(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeOut", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimReverse, engine.FillNone, start)

	// At 500ms (halfway), reverse means we should be at 0.5 (going from 1 to 0).
	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false")
	}
	// Reverse at 50%: offset = 1 - 0.5 = 0.5. Interpolating 0->1 at 0.5 = 0.5.
	if val != "0.5" {
		t.Errorf("opacity at 500ms reverse = %q, want '0.5'", val)
	}

	// At 0ms, reverse means we start at offset=1 (the end).
	ac2 := engine.AnimationController{}
	ac2.Add(dom.NodeID(1), "fadeOut", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimReverse, engine.FillNone, start)
	ac2.Update(start)
	val2, ok := ac2.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false at start")
	}
	// At the very start with reverse, offset=1, so opacity should be 1.
	if val2 != "1" {
		t.Errorf("opacity at start reverse = %q, want '1'", val2)
	}
}

// TestAnimationDirectionAlternate verifies that the animation alternates
// direction each iteration.
func TestAnimationDirectionAlternate(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	// 2 iterations, 1 second each.
	ac.Add(dom.NodeID(1), "pulse", kf, time.Second, 0, css.TimingLinear, 2, engine.AnimAlternate, engine.FillNone, start)

	// First iteration (0-1s): forward. At 500ms, offset=0.5, opacity=0.5.
	ac.Update(start.Add(500 * time.Millisecond))
	val, _ := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if val != "0.5" {
		t.Errorf("first iteration at 500ms = %q, want '0.5'", val)
	}

	// Second iteration (1-2s): backward. At 1.5s, progress within iteration = 0.5,
	// but reversed: offset = 1 - 0.5 = 0.5, opacity = 0.5.
	ac.Update(start.Add(1500 * time.Millisecond))
	val, _ = ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if val != "0.5" {
		t.Errorf("second iteration at 1500ms = %q, want '0.5'", val)
	}

	// At 1.0s exactly (start of second iteration), backward means offset=1.
	ac.Update(start.Add(1000 * time.Millisecond))
	val, _ = ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if val != "1" {
		t.Errorf("second iteration start = %q, want '1'", val)
	}
}

// TestAnimationFillForwards verifies that the animation holds its last value
// after completion.
func TestAnimationFillForwards(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillForwards, start)

	// After completion.
	end := start.Add(2 * time.Second)
	ac.Update(end)

	// Should still be active because of fill-forwards.
	if !ac.HasActive() {
		t.Error("HasActive() = false with FillForwards after completion, want true")
	}

	// Should still return the final value.
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false with FillForwards")
	}
	if val != "1" {
		t.Errorf("opacity after completion with FillForwards = %q, want '1'", val)
	}
}

// TestAnimationFillBackwards verifies that the animation applies the first
// keyframe value during the delay period.
func TestAnimationFillBackwards(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 500*time.Millisecond, css.TimingLinear, 1, engine.AnimNormal, engine.FillBackwards, start)

	// During delay, fill-backwards should apply the first keyframe.
	during := start.Add(250 * time.Millisecond)
	ac.Update(during)

	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false during delay with FillBackwards")
	}
	if val != "0" {
		t.Errorf("opacity during delay with FillBackwards = %q, want '0'", val)
	}
}

// TestAnimationColorInterpolation verifies that color values are interpolated.
func TestAnimationColorInterpolation(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := []css.KeyframeStop{
		{Offset: 0.0, Declarations: map[string]string{"background-color": "rgb(0, 0, 0)"}},
		{Offset: 1.0, Declarations: map[string]string{"background-color": "rgb(100, 200, 50)"}},
	}
	ac.Add(dom.NodeID(1), "colorFade", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	// At 50%.
	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "background-color")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false for color")
	}
	// Should be approximately rgb(50, 100, 25).
	if !strings.Contains(val, "50") || !strings.Contains(val, "100") || !strings.Contains(val, "25") {
		t.Errorf("background-color at 50%% = %q, want approximately rgb(50, 100, 25)", val)
	}
}

// TestAnimationNumericInterpolation verifies that numeric values with units
// are interpolated.
func TestAnimationNumericInterpolation(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := []css.KeyframeStop{
		{Offset: 0.0, Declarations: map[string]string{"width": "0px"}},
		{Offset: 1.0, Declarations: map[string]string{"width": "100px"}},
	}
	ac.Add(dom.NodeID(1), "grow", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "width")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false for width")
	}
	if !strings.Contains(val, "50") || !strings.Contains(val, "px") {
		t.Errorf("width at 50%% = %q, want value containing '50' and 'px'", val)
	}
}

// TestAnimationNoProperty verifies that GetInterpolatedValue returns false
// for properties not in the keyframes.
func TestAnimationNoProperty(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)
	ac.Update(start.Add(500 * time.Millisecond))

	_, ok := ac.GetInterpolatedValue(dom.NodeID(1), "background-color")
	if ok {
		t.Error("GetInterpolatedValue for unanimated property = true, want false")
	}

	_, ok = ac.GetInterpolatedValue(dom.NodeID(999), "opacity")
	if ok {
		t.Error("GetInterpolatedValue for non-animated node = true, want false")
	}
}

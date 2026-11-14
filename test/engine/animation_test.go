package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func simpleKeyframes(property, fromVal, toVal string) []css.KeyframeStop {
	return []css.KeyframeStop{
		{Offset: 0.0, Declarations: map[string]string{property: fromVal}},
		{Offset: 1.0, Declarations: map[string]string{property: toVal}},
	}
}

func TestAnimationControllerAdd(t *testing.T) {
	var ac engine.AnimationController
	now := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, now)

	if !ac.HasActive() {
		t.Error("HasActive() = false after Add, want true")
	}
}

func TestAnimationControllerUpdate(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)

	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false, want true")
	}
	if !strings.Contains(val, "0.5") {
		t.Errorf("opacity at 500ms = %q, want value containing '0.5'", val)
	}
}

func TestAnimationControllerCompletes(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	end := start.Add(time.Second + time.Millisecond)
	ac.Update(end)

	if ac.HasActive() {
		t.Error("HasActive() = true after completion with FillNone, want false")
	}
}

func TestAnimationControllerInfiniteLoop(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "pulse", kf, time.Second, 0, css.TimingLinear, 0, engine.AnimNormal, engine.FillNone, start)

	far := start.Add(10 * time.Second)
	ac.Update(far)

	if !ac.HasActive() {
		t.Error("HasActive() = false for infinite animation, want true")
	}
}

func TestAnimationControllerDelay(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 500*time.Millisecond, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	during := start.Add(250 * time.Millisecond)
	ac.Update(during)
	if !ac.HasActive() {
		t.Error("HasActive() = false during delay, want true")
	}

	afterDelay := start.Add(500 * time.Millisecond)
	ac.Update(afterDelay)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false after delay")
	}
	if val != "0" {
		t.Errorf("opacity right after delay = %q, want '0'", val)
	}
}

func TestAnimationControllerRemoveCompleted(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	end := start.Add(time.Second + time.Millisecond)
	ac.Update(end)
	ac.RemoveCompleted()

	if ac.HasActive() {
		t.Error("HasActive() = true after RemoveCompleted, want false")
	}
}

func TestAnimationControllerReplace(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf1 := simpleKeyframes("opacity", "0", "1")
	kf2 := simpleKeyframes("opacity", "0", "0.5")

	ac.Add(dom.NodeID(1), "fadeIn", kf1, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)
	ac.Add(dom.NodeID(1), "fadeIn", kf2, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)

	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false")
	}
	if !strings.Contains(val, "0.25") {
		t.Errorf("opacity after replace at 500ms = %q, want value containing '0.25'", val)
	}
}

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

func TestAnimationMaxConcurrent(t *testing.T) {
	var ac engine.AnimationController
	now := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")

	for i := 0; i < engine.MaxConcurrentAnimations+5; i++ {
		ac.Add(dom.NodeID(i), "anim", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, now)
	}

	if !ac.HasActive() {
		t.Error("HasActive() = false after adding many animations, want true")
	}
}

func TestAnimationDirectionReverse(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeOut", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimReverse, engine.FillNone, start)

	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false")
	}
	if val != "0.5" {
		t.Errorf("opacity at 500ms reverse = %q, want '0.5'", val)
	}

	ac2 := engine.AnimationController{}
	ac2.Add(dom.NodeID(1), "fadeOut", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimReverse, engine.FillNone, start)
	ac2.Update(start)
	val2, ok := ac2.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false at start")
	}
	if val2 != "1" {
		t.Errorf("opacity at start reverse = %q, want '1'", val2)
	}
}

func TestAnimationDirectionAlternate(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "pulse", kf, time.Second, 0, css.TimingLinear, 2, engine.AnimAlternate, engine.FillNone, start)

	ac.Update(start.Add(500 * time.Millisecond))
	val, _ := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if val != "0.5" {
		t.Errorf("first iteration at 500ms = %q, want '0.5'", val)
	}

	ac.Update(start.Add(1500 * time.Millisecond))
	val, _ = ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if val != "0.5" {
		t.Errorf("second iteration at 1500ms = %q, want '0.5'", val)
	}

	ac.Update(start.Add(1000 * time.Millisecond))
	val, _ = ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if val != "1" {
		t.Errorf("second iteration start = %q, want '1'", val)
	}
}

func TestAnimationFillForwards(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillForwards, start)

	end := start.Add(2 * time.Second)
	ac.Update(end)

	if !ac.HasActive() {
		t.Error("HasActive() = false with FillForwards after completion, want true")
	}

	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "opacity")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false with FillForwards")
	}
	if val != "1" {
		t.Errorf("opacity after completion with FillForwards = %q, want '1'", val)
	}
}

func TestAnimationFillBackwards(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := simpleKeyframes("opacity", "0", "1")
	ac.Add(dom.NodeID(1), "fadeIn", kf, time.Second, 500*time.Millisecond, css.TimingLinear, 1, engine.AnimNormal, engine.FillBackwards, start)

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

func TestAnimationColorInterpolation(t *testing.T) {
	var ac engine.AnimationController
	start := time.Now()
	kf := []css.KeyframeStop{
		{Offset: 0.0, Declarations: map[string]string{"background-color": "rgb(0, 0, 0)"}},
		{Offset: 1.0, Declarations: map[string]string{"background-color": "rgb(100, 200, 50)"}},
	}
	ac.Add(dom.NodeID(1), "colorFade", kf, time.Second, 0, css.TimingLinear, 1, engine.AnimNormal, engine.FillNone, start)

	mid := start.Add(500 * time.Millisecond)
	ac.Update(mid)
	val, ok := ac.GetInterpolatedValue(dom.NodeID(1), "background-color")
	if !ok {
		t.Fatal("GetInterpolatedValue returned false for color")
	}
	if !strings.Contains(val, "50") || !strings.Contains(val, "100") || !strings.Contains(val, "25") {
		t.Errorf("background-color at 50%% = %q, want approximately rgb(50, 100, 25)", val)
	}
}

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

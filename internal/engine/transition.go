package engine

import (
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

// ActiveTransition tracks one property transition in progress on an element.
type ActiveTransition struct {
	NodeID    dom.NodeID
	Property  string
	StartTime time.Time
	Duration  time.Duration
	Delay     time.Duration
	Timing    css.TimingFunc

	// Start and end values for interpolation. For color properties, the RGBA
	// channels are interpolated independently. For numeric properties, the
	// float64 values are interpolated.
	StartColor css.Color
	EndColor   css.Color
	StartNum   float64
	EndNum     float64
	IsColor    bool // true for color transitions, false for numeric
}

// Progress returns the current interpolation progress (0..1) accounting for
// delay and timing function. Returns 1 when the transition is complete.
func (at *ActiveTransition) Progress(now time.Time) float64 {
	elapsed := now.Sub(at.StartTime)
	if elapsed < at.Delay {
		return 0
	}
	elapsed -= at.Delay
	if at.Duration <= 0 {
		return 1
	}
	t := float64(elapsed) / float64(at.Duration)
	if t >= 1 {
		return 1
	}
	return at.Timing.Eval(t)
}

// Done reports whether the transition has completed.
func (at *ActiveTransition) Done(now time.Time) bool {
	return at.Progress(now) >= 1
}

// InterpolateColor returns the interpolated color at the given progress (0..1).
func InterpolateColor(from, to css.Color, progress float64) css.Color {
	p := progress
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	lerp := func(a, b uint8) uint8 {
		return uint8(float64(a)*(1-p) + float64(b)*p + 0.5)
	}
	return css.Color{
		R: lerp(from.R, to.R),
		G: lerp(from.G, to.G),
		B: lerp(from.B, to.B),
		A: lerp(from.A, to.A),
	}
}

// InterpolateNum returns the linearly interpolated number at the given progress.
func InterpolateNum(from, to, progress float64) float64 {
	return from*(1-progress) + to*progress
}

// TransitionTracker manages active transitions for a session.
type TransitionTracker struct {
	active []*ActiveTransition
}

// NewTransitionTracker creates a new transition tracker.
func NewTransitionTracker() *TransitionTracker {
	return &TransitionTracker{}
}

// Add registers a new transition. If a transition for the same node and property
// already exists, it is replaced (for mid-transition reversals).
func (tt *TransitionTracker) Add(at *ActiveTransition) {
	// Replace any existing transition on the same node+property.
	for i, existing := range tt.active {
		if existing.NodeID == at.NodeID && existing.Property == at.Property {
			tt.active[i] = at
			return
		}
	}
	tt.active = append(tt.active, at)
}

// RemoveCompleted removes transitions that have finished.
func (tt *TransitionTracker) RemoveCompleted(now time.Time) {
	kept := tt.active[:0]
	for _, at := range tt.active {
		if !at.Done(now) {
			kept = append(kept, at)
		}
	}
	tt.active = kept
}

// Active returns all currently active transitions.
func (tt *TransitionTracker) Active() []*ActiveTransition {
	return tt.active
}

// HasActive reports whether any transitions are in progress.
func (tt *TransitionTracker) HasActive() bool {
	return len(tt.active) > 0
}

// HasTransitionFor reports whether the given style declares a transition for
// the named property. "all" in the transition matches any property.
func HasTransitionFor(cs *style.ComputedStyle, property string) (css.Transition, bool) {
	if cs == nil || len(cs.Transitions) == 0 {
		return css.Transition{}, false
	}
	for _, tr := range cs.Transitions {
		if tr.Property == property || tr.Property == "all" {
			return tr, true
		}
	}
	return css.Transition{}, false
}

// IsTransitionable reports whether a CSS property can be transitioned. Only
// color and numeric properties are supported for now.
func IsTransitionable(property string) bool {
	switch property {
	case "background-color", "color",
		"border-top-color", "border-right-color",
		"border-bottom-color", "border-left-color",
		"opacity",
		"width", "height",
		"margin-top", "margin-right", "margin-bottom", "margin-left",
		"padding-top", "padding-right", "padding-bottom", "padding-left":
		return true
	}
	return false
}

// IsColorProperty reports whether a CSS property holds a color value.
func IsColorProperty(property string) bool {
	switch property {
	case "background-color", "color",
		"border-top-color", "border-right-color",
		"border-bottom-color", "border-left-color":
		return true
	}
	return false
}

// GetColorValue extracts the color value of a named property from a computed style.
func GetColorValue(cs *style.ComputedStyle, property string) css.Color {
	switch property {
	case "background-color":
		return cs.BackgroundColor
	case "color":
		return cs.Color
	case "border-top-color":
		return cs.BorderTopColor
	case "border-right-color":
		return cs.BorderRightColor
	case "border-bottom-color":
		return cs.BorderBottomColor
	case "border-left-color":
		return cs.BorderLeftColor
	}
	return css.Color{}
}

// GetNumericValue extracts the numeric value of a named property from a computed style.
func GetNumericValue(cs *style.ComputedStyle, property string) float64 {
	switch property {
	case "opacity":
		return float64(cs.Opacity)
	case "width":
		return float64(cs.Width)
	case "height":
		return float64(cs.Height)
	case "margin-top":
		return float64(cs.MarginTop)
	case "margin-right":
		return float64(cs.MarginRight)
	case "margin-bottom":
		return float64(cs.MarginBottom)
	case "margin-left":
		return float64(cs.MarginLeft)
	case "padding-top":
		return float64(cs.PaddingTop)
	case "padding-right":
		return float64(cs.PaddingRight)
	case "padding-bottom":
		return float64(cs.PaddingBottom)
	case "padding-left":
		return float64(cs.PaddingLeft)
	}
	return 0
}

// ApplyTransitionValue sets the interpolated value on a computed style. This is
// called during paint to apply the current transition state.
func ApplyTransitionValue(cs *style.ComputedStyle, property string, color css.Color, num float64, isColor bool) {
	if isColor {
		switch property {
		case "background-color":
			cs.BackgroundColor = color
		case "color":
			cs.Color = color
		case "border-top-color":
			cs.BorderTopColor = color
		case "border-right-color":
			cs.BorderRightColor = color
		case "border-bottom-color":
			cs.BorderBottomColor = color
		case "border-left-color":
			cs.BorderLeftColor = color
		}
		return
	}
	switch property {
	case "opacity":
		cs.Opacity = float32(num)
	case "width":
		cs.Width = float32(num)
	case "height":
		cs.Height = float32(num)
	case "margin-top":
		cs.MarginTop = float32(num)
	case "margin-right":
		cs.MarginRight = float32(num)
	case "margin-bottom":
		cs.MarginBottom = float32(num)
	case "margin-left":
		cs.MarginLeft = float32(num)
	case "padding-top":
		cs.PaddingTop = float32(num)
	case "padding-right":
		cs.PaddingRight = float32(num)
	case "padding-bottom":
		cs.PaddingBottom = float32(num)
	case "padding-left":
		cs.PaddingLeft = float32(num)
	}
}

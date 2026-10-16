package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

const (
	// MaxConcurrentAnimations is the maximum number of active animations.
	MaxConcurrentAnimations = 32
	// MaxKeyframesPerAnimation is the maximum number of keyframe stops per animation.
	MaxKeyframesPerAnimation = 64
)

// AnimationDirection controls how animations play.
// Defined in the css package; aliased here for convenience.
type AnimationDirection = css.AnimationDirection

const (
	AnimNormal           = css.AnimNormal
	AnimReverse          = css.AnimReverse
	AnimAlternate        = css.AnimAlternate
	AnimAlternateReverse = css.AnimAlternateReverse
)

// AnimationFillMode controls what happens before/after the animation.
// Defined in the css package; aliased here for convenience.
type AnimationFillMode = css.AnimationFillMode

const (
	FillNone      = css.FillNone
	FillForwards  = css.FillForwards
	FillBackwards = css.FillBackwards
	FillBoth      = css.FillBoth
)

// ActiveAnimation tracks one running @keyframes animation.
type ActiveAnimation struct {
	NodeID    dom.NodeID
	Name      string
	Keyframes []css.KeyframeStop
	StartTime time.Time
	Duration  time.Duration
	Delay     time.Duration
	Timing    css.TimingFunc
	IterCount int // 0 = infinite
	Direction AnimationDirection
	FillMode  AnimationFillMode

	// Current state
	iteration int     // current iteration (0-based)
	offset    float32 // 0.0-1.0 within current iteration
	finished  bool
}

// AnimationController manages all active @keyframes animations for a session.
type AnimationController struct {
	active []ActiveAnimation
}

// Add starts a new animation on a node. If the node already has an animation
// with the same name, it is replaced.
func (ac *AnimationController) Add(nodeID dom.NodeID, name string, keyframes []css.KeyframeStop, duration, delay time.Duration, timing css.TimingFunc, iterCount int, dir AnimationDirection, fill AnimationFillMode, now time.Time) {
	// Bound the number of keyframes.
	if len(keyframes) > MaxKeyframesPerAnimation {
		keyframes = keyframes[:MaxKeyframesPerAnimation]
	}

	anim := ActiveAnimation{
		NodeID:    nodeID,
		Name:      name,
		Keyframes: keyframes,
		StartTime: now,
		Duration:  duration,
		Delay:     delay,
		Timing:    timing,
		IterCount: iterCount,
		Direction: dir,
		FillMode:  fill,
	}

	// Replace existing animation with same name on same node.
	for i, existing := range ac.active {
		if existing.NodeID == nodeID && existing.Name == name {
			ac.active[i] = anim
			return
		}
	}

	// Enforce max concurrent animations.
	if len(ac.active) >= MaxConcurrentAnimations {
		// Replace the oldest animation.
		copy(ac.active, ac.active[1:])
		ac.active[len(ac.active)-1] = anim
		return
	}

	ac.active = append(ac.active, anim)
}

// Update advances all animations to the current time. Returns true if any
// animation is still active (needs another frame).
func (ac *AnimationController) Update(now time.Time) bool {
	anyActive := false
	for i := range ac.active {
		a := &ac.active[i]
		if a.finished {
			// Still active if fill mode holds the value.
			if a.FillMode == FillForwards || a.FillMode == FillBoth {
				anyActive = true
			}
			continue
		}

		elapsed := now.Sub(a.StartTime)

		// Still in delay period.
		if elapsed < a.Delay {
			anyActive = true
			continue
		}

		activeDuration := elapsed - a.Delay
		if a.Duration <= 0 {
			a.finished = true
			a.iteration = 0
			a.offset = finalOffset(a)
			continue
		}

		// Calculate total progress in iterations.
		totalProgress := float64(activeDuration) / float64(a.Duration)

		if a.IterCount > 0 && int(totalProgress) >= a.IterCount {
			// Animation complete.
			a.finished = true
			a.iteration = a.IterCount - 1
			a.offset = finalOffset(a)
			continue
		}

		// Current iteration.
		a.iteration = int(totalProgress)
		// Progress within this iteration (0..1).
		iterProgress := totalProgress - float64(a.iteration)

		// Apply direction.
		a.offset = applyDirection(iterProgress, a.iteration, a.Direction)

		// Apply timing function.
		eased := a.Timing.Eval(float64(a.offset))
		a.offset = float32(eased)

		anyActive = true
	}
	return anyActive
}

// HasActive reports whether any animation is in progress.
func (ac *AnimationController) HasActive() bool {
	for i := range ac.active {
		a := &ac.active[i]
		if !a.finished {
			return true
		}
		// Finished but still holding via fill mode.
		if a.FillMode == FillForwards || a.FillMode == FillBoth {
			return true
		}
	}
	return false
}

// RemoveCompleted removes finished animations (those past their iteration count
// and without a forwards fill).
func (ac *AnimationController) RemoveCompleted() {
	kept := ac.active[:0]
	for _, a := range ac.active {
		if a.finished && a.FillMode != FillForwards && a.FillMode != FillBoth {
			continue
		}
		kept = append(kept, a)
	}
	ac.active = kept
}

// GetInterpolatedValue returns the interpolated value for a property on a node
// at the current animation offset. Returns ("", false) if no animation targets
// that property.
func (ac *AnimationController) GetInterpolatedValue(nodeID dom.NodeID, property string) (string, bool) {
	for i := range ac.active {
		a := &ac.active[i]
		if a.NodeID != nodeID {
			continue
		}
		if len(a.Keyframes) == 0 {
			continue
		}

		// Check if any keyframe has this property.
		hasProp := false
		for _, kf := range a.Keyframes {
			if _, ok := kf.Declarations[property]; ok {
				hasProp = true
				break
			}
		}
		if !hasProp {
			return "", false
		}

		return interpolateAtOffset(a, property, a.offset), true
	}
	return "", false
}

// interpolateAtOffset computes the interpolated value for a property at the
// given offset (0..1) within the keyframe stops.
func interpolateAtOffset(a *ActiveAnimation, property string, offset float32) string {
	stops := a.Keyframes
	if len(stops) == 0 {
		return ""
	}
	if len(stops) == 1 {
		v, ok := stops[0].Declarations[property]
		if !ok {
			return ""
		}
		return v
	}

	// Find the two surrounding stops.
	lo, hi := 0, len(stops)-1
	for i := 0; i < len(stops)-1; i++ {
		if offset >= stops[i].Offset && offset <= stops[i+1].Offset {
			lo = i
			hi = i + 1
			break
		}
	}

	// If offset is at or before the first stop.
	if offset <= stops[0].Offset {
		v, _ := stops[0].Declarations[property]
		return v
	}
	// If offset is at or after the last stop.
	if offset >= stops[len(stops)-1].Offset {
		v, _ := stops[len(stops)-1].Declarations[property]
		return v
	}

	fromVal, fromOK := stops[lo].Declarations[property]
	toVal, toOK := stops[hi].Declarations[property]

	if !fromOK && !toOK {
		return ""
	}
	if !fromOK {
		return toVal
	}
	if !toOK {
		return fromVal
	}

	// Calculate segment progress.
	span := float64(stops[hi].Offset - stops[lo].Offset)
	if span <= 0 {
		return toVal
	}
	segProgress := (float64(offset) - float64(stops[lo].Offset)) / span

	// Try color interpolation.
	if fromColor, ok := css.ParseColor(fromVal); ok {
		if toColor, ok := css.ParseColor(toVal); ok {
			c := InterpolateColor(fromColor, toColor, segProgress)
			return formatColor(c)
		}
	}

	// Try numeric interpolation.
	if fromNum, fromUnit, ok := parseNumericValue(fromVal); ok {
		if toNum, toUnit, ok := parseNumericValue(toVal); ok && toUnit == fromUnit {
			n := InterpolateNum(fromNum, toNum, segProgress)
			return formatNumericValue(n, fromUnit)
		}
	}

	// Discrete: snap at midpoint.
	if segProgress < 0.5 {
		return fromVal
	}
	return toVal
}

// applyDirection computes the effective offset within an iteration, accounting
// for the animation direction and current iteration number.
func applyDirection(progress float64, iteration int, dir AnimationDirection) float32 {
	switch dir {
	case AnimNormal:
		return float32(progress)
	case AnimReverse:
		return float32(1 - progress)
	case AnimAlternate:
		if iteration%2 == 0 {
			return float32(progress) // forward on even iterations
		}
		return float32(1 - progress) // backward on odd iterations
	case AnimAlternateReverse:
		if iteration%2 == 0 {
			return float32(1 - progress) // backward on even iterations
		}
		return float32(progress) // forward on odd iterations
	}
	return float32(progress)
}

// finalOffset returns the offset to use when an animation has finished.
func finalOffset(a *ActiveAnimation) float32 {
	switch a.Direction {
	case AnimNormal, AnimAlternate:
		// Ended forward.
		if a.IterCount > 0 && a.IterCount%2 == 0 && a.Direction == AnimAlternate {
			return 0 // even iteration count with alternate = ended backward
		}
		return 1
	case AnimReverse, AnimAlternateReverse:
		if a.Direction == AnimAlternateReverse {
			if a.IterCount > 0 && a.IterCount%2 == 0 {
				return 1
			}
		}
		return 0
	}
	return 1
}

// parseNumericValue tries to parse a CSS value as a number with an optional unit.
func parseNumericValue(s string) (float64, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", false
	}

	// Find where the numeric part ends.
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	hasDigit := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		hasDigit = true
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			hasDigit = true
			i++
		}
	}
	if !hasDigit {
		return 0, "", false
	}

	numStr := s[:i]
	unit := strings.TrimSpace(s[i:])

	v, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, "", false
	}
	return v, unit, true
}

// formatNumericValue formats a numeric value with its unit.
func formatNumericValue(v float64, unit string) string {
	// Format with enough precision but trim trailing zeros.
	s := strconv.FormatFloat(v, 'f', 4, 64)
	// Trim trailing zeros after decimal point.
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s + unit
}

// formatColor formats a Color as an rgba() string.
func formatColor(c css.Color) string {
	if c.A == 255 {
		return "rgb(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ")"
	}
	alpha := float64(c.A) / 255.0
	return "rgba(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ", " + strconv.FormatFloat(alpha, 'f', 2, 64) + ")"
}

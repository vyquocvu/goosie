package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

const (
	MaxConcurrentAnimations  = 32
	MaxKeyframesPerAnimation = 64
)

type AnimationDirection = css.AnimationDirection

const (
	AnimNormal           = css.AnimNormal
	AnimReverse          = css.AnimReverse
	AnimAlternate        = css.AnimAlternate
	AnimAlternateReverse = css.AnimAlternateReverse
)

type AnimationFillMode = css.AnimationFillMode

const (
	FillNone      = css.FillNone
	FillForwards  = css.FillForwards
	FillBackwards = css.FillBackwards
	FillBoth      = css.FillBoth
)

type ActiveAnimation struct {
	NodeID    dom.NodeID
	Name      string
	Keyframes []css.KeyframeStop
	StartTime time.Time
	Duration  time.Duration
	Delay     time.Duration
	Timing    css.TimingFunc
	IterCount int
	Direction AnimationDirection
	FillMode  AnimationFillMode

	iteration int
	offset    float32
	finished  bool
}

type AnimationController struct {
	active []ActiveAnimation
}

func (ac *AnimationController) Add(nodeID dom.NodeID, name string, keyframes []css.KeyframeStop, duration, delay time.Duration, timing css.TimingFunc, iterCount int, dir AnimationDirection, fill AnimationFillMode, now time.Time) {
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

	for i, existing := range ac.active {
		if existing.NodeID == nodeID && existing.Name == name {
			ac.active[i] = anim
			return
		}
	}

	if len(ac.active) >= MaxConcurrentAnimations {
		copy(ac.active, ac.active[1:])
		ac.active[len(ac.active)-1] = anim
		return
	}

	ac.active = append(ac.active, anim)
}

func (ac *AnimationController) Update(now time.Time) bool {
	anyActive := false
	for i := range ac.active {
		a := &ac.active[i]
		if a.finished {
			if a.FillMode == FillForwards || a.FillMode == FillBoth {
				anyActive = true
			}
			continue
		}

		elapsed := now.Sub(a.StartTime)

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

		totalProgress := float64(activeDuration) / float64(a.Duration)

		if a.IterCount > 0 && int(totalProgress) >= a.IterCount {
			a.finished = true
			a.iteration = a.IterCount - 1
			a.offset = finalOffset(a)
			continue
		}

		a.iteration = int(totalProgress)
		iterProgress := totalProgress - float64(a.iteration)

		a.offset = applyDirection(iterProgress, a.iteration, a.Direction)

		eased := a.Timing.Eval(float64(a.offset))
		a.offset = float32(eased)

		anyActive = true
	}
	return anyActive
}

func (ac *AnimationController) HasActive() bool {
	for i := range ac.active {
		a := &ac.active[i]
		if !a.finished {
			return true
		}
		if a.FillMode == FillForwards || a.FillMode == FillBoth {
			return true
		}
	}
	return false
}

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

func (ac *AnimationController) GetInterpolatedValue(nodeID dom.NodeID, property string) (string, bool) {
	for i := range ac.active {
		a := &ac.active[i]
		if a.NodeID != nodeID {
			continue
		}
		if len(a.Keyframes) == 0 {
			continue
		}

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

	lo, hi := 0, len(stops)-1
	for i := 0; i < len(stops)-1; i++ {
		if offset >= stops[i].Offset && offset <= stops[i+1].Offset {
			lo = i
			hi = i + 1
			break
		}
	}

	if offset <= stops[0].Offset {
		v, _ := stops[0].Declarations[property]
		return v
	}
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

	span := float64(stops[hi].Offset - stops[lo].Offset)
	if span <= 0 {
		return toVal
	}
	segProgress := (float64(offset) - float64(stops[lo].Offset)) / span

	if fromColor, ok := css.ParseColor(fromVal); ok {
		if toColor, ok := css.ParseColor(toVal); ok {
			c := InterpolateColor(fromColor, toColor, segProgress)
			return formatColor(c)
		}
	}

	if fromNum, fromUnit, ok := parseNumericValue(fromVal); ok {
		if toNum, toUnit, ok := parseNumericValue(toVal); ok && toUnit == fromUnit {
			n := InterpolateNum(fromNum, toNum, segProgress)
			return formatNumericValue(n, fromUnit)
		}
	}

	if segProgress < 0.5 {
		return fromVal
	}
	return toVal
}

func applyDirection(progress float64, iteration int, dir AnimationDirection) float32 {
	switch dir {
	case AnimNormal:
		return float32(progress)
	case AnimReverse:
		return float32(1 - progress)
	case AnimAlternate:
		if iteration%2 == 0 {
			return float32(progress)
		}
		return float32(1 - progress)
	case AnimAlternateReverse:
		if iteration%2 == 0 {
			return float32(1 - progress)
		}
		return float32(progress)
	}
	return float32(progress)
}

func finalOffset(a *ActiveAnimation) float32 {
	switch a.Direction {
	case AnimNormal, AnimAlternate:
		if a.IterCount > 0 && a.IterCount%2 == 0 && a.Direction == AnimAlternate {
			return 0
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

func parseNumericValue(s string) (float64, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", false
	}

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

func formatNumericValue(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s + unit
}

func formatColor(c css.Color) string {
	if c.A == 255 {
		return "rgb(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ")"
	}
	alpha := float64(c.A) / 255.0
	return "rgba(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ", " + strconv.FormatFloat(alpha, 'f', 2, 64) + ")"
}

package js

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

const inputMillisecondsPerDay = 86_400_000.0

func (r *Runtime) installInputNumericValue(proto *goja.Object) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" {
			return r.vm.ToValue(math.NaN())
		}
		typ := asciiLowerString(node.GetAttribute("type"))
		if _, _, _, ok := inputStepParameters(typ); !ok {
			return r.vm.ToValue(math.NaN())
		}
		value := node.InputControlValue()
		number, valid := parseInputRangeValue(typ, value)
		if !valid {
			return r.vm.ToValue(math.NaN())
		}
		if typ == "date" || typ == "week" {
			number *= inputMillisecondsPerDay
		}
		return r.vm.ToValue(number)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		number := math.NaN()
		if len(call.Arguments) > 0 && call.Argument(0) != nil {
			number = call.Argument(0).ToFloat()
		}
		if math.IsInf(number, 0) {
			panic(r.vm.NewTypeError("The value for valueAsNumber must be a finite number."))
		}
		typ := asciiLowerString(node.GetAttribute("type"))
		if _, _, _, ok := inputStepParameters(typ); !ok {
			panic(r.domError("InvalidStateError", "The valueAsNumber property is not applicable to this input type."))
		}
		value := ""
		if !math.IsNaN(number) {
			switch typ {
			case "date":
				value = formatInputStepDate(math.Floor(number / inputMillisecondsPerDay))
			case "week":
				value = formatInputStepWeek(math.Floor(number / inputMillisecondsPerDay))
			case "time":
				value = formatInputStepTime(number)
			case "datetime-local":
				if math.Abs(number) < 8.64e15 {
					days := math.Floor(number / inputMillisecondsPerDay)
					value = formatInputStepDate(days) + "T" + formatInputStepTime(number-days*inputMillisecondsPerDay)
				}
			case "month":
				value = formatInputStepValue(typ, math.Trunc(number))
			default:
				value = strconv.FormatFloat(number, 'f', -1, 64)
			}
		}
		before := node.InputControlValue()
		node.SetInputControlValue(r.sanitizeInputValue(node, value), true)
		node.ValueChangedByUser = false
		r.RefreshInputPatternMismatch(node)
		if before != node.InputControlValue() {
			r.resetTextSelectionAfterValueSet(node, node.InputControlValue())
			r.notifyMutation()
		}
		return goja.Undefined()
	})
	_ = proto.DefineAccessorProperty("valueAsNumber", getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	dateGetter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" {
			return goja.Null()
		}
		typ := asciiLowerString(node.GetAttribute("type"))
		if typ != "date" && typ != "month" && typ != "week" && typ != "time" {
			return goja.Null()
		}
		value := node.InputControlValue()
		number, valid := parseInputRangeValue(typ, value)
		if !valid {
			return goja.Null()
		}
		switch typ {
		case "date", "week":
			number *= inputMillisecondsPerDay
		case "month":
			month := formatInputStepValue("month", number)
			days, ok := parseHTMLDate(month + "-01")
			if !ok {
				return goja.Null()
			}
			number = days * inputMillisecondsPerDay
		}
		return r.newDateValue(number)
	})
	dateSetter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		typ := asciiLowerString(node.GetAttribute("type"))
		if typ != "date" && typ != "month" && typ != "week" && typ != "time" {
			panic(r.domError("InvalidStateError", "The valueAsDate property is not applicable to this input type."))
		}
		if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) {
			r.setInputDateValue(node, "")
			return goja.Undefined()
		}
		dateTime, ok := call.Argument(0).Export().(time.Time)
		if !ok {
			panic(r.vm.NewTypeError("The value for valueAsDate must be a Date object."))
		}
		dateObject := call.Argument(0).ToObject(r.vm)
		getTime, ok := goja.AssertFunction(dateObject.Get("getTime"))
		if !ok {
			panic(r.vm.NewTypeError("The value for valueAsDate must be a Date object."))
		}
		millisValue, err := getTime(dateObject)
		if err != nil {
			panic(err)
		}
		millis := millisValue.ToFloat()
		value := ""
		if !math.IsNaN(millis) && !math.IsInf(millis, 0) {
			utc := dateTime.UTC()
			switch typ {
			case "date":
				value = utc.Format("2006-01-02")
			case "month":
				value = utc.Format("2006-01")
			case "week":
				year, week := utc.ISOWeek()
				value = strconv.Itoa(year) + "-W" + twoDigits(int64(week))
			case "time":
				dayMillis := math.Mod(millis, inputMillisecondsPerDay)
				if dayMillis < 0 {
					dayMillis += inputMillisecondsPerDay
				}
				value = formatInputStepTime(dayMillis)
			}
		}
		r.setInputDateValue(node, value)
		return goja.Undefined()
	})
	_ = proto.DefineAccessorProperty("valueAsDate", dateGetter, dateSetter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) newDateValue(milliseconds float64) goja.Value {
	constructor, ok := goja.AssertConstructor(r.vm.Get("Date"))
	if !ok {
		return goja.Null()
	}
	date, err := constructor(nil, r.vm.ToValue(milliseconds))
	if err != nil {
		return goja.Null()
	}
	return date
}

func (r *Runtime) setInputDateValue(node *dom.Node, value string) {
	if node == nil {
		return
	}
	before := node.InputControlValue()
	node.SetInputControlValue(r.sanitizeInputValue(node, value), true)
	node.ValueChangedByUser = false
	r.RefreshInputPatternMismatch(node)
	if before != node.InputControlValue() {
		r.resetTextSelectionAfterValueSet(node, node.InputControlValue())
		r.notifyMutation()
	}
}

func (r *Runtime) installInputStepping(proto *goja.Object) {
	_ = proto.Set("stepUp", func(call goja.FunctionCall) goja.Value {
		return r.stepInput(call, 1)
	})
	_ = proto.Set("stepDown", func(call goja.FunctionCall) goja.Value {
		return r.stepInput(call, -1)
	})
}

func (r *Runtime) stepInput(call goja.FunctionCall, direction int64) goja.Value {
	node := r.getNode(call.This)
	if node == nil || node.Data != "input" {
		panic(r.domError("InvalidStateError", "This input type does not support stepping."))
	}
	typ := asciiLowerString(node.GetAttribute("type"))
	defaultStep, scale, defaultBase, supported := inputStepParameters(typ)
	if !supported || node.HasAttribute("step") && strings.EqualFold(node.GetAttribute("step"), "any") {
		panic(r.domError("InvalidStateError", "This input type does not support stepping."))
	}
	steps := int64(1)
	if len(call.Arguments) > 0 {
		steps = int64(int32(call.Arguments[0].ToInteger()))
	}
	if steps == 0 {
		return goja.Undefined()
	}
	steps *= direction
	if minimum, hasMinimum := parseInputRangeValue(typ, node.GetAttribute("min")); node.HasAttribute("min") && hasMinimum {
		if maximum, hasMaximum := parseInputRangeValue(typ, node.GetAttribute("max")); node.HasAttribute("max") && hasMaximum && minimum > maximum {
			return goja.Undefined()
		}
	}

	step := defaultStep
	if parsed, ok := parseHTMLFloatingPoint(node.GetAttribute("step")); node.HasAttribute("step") && ok && parsed > 0 {
		step = parsed
	}
	step *= scale
	base := defaultBase
	if parsed, ok := parseInputRangeValue(typ, node.GetAttribute("min")); node.HasAttribute("min") && ok {
		base = parsed
	} else if parsed, ok := parseInputRangeValue(typ, node.GetAttribute("value")); node.HasAttribute("value") && ok {
		base = parsed
	}
	current, valid := parseInputRangeValue(typ, node.InputControlValue())
	if !node.InputValueDirty {
		clean := r.sanitizeInputValue(node, node.InputControlValue())
		if clean != node.InputControlValue() {
			node.SetInputControlValue(clean, false)
		}
		current, valid = parseInputRangeValue(typ, clean)
	}
	if !valid {
		current = defaultBase
		if minimum, ok := parseInputRangeValue(typ, node.GetAttribute("min")); node.HasAttribute("min") && ok && minimum > defaultBase {
			current = minimum
		}
	}
	if valid {
		if minimum, ok := parseInputRangeValue(typ, node.GetAttribute("min")); node.HasAttribute("min") && ok && current < minimum && steps < 0 {
			return goja.Undefined()
		}
		if maximum, ok := parseInputRangeValue(typ, node.GetAttribute("max")); node.HasAttribute("max") && ok && current > maximum && steps > 0 {
			return goja.Undefined()
		}
	}
	quotient := (current - base) / step
	var target float64
	if steps > 0 {
		target = base + (math.Floor(quotient+1e-10)+float64(steps))*step
	} else {
		target = base + (math.Ceil(quotient-1e-10)+float64(steps))*step
	}
	if minimum, ok := parseInputRangeValue(typ, node.GetAttribute("min")); node.HasAttribute("min") && ok && target < minimum {
		target = base + math.Ceil((minimum-base)/step)*step
	}
	if maximum, ok := parseInputRangeValue(typ, node.GetAttribute("max")); node.HasAttribute("max") && ok && target > maximum {
		target = base + math.Floor((maximum-base)/step)*step
	}
	value := formatInputStepValue(typ, target)
	node.SetInputControlValue(r.sanitizeInputValue(node, value), true)
	node.ValueChangedByUser = false
	r.RefreshInputPatternMismatch(node)
	r.resetTextSelectionAfterValueSet(node, node.InputControlValue())
	r.notifyMutation()
	return goja.Undefined()
}

func inputStepParameters(typ string) (defaultStep, scale, defaultBase float64, ok bool) {
	switch typ {
	case "number", "range":
		return 1, 1, 0, true
	case "date", "month":
		return 1, 1, 0, true
	case "week":
		return 1, 7, -3, true
	case "time", "datetime-local":
		return 60, 1000, 0, true
	default:
		return 0, 0, 0, false
	}
}

func formatInputStepValue(typ string, value float64) string {
	switch typ {
	case "number", "range":
		if math.Abs(value) < 1e12 {
			value = math.Round(value*1e12) / 1e12
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case "date":
		return formatInputStepDate(value)
	case "month":
		months := int64(math.Round(value))
		year := int64(1970) + floorDiv(months, 12)
		month := months - floorDiv(months, 12)*12 + 1
		return strconv.FormatInt(year, 10) + "-" + twoDigits(month)
	case "week":
		return formatInputStepWeek(value)
	case "time":
		return formatInputStepTime(value)
	case "datetime-local":
		days := math.Floor(value / 86_400_000)
		milliseconds := value - days*86_400_000
		return formatInputStepDate(days) + "T" + formatInputStepTime(milliseconds)
	default:
		return ""
	}
}

func formatInputStepDate(days float64) string {
	date := time.Unix(int64(math.Round(days))*86400, 0).UTC()
	return date.Format("2006-01-02")
}

func formatInputStepWeek(days float64) string {
	date := time.Unix(int64(math.Round(days))*86400, 0).UTC()
	year, week := date.ISOWeek()
	return strconv.Itoa(year) + "-W" + twoDigits(int64(week))
}

func formatInputStepTime(milliseconds float64) string {
	day := 86_400_000.0
	milliseconds = math.Mod(milliseconds, day)
	if milliseconds < 0 {
		milliseconds += day
	}
	total := int64(math.Round(milliseconds))
	hour := total / 3_600_000
	minute := total / 60_000 % 60
	second := total / 1000 % 60
	millis := total % 1000
	value := twoDigits(hour) + ":" + twoDigits(minute)
	if second != 0 || millis != 0 {
		value += ":" + twoDigits(second)
		if millis != 0 {
			fraction := strings.TrimRight(strconv.FormatInt(1000+millis, 10)[1:], "0")
			value += "." + fraction
		}
	}
	return value
}

func twoDigits(value int64) string {
	if value < 10 {
		return "0" + strconv.FormatInt(value, 10)
	}
	return strconv.FormatInt(value, 10)
}

func floorDiv(value, divisor int64) int64 {
	q, rem := value/divisor, value%divisor
	if rem < 0 {
		q--
	}
	return q
}

package js

import (
	"fmt"
	"math"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"golang.org/x/net/idna"
)

const htmlEmailAddressPattern = "^[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$"

var validityFlagNames = []string{
	"badInput", "customError", "patternMismatch", "rangeOverflow", "rangeUnderflow",
	"stepMismatch", "tooLong", "tooShort", "typeMismatch", "valueMissing",
}

func (r *Runtime) setupValidityStateInterface() {
	if r.validityProto != nil {
		return
	}
	objectCtor := r.vm.Get("Object").ToObject(r.vm)
	r.validityProto = r.newChildProto(objectCtor.Get("prototype").ToObject(r.vm))
	r.defineConstructor("ValidityState", r.validityProto)
	for _, name := range validityFlagNames {
		name := name
		_ = r.validityProto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			return r.vm.ToValue(r.formControlValidityFlag(r.validityNode(call.This), name))
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	_ = r.validityProto.DefineAccessorProperty("valid", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.validityNode(call.This)
		for _, name := range validityFlagNames {
			if r.formControlValidityFlag(node, name) {
				return r.vm.ToValue(false)
			}
		}
		return r.vm.ToValue(true)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) installFormControlValidation(proto *goja.Object) {
	r.setupValidityStateInterface()
	_ = proto.DefineAccessorProperty("validity", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.formControlValidity(node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("willValidate", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.formControlWillValidate(r.getNode(call.This)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("validationMessage", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.formControlValidationMessage(r.getNode(call.This)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	check := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.checkNativeFormControlValidity(r.getNode(call.This)))
	})
	_ = proto.Set("checkValidity", check)
	_ = proto.Set("reportValidity", check)
	_ = proto.Set("setCustomValidity", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'setCustomValidity': 1 argument required."))
		}
		if node != nil {
			message := call.Argument(0).String()
			node.CustomValidityMessage = strings.ReplaceAll(strings.ReplaceAll(message, "\r\n", "\n"), "\r", "\n")
		}
		return goja.Undefined()
	}))
}

func (r *Runtime) formControlValidity(node *dom.Node) *goja.Object {
	if r.formControlValidities == nil {
		r.formControlValidities = make(map[*dom.Node]*goja.Object)
	}
	if validity := r.formControlValidities[node]; validity != nil {
		return validity
	}
	validity := r.vm.NewObject()
	_ = validity.SetPrototype(r.validityProto)
	if r.validityControlNodes == nil {
		r.validityControlNodes = make(map[*goja.Object]*dom.Node)
	}
	r.validityControlNodes[validity] = node
	r.formControlValidities[node] = validity
	return validity
}

func (r *Runtime) validityNode(value goja.Value) *dom.Node {
	object, _ := value.(*goja.Object)
	if object == nil {
		return nil
	}
	return r.validityControlNodes[object]
}

func (r *Runtime) formControlValidityFlag(node *dom.Node, name string) bool {
	if node == nil {
		return false
	}
	switch name {
	case "badInput":
		return node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "number") && node.InputBadInput
	case "customError":
		return node.CustomValidityMessage != ""
	case "patternMismatch":
		return r.inputPatternMismatch(node)
	case "typeMismatch":
		return r.inputTypeMismatch(node)
	case "rangeUnderflow", "rangeOverflow", "stepMismatch":
		r.updateRangeValidity(node)
		switch name {
		case "rangeUnderflow":
			return node.RangeUnderflow
		case "rangeOverflow":
			return node.RangeOverflow
		default:
			return node.StepMismatch
		}
	case "tooLong":
		return dom.IsTooLong(node)
	case "tooShort":
		return dom.IsTooShort(node)
	case "valueMissing":
		return r.formControlValueMissing(node)
	default:
		return false
	}
}

func (r *Runtime) refreshDocumentPatternMismatches() {
	if r.domDoc == nil {
		return
	}
	r.domDoc.Node.Walk(func(node *dom.Node) bool {
		if node.Data == "input" {
			r.RefreshInputPatternMismatch(node)
		}
		return true
	})
}

// RefreshInputPatternMismatch updates cached input validity after a host-side
// user edit so CSS :invalid/:valid selectors observe the new value.
func (r *Runtime) RefreshInputPatternMismatch(node *dom.Node) {
	if r != nil {
		if node != nil && node.Data == "input" {
			typ := strings.ToLower(node.GetAttribute("type"))
			if typ == "file" {
				if !strings.EqualFold(node.InputValueType, "file") {
					r.clearInputFiles(node)
				}
				node.InputValueType = typ
			} else if !node.InputValueDirty {
				node.SyncInputControlValueFromAttribute()
				value := r.sanitizeInputValue(node, node.InputValue)
				node.SetInputControlValue(value, false)
			} else if node.InputValueType != typ || (typ == "email" && node.InputValueMultiple != node.HasAttribute("multiple")) {
				if dom.InputValueMode(typ) {
					node.SetInputControlValue(r.sanitizeInputValue(node, node.InputValue), false)
				} else {
					node.InputValueType = typ
				}
			}
		}
		if node != nil && (node.Data == "input" || node.Data == "textarea") {
			r.clampTextControlSelection(node)
		}
		r.inputPatternMismatch(node)
		r.inputTypeMismatch(node)
		r.updateRangeValidity(node)
	}
}

func (r *Runtime) sanitizeInputValue(node *dom.Node, value string) string {
	if node == nil || node.Data != "input" {
		return value
	}
	typ := strings.ToLower(node.GetAttribute("type"))
	switch typ {
	case "", "text", "search", "tel", "password":
		return strings.Map(func(ch rune) rune {
			if ch == '\n' || ch == '\r' {
				return -1
			}
			return ch
		}, value)
	case "url":
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", "")
		return strings.Trim(value, "\t\n\f\r ")
	case "email":
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", "")
		if node.HasAttribute("multiple") {
			addresses := strings.Split(value, ",")
			for i := range addresses {
				addresses[i] = asciiEmailAddressToASCII(strings.Trim(addresses[i], "\t\n\f\r "))
			}
			return strings.Join(addresses, ",")
		}
		return asciiEmailAddressToASCII(strings.Trim(value, "\t\n\f\r "))
	case "date", "month", "week", "time", "datetime-local":
		number, valid := parseInputRangeValue(typ, value)
		if !valid {
			return ""
		}
		if typ == "datetime-local" {
			return formatInputStepValue(typ, number)
		}
		return value
	case "number":
		_, valid := parseHTMLFloatingPoint(value)
		if !valid {
			return ""
		}
		return value
	case "range":
		return sanitizeRangeValue(node, value)
	case "color":
		return sanitizeLegacyColorValue(value)
	default:
		return value
	}
}

func asciiEmailAddressToASCII(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at <= 0 || at == len(address)-1 || strings.Contains(address[:at], "@") {
		return address
	}
	domain, err := idna.Lookup.ToASCII(address[at+1:])
	if err != nil {
		return address
	}
	return address[:at+1] + domain
}

// SanitizeInputValue applies the current input state's HTML value
// sanitization algorithm. Engine editing paths use this before committing a
// user-selected value so script and user writes share the same normalization.
func (r *Runtime) SanitizeInputValue(node *dom.Node, value string) string {
	return r.sanitizeInputValue(node, value)
}

func sanitizeLegacyColorValue(value string) string {
	color, ok := css.ParseColor(value)
	if !ok || color.A != 255 {
		return "#000000"
	}
	return fmt.Sprintf("#%02x%02x%02x", color.R, color.G, color.B)
}

func sanitizeRangeValue(node *dom.Node, value string) string {
	minimum, hasMinimum := parseHTMLFloatingPoint(node.GetAttribute("min"))
	if !hasMinimum {
		minimum = 0
	}
	maximum, hasMaximum := parseHTMLFloatingPoint(node.GetAttribute("max"))
	if !hasMaximum {
		maximum = 100
	}
	if maximum < minimum {
		maximum = minimum
	}
	number, valid := parseHTMLFloatingPoint(value)
	if !valid {
		number = minimum + (maximum-minimum)/2
	}
	if number < minimum {
		number = minimum
	}
	if maximum >= minimum && number > maximum {
		number = maximum
	}
	stepText := node.GetAttribute("step")
	if !(node.HasAttribute("step") && strings.EqualFold(stepText, "any")) {
		step := float64(1)
		if parsed, ok := parseHTMLFloatingPoint(stepText); node.HasAttribute("step") && ok && parsed > 0 {
			step = parsed
		}
		base := minimum
		if !hasMinimum {
			if parsed, ok := parseHTMLFloatingPoint(node.GetAttribute("value")); ok && node.HasAttribute("value") {
				base = parsed
			} else {
				base = 0
			}
		}
		quotient := (number - base) / step
		lower := math.Floor(quotient)
		upper := lower + 1
		if quotient-lower >= 0.5 {
			lower = upper
		}
		number = base + lower*step
		minStep := math.Ceil((minimum - base) / step)
		if number < minimum {
			number = base + minStep*step
		}
		if maximum >= minimum && number > maximum {
			maxStep := math.Floor((maximum - base) / step)
			number = base + maxStep*step
		}
	}
	precision := 0
	if node.HasAttribute("step") && strings.EqualFold(stepText, "any") {
		precision = decimalPlaceCount(value)
	} else {
		precision = decimalPlaceCount(stepText)
	}
	precision = max(precision, decimalPlaceCount(node.GetAttribute("min")))
	precision = max(precision, decimalPlaceCount(node.GetAttribute("value")))
	if precision > 15 {
		precision = 15
	}
	formatted := strconv.FormatFloat(number, 'f', precision, 64)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	return formatted
}

func decimalPlaceCount(value string) int {
	if _, ok := parseHTMLFloatingPoint(value); !ok {
		return 0
	}
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(value), "e")
	places := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		places = len(mantissa) - dot - 1
	}
	if hasExponent {
		exponent, _ := strconv.Atoi(exponentText)
		places -= exponent
	}
	if places < 0 {
		return 0
	}
	return places
}

func (r *Runtime) inputPatternMismatch(node *dom.Node) bool {
	if node == nil || node.Data != "input" || !node.HasAttribute("pattern") {
		if node != nil {
			node.PatternMismatch = false
		}
		return false
	}
	typ := strings.ToLower(node.GetAttribute("type"))
	switch typ {
	case "", "text", "search", "tel", "url", "email", "password":
	default:
		node.PatternMismatch = false
		return false
	}
	value := node.InputControlValue()
	if value == "" {
		node.PatternMismatch = false
		return false
	}
	pattern := node.GetAttribute("pattern")
	compiled, validPattern := compileHTMLInputPattern(pattern)
	if !validPattern {
		node.PatternMismatch = false
		return false
	}
	values := []string{value}
	if typ == "email" && node.HasAttribute("multiple") {
		values = strings.Split(value, ",")
	}
	node.PatternMismatch = false
	for _, candidate := range values {
		if typ == "email" && node.HasAttribute("multiple") {
			candidate = strings.Trim(candidate, "\t\n\f\r ")
		}
		if !compiled.MatchString(candidate) {
			node.PatternMismatch = true
			break
		}
	}
	return node.PatternMismatch
}

func (r *Runtime) inputTypeMismatch(node *dom.Node) bool {
	if node == nil || node.Data != "input" {
		if node != nil {
			node.TypeMismatch = false
		}
		return false
	}
	typ := strings.ToLower(node.GetAttribute("type"))
	value := node.InputControlValue()
	if value == "" {
		node.TypeMismatch = false
		return false
	}
	switch typ {
	case "email":
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", "")
		if node.HasAttribute("multiple") {
			for _, address := range strings.Split(value, ",") {
				if !validHTMLEmailAddress(strings.Trim(address, "\t\n\f\r ")) {
					node.TypeMismatch = true
					return true
				}
			}
			node.TypeMismatch = false
			return false
		}
		value = strings.Trim(value, "\t\n\f\r ")
		node.TypeMismatch = !validHTMLEmailAddress(value)
		return node.TypeMismatch
	case "url":
		value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", "")
		value = strings.Trim(value, "\t\n\f\r ")
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" {
			node.TypeMismatch = true
			return true
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "ftp", "ws", "wss":
			node.TypeMismatch = parsed.Host == "" && parsed.Opaque == ""
		default:
			node.TypeMismatch = parsed.Opaque == "" && parsed.Host == "" && parsed.Path == ""
		}
		return node.TypeMismatch
	default:
		node.TypeMismatch = false
		return false
	}
}

func validHTMLEmailAddress(value string) bool {
	valid, err := regexp.MatchString(htmlEmailAddressPattern, value)
	return err == nil && valid
}

const htmlFloatingPointPattern = "^-?([0-9]+(\\.[0-9]+)?|\\.[0-9]+)([eE][+-]?[0-9]+)?$"

func parseHTMLFloatingPoint(value string) (float64, bool) {
	if matched, err := regexp.MatchString(htmlFloatingPointPattern, value); err != nil || !matched {
		return 0, false
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return 0, false
	}
	return number, true
}

func (r *Runtime) updateRangeValidity(node *dom.Node) {
	if node == nil {
		return
	}
	node.RangeUnderflow = false
	node.RangeOverflow = false
	node.StepMismatch = false
	node.RangeLimited = false
	if node.Data != "input" {
		return
	}
	typ := strings.ToLower(node.GetAttribute("type"))
	defaultStep := float64(1)
	stepScale := float64(1)
	defaultBase := float64(0)
	switch typ {
	case "number", "date", "month":
	case "week":
		stepScale = 7    // values are represented as day numbers
		defaultBase = -3 // Monday of 1970-W01
	case "time", "datetime-local":
		defaultStep = 60
		stepScale = 1000 // values are represented in milliseconds
	default:
		return
	}
	minimum, hasMinimum := parseInputRangeValue(typ, node.GetAttribute("min"))
	hasMinimum = hasMinimum && node.HasAttribute("min")
	maximum, hasMaximum := parseInputRangeValue(typ, node.GetAttribute("max"))
	hasMaximum = hasMaximum && node.HasAttribute("max")
	node.RangeLimited = hasMinimum || hasMaximum
	value, valid := parseInputRangeValue(typ, node.InputControlValue())
	if !valid {
		return
	}
	if hasMinimum {
		node.RangeUnderflow = value < minimum
	}
	if hasMaximum {
		node.RangeOverflow = value > maximum
	}
	if typ == "time" && hasMinimum && hasMaximum && minimum > maximum {
		inWrappedRange := value > maximum && value < minimum
		node.RangeUnderflow = inWrappedRange
		node.RangeOverflow = inWrappedRange
	}
	stepText := node.GetAttribute("step")
	if node.HasAttribute("step") && strings.EqualFold(stepText, "any") {
		return
	}
	step := defaultStep * stepScale
	if parsedStep, ok := parseHTMLFloatingPoint(stepText); node.HasAttribute("step") && ok && parsedStep > 0 {
		step = parsedStep * stepScale
	}
	base := defaultBase
	if hasMinimum {
		base = minimum
	} else if defaultValue, ok := parseInputRangeValue(typ, node.GetAttribute("value")); ok && node.HasAttribute("value") {
		base = defaultValue
	}
	quotient := (value - base) / step
	if typ == "number" {
		if mismatch, ok := exactNumberStepMismatch(node, stepText, hasMinimum); ok {
			node.StepMismatch = mismatch
			return
		}
	}
	node.StepMismatch = math.Abs(quotient-math.Round(quotient)) > 1e-9
}

// exactNumberStepMismatch avoids losing the fractional part of large step
// quotients. A number such as 17 divided by a very small step such as 3e-15
// exceeds float64's integer precision, even though the exact decimal quotient
// is not an integer.
func exactNumberStepMismatch(node *dom.Node, stepText string, hasMinimum bool) (bool, bool) {
	value, ok := new(big.Rat).SetString(node.InputControlValue())
	if !ok {
		return false, false
	}
	baseText := "0"
	if hasMinimum {
		baseText = node.GetAttribute("min")
	} else if node.HasAttribute("value") {
		if _, valid := parseHTMLFloatingPoint(node.GetAttribute("value")); valid {
			baseText = node.GetAttribute("value")
		}
	}
	base, ok := new(big.Rat).SetString(baseText)
	if !ok {
		return false, false
	}
	step := "1"
	if parsed, valid := parseHTMLFloatingPoint(stepText); node.HasAttribute("step") && valid && parsed > 0 {
		step = stepText
	}
	stepValue, ok := new(big.Rat).SetString(step)
	if !ok || stepValue.Sign() <= 0 {
		return false, false
	}
	quotient := new(big.Rat).Sub(value, base)
	quotient.Quo(quotient, stepValue)
	return !quotient.IsInt(), true
}

func parseInputRangeValue(inputType, value string) (float64, bool) {
	switch inputType {
	case "date":
		return parseHTMLDate(value)
	case "month":
		return parseHTMLMonth(value)
	case "week":
		return parseHTMLWeek(value)
	case "time":
		return parseHTMLTime(value)
	case "datetime-local":
		if !strings.Contains(value, "T") {
			if i := strings.IndexByte(value, ' '); i >= 0 {
				value = value[:i] + "T" + value[i+1:]
			}
		}
		dateText, timeText, found := strings.Cut(value, "T")
		if !found {
			return 0, false
		}
		days, validDate := parseHTMLDate(dateText)
		milliseconds, validTime := parseHTMLTime(timeText)
		if !validDate || !validTime {
			return 0, false
		}
		return days*86_400_000 + milliseconds, true
	}
	return parseHTMLFloatingPoint(value)
}

func parseHTMLMonth(value string) (float64, bool) {
	separator := strings.IndexByte(value, '-')
	if separator < 4 || separator != len(value)-3 {
		return 0, false
	}
	for i := 0; i < len(value); i++ {
		if i == separator {
			continue
		}
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	year, errYear := strconv.ParseInt(value[:separator], 10, 64)
	month, errMonth := strconv.ParseInt(value[separator+1:], 10, 64)
	if errYear != nil || errMonth != nil || year < 1 || month < 1 || month > 12 {
		return 0, false
	}
	return float64(year-1970)*12 + float64(month-1), true
}

func parseHTMLWeek(value string) (float64, bool) {
	separator := strings.Index(value, "-W")
	if separator < 4 || separator != len(value)-4 {
		return 0, false
	}
	for i := 0; i < separator; i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	if value[separator+2] < '0' || value[separator+2] > '9' || value[separator+3] < '0' || value[separator+3] > '9' {
		return 0, false
	}
	yearText := value[:separator]
	year, errYear := strconv.ParseInt(yearText, 10, 64)
	week, errWeek := strconv.ParseInt(value[separator+2:], 10, 64)
	if errYear != nil || errWeek != nil || year < 1 || week < 1 {
		return 0, false
	}
	jan1, valid := parseHTMLDate(yearText + "-01-01")
	jan4, validJan4 := parseHTMLDate(yearText + "-01-04")
	if !valid || !validJan4 {
		return 0, false
	}
	weekdayJan1 := int64(math.Mod(jan1+3, 7))
	if weekdayJan1 < 0 {
		weekdayJan1 += 7
	}
	leap := year%4 == 0 && (year%100 != 0 || year%400 == 0)
	weeksInYear := int64(52)
	if weekdayJan1 == 3 || (weekdayJan1 == 2 && leap) {
		weeksInYear = 53
	}
	if week > weeksInYear {
		return 0, false
	}
	weekdayJan4 := int64(math.Mod(jan4+3, 7))
	if weekdayJan4 < 0 {
		weekdayJan4 += 7
	}
	weekOneMonday := jan4 - float64(weekdayJan4)
	return weekOneMonday + float64((week-1)*7), true
}

func parseHTMLTime(value string) (float64, bool) {
	whole, fraction, hasFraction := strings.Cut(value, ".")
	if hasFraction && (len(fraction) == 0 || len(fraction) > 3) {
		return 0, false
	}
	parts := strings.Split(whole, ":")
	if len(parts) < 2 || len(parts) > 3 || (hasFraction && len(parts) != 3) {
		return 0, false
	}
	for _, part := range parts {
		if len(part) != 2 || part[0] < '0' || part[0] > '9' || part[1] < '0' || part[1] > '9' {
			return 0, false
		}
	}
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	second := 0
	if len(parts) == 3 {
		second, _ = strconv.Atoi(parts[2])
	}
	if hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}
	milliseconds := float64(((hour*60+minute)*60 + second) * 1000)
	if hasFraction {
		for i := 0; i < len(fraction); i++ {
			if fraction[i] < '0' || fraction[i] > '9' {
				return 0, false
			}
		}
		fractional, err := strconv.ParseFloat("0."+fraction, 64)
		if err != nil {
			return 0, false
		}
		milliseconds += fractional * 1000
	}
	return milliseconds, true
}

func parseHTMLDate(value string) (float64, bool) {
	if len(value) < 10 {
		return 0, false
	}
	yearEnd := len(value) - 6
	if yearEnd < 4 || value[yearEnd] != '-' || value[yearEnd+3] != '-' {
		return 0, false
	}
	for i := 0; i < len(value); i++ {
		if i == yearEnd || i == yearEnd+3 {
			continue
		}
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	year, errYear := strconv.ParseInt(value[:yearEnd], 10, 64)
	month, errMonth := strconv.ParseInt(value[yearEnd+1:yearEnd+3], 10, 64)
	day, errDay := strconv.ParseInt(value[yearEnd+4:], 10, 64)
	if errYear != nil || errMonth != nil || errDay != nil || year < 1 || month < 1 || month > 12 {
		return 0, false
	}
	monthDays := [...]int64{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		monthDays[1] = 29
	}
	if day < 1 || day > monthDays[month-1] {
		return 0, false
	}
	y := float64(year)
	if month <= 2 {
		y--
	}
	era := math.Floor(y / 400)
	yearOfEra := y - era*400
	monthPrime := month - 3
	if month <= 2 {
		monthPrime = month + 9
	}
	dayOfYear := (153*monthPrime+2)/5 + day - 1
	dayOfEra := yearOfEra*365 + math.Floor(yearOfEra/4) - math.Floor(yearOfEra/100) + float64(dayOfYear)
	daysSinceEpoch := era*146097 + dayOfEra - 719468
	return daysSinceEpoch, true
}

func compileHTMLInputPattern(pattern string) (*regexp.Regexp, bool) {
	// HTML compiles pattern with ECMAScript's Unicode Sets mode. Goja currently
	// exposes the Unicode (u) transformer, so translate the supported v-mode
	// string and set expressions before sending the remaining expression to
	// RE2. Invalid v-mode patterns are ignored by constraint validation.
	if translated, ok := translatePatternStringSet(pattern); ok {
		pattern = translated
	}
	if translated, ok := translateASCIISubtraction(pattern); ok {
		pattern = translated
	} else if containsPatternSetOperator(pattern) {
		return nil, false
	}
	if invalidUnicodeSetsClass(pattern) {
		return nil, false
	}
	pattern = strings.ReplaceAll(pattern, `\p{ASCII_Hex_Digit}`, `[0-9A-Fa-f]`)
	// Go's regexp engine lacks Unicode properties of strings. This translation
	// covers emoji presentation characters, flags, keycaps, and ZWJ sequences;
	// the Unicode RGI list still needs a generated table for exact membership.
	emoji := `(?:[0-9#*]\x{FE0F}?\x{20E3}|[\x{1F1E6}-\x{1F1FF}]{2}|[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}](?:\x{FE0F}|[\x{1F3FB}-\x{1F3FF}])?(?:\x{200D}[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}](?:\x{FE0F}|[\x{1F3FB}-\x{1F3FF}])?)*)`
	pattern = strings.ReplaceAll(pattern, `\p{RGI_Emoji}`, emoji)
	transformed, err := parser.TransformRegExp(pattern, false, true)
	if err != nil {
		return nil, false
	}
	compiled, err := regexp.Compile("^(" + transformed + ")$")
	return compiled, err == nil
}

func translatePatternStringSet(pattern string) (string, bool) {
	start := strings.Index(pattern, `\q{`)
	if start < 0 {
		return pattern, false
	}
	open := strings.LastIndex(pattern[:start], "[")
	close := strings.Index(pattern[start:], "}")
	if open < 0 || close < 0 {
		return pattern, false
	}
	close += start
	if close+1 >= len(pattern) || pattern[close+1] != ']' {
		return pattern, false
	}
	var alternatives []string
	for _, ch := range pattern[open+1 : start] {
		alternatives = append(alternatives, regexp.QuoteMeta(string(ch)))
	}
	for _, term := range strings.Split(pattern[start+3:close], "|") {
		if term == "" {
			return pattern, false
		}
		alternatives = append(alternatives, regexp.QuoteMeta(term))
	}
	for _, ch := range pattern[close+2:] {
		if ch == ']' {
			break
		}
		return pattern, false
	}
	return pattern[:open] + "(?:" + strings.Join(alternatives, "|") + ")" + pattern[close+2:], true
}

func translateASCIISubtraction(pattern string) (string, bool) {
	open := strings.Index(pattern, "[")
	operator := strings.Index(pattern, "--[")
	if open < 0 || operator < open {
		return pattern, false
	}
	leftEnd := operator
	rightStart := operator + 2
	rightEnd := strings.Index(pattern[rightStart:], "]")
	if rightEnd < 0 {
		return pattern, false
	}
	rightEnd += rightStart
	outerEnd := strings.Index(pattern[rightEnd+1:], "]")
	if outerEnd < 0 {
		return pattern, false
	}
	outerEnd += rightEnd + 1
	left := "[" + pattern[open+1:leftEnd] + "]"
	right := pattern[rightStart : rightEnd+1]
	left = strings.ReplaceAll(left, `\p{ASCII_Hex_Digit}`, `[0-9A-Fa-f]`)
	leftSet, leftErr := regexp.Compile(left)
	rightSet, rightErr := regexp.Compile(right)
	if leftErr != nil || rightErr != nil {
		return pattern, false
	}
	var members strings.Builder
	for ch := rune(0); ch < 128; ch++ {
		value := string(ch)
		if leftSet.MatchString(value) && !rightSet.MatchString(value) {
			if strings.ContainsRune(`\-]^`, ch) {
				members.WriteByte('\\')
			}
			members.WriteRune(ch)
		}
	}
	return pattern[:open] + "[" + members.String() + "]" + pattern[outerEnd+1:], true
}

func containsPatternSetOperator(pattern string) bool {
	inClass, escaped := false, false
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '[' {
			inClass = true
			continue
		}
		if ch == ']' {
			inClass = false
			continue
		}
		if inClass && i+1 < len(pattern) && (pattern[i:i+2] == "&&" || pattern[i:i+2] == "--") {
			return true
		}
	}
	return false
}

func invalidUnicodeSetsClass(pattern string) bool {
	inClass, escaped := false, false
	classStart := -1
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '[' {
			if inClass {
				return true
			}
			inClass, classStart = true, i
			continue
		}
		if ch == ']' && inClass {
			inClass = false
			classStart = -1
			continue
		}
		if !inClass {
			continue
		}
		if strings.ContainsRune("(){}/|", rune(ch)) || ch == '-' && (i == classStart+1 || i+1 >= len(pattern) || pattern[i+1] == ']') {
			return true
		}
		if i+1 < len(pattern) && pattern[i+1] == ch && strings.ContainsRune("!#$%&*+,.:;<=>?@^_`~", rune(ch)) {
			return true
		}
	}
	return inClass
}

func (r *Runtime) formControlValueMissing(node *dom.Node) bool {
	if node == nil || !node.HasAttribute("required") {
		return false
	}
	if node.Data == "input" {
		typ := strings.ToLower(node.GetAttribute("type"))
		switch typ {
		case "checkbox":
			return !node.InputChecked()
		case "radio":
			if node.GetAttribute("name") == "" {
				return false
			}
			return !r.radioGroupHasChecked(node)
		case "file":
			return len(r.inputFiles[node]) == 0
		case "hidden", "button", "reset", "submit", "image":
			return false
		}
		// For value-mode input states, the required constraint only applies
		// while the control is mutable. Checkbox, radio, and file states have
		// their own rules above, which can remain missing while disabled.
		if dom.IsFormControlDisabled(node) || node.HasAttribute("readonly") {
			return false
		}
		return node.InputControlValue() == ""
	}
	if node.Data == "textarea" {
		if dom.IsFormControlDisabled(node) || node.HasAttribute("readonly") {
			return false
		}
		return node.InputControlValue() == ""
	}
	if node.Data == "select" {
		return dom.SelectValueMissing(node)
	}
	return false
}

func (r *Runtime) checkNativeFormControlValidity(node *dom.Node) bool {
	if node == nil || !r.formControlWillValidate(node) {
		return true
	}
	r.updateRangeValidity(node)
	invalid := node.CustomValidityMessage != "" || r.inputTypeMismatch(node) || r.inputPatternMismatch(node) || node.RangeUnderflow || node.RangeOverflow || node.StepMismatch || dom.IsTooLong(node) || dom.IsTooShort(node) || r.formControlValueMissing(node)
	if invalid {
		dom.DispatchEvent(node, dom.NewEvent("invalid", false, true))
	}
	return !invalid
}

func (r *Runtime) formControlValidationMessage(node *dom.Node) string {
	if node == nil || !r.formControlWillValidate(node) {
		return ""
	}
	if node.CustomValidityMessage != "" {
		return node.CustomValidityMessage
	}
	if r.inputTypeMismatch(node) {
		return "Please enter a valid value."
	}
	r.updateRangeValidity(node)
	if node.RangeUnderflow {
		return "Value must be greater than or equal to the minimum."
	}
	if node.RangeOverflow {
		return "Value must be less than or equal to the maximum."
	}
	if node.StepMismatch {
		return "Value does not match the allowed step."
	}
	if r.inputPatternMismatch(node) {
		return "Please match the requested format."
	}
	if dom.IsTooLong(node) {
		return "The value is too long."
	}
	if dom.IsTooShort(node) {
		return "The value is too short."
	}
	if r.formControlValueMissing(node) {
		return "Please fill out this field."
	}
	return ""
}

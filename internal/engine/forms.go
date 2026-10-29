package engine

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// MaxFormControls bounds the number of controls per form.
const MaxFormControls = 256

// MaxFormDataBytes bounds the encoded form data payload.
const MaxFormDataBytes = 1 << 20 // 1 MiB

// FormData is a single name/value pair from a form control.
type FormData struct {
	Name  string
	Value string
}

// CollectFormData gathers successful controls from a form element.
// It walks the form's subtree in tree order and collects name/value pairs
// from input, select, textarea, and button elements.
// Disabled controls and controls without a name attribute are skipped.
func CollectFormData(form, submitter *dom.Node) []FormData {
	if form == nil {
		return nil
	}
	var data []FormData
	collectFromNode(form, form, submitter, &data)
	return data
}

func collectFromNode(n, form, submitter *dom.Node, data *[]FormData) {
	if len(*data) >= MaxFormControls {
		return
	}
	if n == submitter {
		if isSubmitButton(n, form) && n.GetAttribute("name") != "" {
			*data = append(*data, FormData{Name: n.GetAttribute("name"), Value: controlSubmissionValue(n)})
		}
	} else if n.Element() && isSuccessfulControl(n, form) {
		name := n.GetAttribute("name")
		if name != "" {
			if n.Data == "select" {
				for _, option := range dom.SelectOptions(n) {
					if len(*data) >= MaxFormControls {
						break
					}
					if option.OptionSelected() && !option.OptionDisabled() {
						*data = append(*data, FormData{Name: name, Value: optionValue(option)})
					}
				}
			} else {
				value := controlSubmissionValue(n)
				*data = append(*data, FormData{Name: name, Value: value})
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectFromNode(c, form, submitter, data)
	}
}

func isSubmitButton(n, form *dom.Node) bool {
	if n == nil || !n.Element() || dom.IsFormControlDisabled(n) || FindForm(n) != form {
		return false
	}
	switch n.Data {
	case "input":
		return strings.EqualFold(n.GetAttribute("type"), "submit")
	case "button":
		typ := strings.ToLower(n.GetAttribute("type"))
		return typ == "" || typ == "submit"
	default:
		return false
	}
}

// FirstSubmitButton returns the first enabled submit button in tree order.
func FirstSubmitButton(form *dom.Node) *dom.Node {
	if form == nil {
		return nil
	}
	var found *dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n == nil || found != nil {
			return
		}
		if isSubmitButton(n, form) {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)
	return found
}

// ValidateForm returns invalid controls in tree order. This intentionally
// implements only required, email, and number constraints.
func ValidateForm(form *dom.Node) []*dom.Node {
	if form == nil || form.HasAttribute("novalidate") {
		return nil
	}
	var controls, invalid []*dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && (n.Data == "input" || n.Data == "textarea" || n.Data == "select") &&
			!dom.IsFormControlDisabled(n) {
			controls = append(controls, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)

	invalidSet := make(map[*dom.Node]bool)
	radioGroups := make(map[string][]*dom.Node)
	for _, n := range controls {
		if n.Data == "input" && strings.EqualFold(n.GetAttribute("type"), "radio") {
			radioGroups[n.GetAttribute("name")] = append(radioGroups[n.GetAttribute("name")], n)
		}
	}
	for _, n := range controls {
		value := controlValue(n)
		typ := ""
		if n.Data == "input" {
			typ = strings.ToLower(n.GetAttribute("type"))
		}
		if n.HasAttribute("required") {
			empty := value == ""
			switch typ {
			case "checkbox":
				empty = !n.InputChecked()
			case "radio":
				group := radioGroups[n.GetAttribute("name")]
				checked := false
				for _, member := range group {
					checked = checked || member.InputChecked()
				}
				if !checked && len(group) > 0 {
					invalidSet[group[0]] = true
				}
				empty = false
			case "":
				if n.Data == "select" {
					empty = dom.SelectValueMissing(n)
				}
			}
			if empty {
				invalidSet[n] = true
			}
		}
		if value != "" && typ == "email" && !strings.Contains(value, "@") {
			invalidSet[n] = true
		}
		if value != "" && typ == "number" {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || parsed != parsed || parsed > 1.7976931348623157e308 || parsed < -1.7976931348623157e308 {
				invalidSet[n] = true
			}
		}
	}
	for _, n := range controls {
		if invalidSet[n] {
			invalid = append(invalid, n)
		}
	}
	return invalid
}

// isSuccessfulControl reports whether n is a successful control per HTML spec.
// A successful control is one that is not disabled and has a name attribute.
func isSuccessfulControl(n, form *dom.Node) bool {
	if !n.Element() {
		return false
	}
	// Check disabled
	if dom.IsFormControlDisabled(n) {
		return false
	}
	// Check name exists
	if !n.HasAttribute("name") {
		return false
	}

	switch n.Data {
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		switch typ {
		case "submit", "button", "reset":
			return false // submitter is added at its tree-order position
		case "checkbox", "radio":
			// Only successful if checked
			return n.InputChecked()
		case "file":
			return false // not supported yet
		case "image":
			return false // not supported yet
		default:
			return true // text, password, hidden, email, etc.
		}
	case "textarea":
		return true
	case "select":
		return true // value is the selected option
	case "button":
		return false // submitter is added at its tree-order position
	}
	return false
}

// controlValue returns the value of a form control.
func controlValue(n *dom.Node) string {
	switch n.Data {
	case "input":
		return n.InputControlValue()
	case "textarea":
		return n.InputControlValue()
	case "select":
		// The same rule the keyboard and the accessibility tree use, so a
		// submitted value and an announced value cannot diverge.
		if option := SelectedOption(n); option != nil {
			return optionValue(option)
		}
		return ""
	case "button":
		return n.GetAttribute("value")
	}
	return ""
}

func controlSubmissionValue(n *dom.Node) string {
	if n != nil && n.Data == "textarea" {
		return n.TextareaSubmissionValue()
	}
	return controlValue(n)
}

// EncodeFormURLEncoded encodes form data as application/x-www-form-urlencoded.
func EncodeFormURLEncoded(data []FormData) string {
	var buf strings.Builder
	for i, d := range data {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(url.QueryEscape(normalizeFormLineBreaks(d.Name)))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(normalizeFormLineBreaks(d.Value)))
		if buf.Len() > MaxFormDataBytes {
			break
		}
	}
	return buf.String()
}

func normalizeFormLineBreaks(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}

// FormMethod returns the HTTP method for a form (GET, POST, or dialog).
func FormMethod(form *dom.Node) string {
	method := form.GetAttribute("method")
	switch strings.ToLower(method) {
	case "post":
		return "POST"
	case "dialog":
		return "dialog"
	default:
		return "GET"
	}
}

// FormAction returns the action URL for a form.
// Returns empty string if no action is specified.
func FormAction(form *dom.Node) string {
	return form.GetAttribute("action")
}

// FormEnctype returns the encoding type for a form.
func FormEnctype(form *dom.Node) string {
	enc := form.GetAttribute("enctype")
	switch strings.ToLower(enc) {
	case "multipart/form-data":
		return "multipart/form-data"
	case "text/plain":
		return "text/plain"
	default:
		return "application/x-www-form-urlencoded"
	}
}

// FindForm returns the ancestor form element for a control, or nil.
func FindForm(control *dom.Node) *dom.Node {
	for p := control.Parent; p != nil; p = p.Parent {
		if p.Element() && p.Data == "form" {
			return p
		}
	}
	return nil
}

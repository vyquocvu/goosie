package engine

import (
	"net/url"
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
func CollectFormData(form *dom.Node) []FormData {
	if form == nil {
		return nil
	}
	var data []FormData
	collectFromNode(form, form, &data)
	return data
}

func collectFromNode(n, form *dom.Node, data *[]FormData) {
	if len(*data) >= MaxFormControls {
		return
	}
	if n.Element() && isSuccessfulControl(n, form) {
		name := n.GetAttribute("name")
		if name != "" {
			value := controlValue(n)
			*data = append(*data, FormData{Name: name, Value: value})
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectFromNode(c, form, data)
	}
}

// isSuccessfulControl reports whether n is a successful control per HTML spec.
// A successful control is one that is not disabled and has a name attribute.
func isSuccessfulControl(n, form *dom.Node) bool {
	if !n.Element() {
		return false
	}
	// Check disabled
	if n.HasAttribute("disabled") {
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
			// Only successful if used to submit the form
			return false // handled separately
		case "checkbox", "radio":
			// Only successful if checked
			return n.HasAttribute("checked")
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
		return false // only successful if used to submit
	}
	return false
}

// controlValue returns the value of a form control.
func controlValue(n *dom.Node) string {
	switch n.Data {
	case "input":
		return n.GetAttribute("value")
	case "textarea":
		// Value is the text content
		return n.TextContent()
	case "select":
		// Value is the selected option's value (or text content)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() && c.Data == "option" {
				if c.HasAttribute("selected") {
					if c.HasAttribute("value") {
						return c.GetAttribute("value")
					}
					return c.TextContent()
				}
			}
		}
		// Default: first option
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() && c.Data == "option" {
				if c.HasAttribute("value") {
					return c.GetAttribute("value")
				}
				return c.TextContent()
			}
		}
		return ""
	}
	return ""
}

// EncodeFormURLEncoded encodes form data as application/x-www-form-urlencoded.
func EncodeFormURLEncoded(data []FormData) string {
	var buf strings.Builder
	for i, d := range data {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(url.QueryEscape(d.Name))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(d.Value))
		if buf.Len() > MaxFormDataBytes {
			break
		}
	}
	return buf.String()
}

// FormMethod returns the HTTP method for a form (GET or POST).
func FormMethod(form *dom.Node) string {
	method := form.GetAttribute("method")
	switch strings.ToLower(method) {
	case "post":
		return "POST"
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

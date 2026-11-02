package engine

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// MaxFormControls bounds the number of controls per form.
const MaxFormControls = 256

// MaxFormDataBytes bounds the encoded form data payload.
const MaxFormDataBytes = 1 << 20 // 1 MiB

// FormFile is a file selected by an <input type="file"> when the submission
// was assembled.
type FormFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

// FormData is a single name/value pair from a form control. A file control
// contributes one entry per selected file with File set; Value carries the
// filename so URL-encoded and text/plain submissions stay correct.
type FormData struct {
	Name  string
	Value string
	File  *FormFile
}

// FileSource returns the files selected on a file input, in FileList order.
type FileSource func(*dom.Node) []FormFile

// CollectFormData gathers successful controls from a form element.
// It walks the form's subtree in tree order and collects name/value pairs
// from input, select, textarea, and button elements.
// Disabled controls and controls without a name attribute are skipped.
func CollectFormData(form, submitter *dom.Node) []FormData {
	return collectFormData(form, submitter, nil)
}

func collectFormData(form, submitter *dom.Node, files FileSource) []FormData {
	if form == nil {
		return nil
	}
	var data []FormData
	collectFromNode(form, form, submitter, files, &data)
	return data
}

func collectFromNode(n, form, submitter *dom.Node, files FileSource, data *[]FormData) {
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
			switch {
			case n.Data == "select":
				for _, option := range dom.SelectOptions(n) {
					if len(*data) >= MaxFormControls {
						break
					}
					if option.OptionSelected() && !option.OptionDisabled() {
						*data = append(*data, FormData{Name: name, Value: optionValue(option)})
					}
				}
			case isFileInput(n):
				// A file control always contributes at least one entry; with no
				// files selected it is an empty filename part.
				var selected []FormFile
				if files != nil {
					selected = files(n)
				}
				if len(selected) == 0 {
					*data = append(*data, FormData{Name: name, File: &FormFile{}})
				}
				for i := range selected {
					file := selected[i]
					*data = append(*data, FormData{Name: name, Value: file.Filename, File: &file})
				}
			default:
				value := controlSubmissionValue(n)
				*data = append(*data, FormData{Name: name, Value: value})
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectFromNode(c, form, submitter, files, data)
	}
}

func isFileInput(n *dom.Node) bool {
	return n.Data == "input" && strings.EqualFold(n.GetAttribute("type"), "file")
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
	return validateForm(form, nil)
}

func validateForm(form *dom.Node, files FileSource) []*dom.Node {
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
			case "file":
				empty = files == nil || len(files(n)) == 0
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
			return true
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

// EncodeFormPlainText encodes form data as text/plain: each entry is written as
// name=value followed by CRLF, with line breaks normalized and nothing escaped.
// A value containing "=" or "&" is therefore deliberately lossy, as HTML defines.
func EncodeFormPlainText(data []FormData) string {
	var buf strings.Builder
	for _, d := range data {
		buf.WriteString(normalizeFormLineBreaks(d.Name))
		buf.WriteByte('=')
		buf.WriteString(normalizeFormLineBreaks(d.Value))
		buf.WriteString("\r\n")
		if buf.Len() > MaxFormDataBytes {
			break
		}
	}
	return buf.String()
}

// EncodeFormMultipart encodes form data as multipart/form-data and returns the
// boundary the caller must append to the Content-Type header. File entries carry
// their filename, content type and bytes; text entries are written as UTF-8.
// The payload is capped like every other form encoding.
func EncodeFormMultipart(data []FormData) (string, []byte, error) {
	var token [18]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", nil, fmt.Errorf("could not create multipart boundary: %w", err)
	}
	boundary := "----GoosieFormBoundary" + hex.EncodeToString(token[:])
	closing := len(boundary) + 6 // "--" + boundary + "--\r\n"
	var body bytes.Buffer
	for _, d := range data {
		var header bytes.Buffer
		header.WriteString("--" + boundary + "\r\nContent-Disposition: form-data; name=\"")
		header.WriteString(escapeMultipartValue(normalizeFormLineBreaks(d.Name)))
		header.WriteByte('"')
		var payload []byte
		if d.File == nil {
			header.WriteString("\r\n\r\n")
			payload = []byte(normalizeFormLineBreaks(d.Value))
		} else {
			header.WriteString("; filename=\"")
			header.WriteString(escapeMultipartValue(d.File.Filename))
			header.WriteString("\"\r\nContent-Type: ")
			header.WriteString(cleanMultipartContentType(d.File.ContentType))
			header.WriteString("\r\n\r\n")
			payload = d.File.Data
		}
		// Check before copying so an oversized file never reaches the buffer.
		if body.Len()+header.Len()+len(payload)+2+closing > MaxFormDataBytes {
			return "", nil, fmt.Errorf("multipart form data exceeds %d bytes", MaxFormDataBytes)
		}
		body.Write(header.Bytes())
		body.Write(payload)
		body.WriteString("\r\n")
	}
	body.WriteString("--" + boundary + "--\r\n")
	return boundary, body.Bytes(), nil
}

func escapeMultipartValue(value string) string {
	return strings.NewReplacer("\r", "%0D", "\n", "%0A", "\"", "%22").Replace(value)
}

func cleanMultipartContentType(value string) string {
	value = strings.NewReplacer("\r", "", "\n", "").Replace(strings.TrimSpace(value))
	if value == "" {
		return "application/octet-stream"
	}
	return value
}

// fileSource returns the live file selections, or nil without a scripting
// runtime. File selections belong to the runtime's File objects, so a document
// with scripts disabled can only contribute an empty file part.
func (s *Session) fileSource() FileSource {
	if s.jsRT == nil {
		return nil
	}
	return func(n *dom.Node) []FormFile {
		selected := s.jsRT.SelectedFiles(n)
		if len(selected) == 0 {
			return nil
		}
		files := make([]FormFile, len(selected))
		for i, file := range selected {
			files[i] = FormFile{Filename: file.Name, ContentType: file.ContentType, Data: file.Data}
		}
		return files
	}
}

// collectFormData gathers submission entries including the files currently
// selected on file controls.
func (s *Session) collectFormData(form, submitter *dom.Node) []FormData {
	return collectFormData(form, submitter, s.fileSource())
}

func (s *Session) validateForm(form *dom.Node) []*dom.Node {
	return validateForm(form, s.fileSource())
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

// isSubmitter reports whether n is a submit button. The form* override
// attributes are honored only on these elements, never on a plain button.
func isSubmitter(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	switch n.Data {
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		return typ == "submit" || typ == "image"
	case "button":
		typ := strings.ToLower(n.GetAttribute("type"))
		return typ == "" || typ == "submit"
	default:
		return false
	}
}

func submitterAttr(submitter *dom.Node, name string) (string, bool) {
	if !isSubmitter(submitter) || !submitter.HasAttribute(name) {
		return "", false
	}
	return submitter.GetAttribute(name), true
}

// FormMethodFor resolves the method of a submission. A submit button's
// formmethod wins over the form's method when it names get, post, or dialog;
// any other value leaves the form's method in force.
func FormMethodFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formmethod"); ok {
		switch strings.ToLower(value) {
		case "get":
			return "GET"
		case "post":
			return "POST"
		case "dialog":
			return "dialog"
		}
	}
	return FormMethod(form)
}

// FormAction returns the action URL for a form.
// Returns empty string if no action is specified.
func FormAction(form *dom.Node) string {
	return form.GetAttribute("action")
}

// FormActionFor resolves the action of a submission. A submit button's
// formaction wins over the form's action, including an explicitly empty value,
// which the host resolves to the current document URL.
func FormActionFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formaction"); ok {
		return value
	}
	return FormAction(form)
}

// SubmitterNoValidate reports whether the activated submit button opted out of
// constraint validation with formnovalidate.
func SubmitterNoValidate(submitter *dom.Node) bool {
	return isSubmitter(submitter) && submitter.HasAttribute("formnovalidate")
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

// FormEnctypeFor resolves the encoding of a submission. A submit button's
// formenctype wins over the form's enctype when it names one of the three
// supported encodings; otherwise the form's enctype applies.
func FormEnctypeFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formenctype"); ok {
		switch strings.ToLower(value) {
		case "application/x-www-form-urlencoded", "multipart/form-data", "text/plain":
			return strings.ToLower(value)
		}
	}
	return FormEnctype(form)
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

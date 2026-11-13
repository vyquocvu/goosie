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
	return isSubmitButtonInAnyState(n, form) && !dom.IsFormControlDisabled(n)
}

func isSubmitButtonInAnyState(n, form *dom.Node) bool {
	if n == nil || !n.Element() || FindForm(n) != form {
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

// FirstSubmitButton returns the first submit button in tree order, disabled or
// not. Implicit submission picks its submitter this way: a disabled button
// stays the chosen submitter and blocks the submission rather than handing the
// job to the next button (measured in Chromium 131.0.6778.33 with /tmp/v2/run4.py
// and /tmp/v2/run5b.py, 2026-11-13: Enter in a form whose first submit button
// is disabled records no navigation at all, and removing that button lets the
// same Enter submit `/s2?t2=t2v`).
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
		if isSubmitButtonInAnyState(n, form) {
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

// disabledFormControlAt returns the nearest disabled form control at or above
// n, or nil. Only the disableable controls suppress pointer input: a disabled
// fieldset and a stray `disabled` attribute on a non-control both keep their
// clicks (/tmp/v2/run7.py, 2026-11-13: `<fieldset disabled>` -> `click:fs2`,
// `<div disabled>` -> `click:ddiv`).
func disabledFormControlAt(n *dom.Node) *dom.Node {
	for cur := n; cur != nil; cur = cur.Parent {
		if dom.IsDisableableFormControl(cur) && dom.IsFormControlDisabled(cur) {
			return cur
		}
	}
	return nil
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

// constraintValidationMessage names the constraint that made n invalid, using
// the strings browsers show in their validation bubble. The caller only passes
// controls validateForm already rejected, so the fallback covers a required
// control with no more specific wording.
func constraintValidationMessage(n *dom.Node) string {
	if n == nil || !n.Element() {
		return ""
	}
	typ := ""
	if n.DataAtom == dom.AtomInput {
		typ = strings.ToLower(n.GetAttribute("type"))
	}
	if n.HasAttribute("required") {
		missing := controlValue(n) == ""
		switch typ {
		case "checkbox":
			missing = !n.InputChecked()
		case "radio":
			missing = !n.InputChecked()
		case "file":
			missing = true
		case "":
			if n.DataAtom == dom.AtomSelect {
				missing = dom.SelectValueMissing(n)
			}
		}
		if missing {
			switch typ {
			case "checkbox":
				return "Please check this box if you want to proceed."
			case "radio":
				return "Please select one of these options."
			case "file":
				return "Please select a file."
			}
			if n.DataAtom == dom.AtomSelect {
				return "Please select an item in the list."
			}
			return "Please fill out this field."
		}
	}
	value := controlValue(n)
	if typ == "email" && value != "" && !strings.Contains(value, "@") {
		return "Please include an '@' in the email address."
	}
	if typ == "number" && value != "" {
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return "Please enter a number."
		}
	}
	return "Please fill out this field."
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

// FormTarget returns the browsing context name a form submits into. The empty
// string means the form's own context, like _self.
func FormTarget(form *dom.Node) string {
	return strings.TrimSpace(form.GetAttribute("target"))
}

// FormTargetFor resolves the target with the submitter's formtarget override.
// Like formaction and formmethod, the attribute only applies to a submit button.
func FormTargetFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formtarget"); ok {
		return strings.TrimSpace(value)
	}
	return FormTarget(form)
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

// formForControl resolves a control's form, honouring the form attribute
// before the ancestor chain.
func formForControl(n *dom.Node) *dom.Node {
	if n == nil {
		return nil
	}
	if id := n.GetAttribute("form"); id != "" && n.Doc != nil {
		if owner := n.Doc.ElementByID(id); owner != nil && owner.Data == "form" {
			return owner
		}
	}
	return FindForm(n)
}

// noteControlBlur records that the user left a form control. Constraint
// validation exposes :user-invalid/:user-valid once focus moves away, so a
// required field reveals its error by tabbing out even without a submission.
func (s *Session) noteControlBlur(n *dom.Node) {
	if n == nil || !n.Element() {
		return
	}
	switch n.Data {
	case "input":
		switch strings.ToLower(n.GetAttribute("type")) {
		case "hidden", "button", "reset", "submit", "image":
			return
		}
	case "textarea", "select":
	default:
		return
	}
	n.UserInteracted = true
	form := formForControl(n)
	if form == nil {
		return
	}
	// Promote the form only when the control that lost focus is itself
	// invalid; leaving a valid field must not light up an untouched error in
	// the same form.
	for _, invalid := range s.validateForm(form) {
		if invalid == n {
			form.UserInteracted = true
			return
		}
	}
}

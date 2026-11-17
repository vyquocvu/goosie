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

const MaxFormControls = 256

const MaxFormDataBytes = 1 << 20

type FormFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

type FormData struct {
	Name  string
	Value string
	File  *FormFile
}

type FileSource func(*dom.Node) []FormFile

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

func disabledFormControlAt(n *dom.Node) *dom.Node {
	for cur := n; cur != nil; cur = cur.Parent {
		if !dom.IsDisableableFormControl(cur) {
			continue
		}
		if dom.IsFormControlDisabled(cur) {
			return cur
		}
		return nil
	}
	return nil
}

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

func isSuccessfulControl(n, form *dom.Node) bool {
	if !n.Element() {
		return false
	}
	if dom.IsFormControlDisabled(n) {
		return false
	}
	if !n.HasAttribute("name") {
		return false
	}

	switch n.Data {
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		switch typ {
		case "submit", "button", "reset":
			return false
		case "checkbox", "radio":
			return n.InputChecked()
		case "file":
			return true
		case "image":
			return false
		default:
			return true
		}
	case "textarea":
		return true
	case "select":
		return true
	case "button":
		return false
	}
	return false
}

func controlValue(n *dom.Node) string {
	switch n.Data {
	case "input":
		return n.InputControlValue()
	case "textarea":
		return n.InputControlValue()
	case "select":
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

func EncodeFormMultipart(data []FormData) (string, []byte, error) {
	var token [18]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", nil, fmt.Errorf("could not create multipart boundary: %w", err)
	}
	boundary := "----GoosieFormBoundary" + hex.EncodeToString(token[:])
	closing := len(boundary) + 6
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

func (s *Session) collectFormData(form, submitter *dom.Node) []FormData {
	return collectFormData(form, submitter, s.fileSource())
}

func (s *Session) validateForm(form *dom.Node) []*dom.Node {
	return validateForm(form, s.fileSource())
}

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

func FormAction(form *dom.Node) string {
	return form.GetAttribute("action")
}

func FormTarget(form *dom.Node) string {
	return strings.TrimSpace(form.GetAttribute("target"))
}

func FormTargetFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formtarget"); ok {
		return strings.TrimSpace(value)
	}
	return FormTarget(form)
}

func FormActionFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formaction"); ok {
		return value
	}
	return FormAction(form)
}

func SubmitterNoValidate(submitter *dom.Node) bool {
	return isSubmitter(submitter) && submitter.HasAttribute("formnovalidate")
}

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

func FormEnctypeFor(form, submitter *dom.Node) string {
	if value, ok := submitterAttr(submitter, "formenctype"); ok {
		switch strings.ToLower(value) {
		case "application/x-www-form-urlencoded", "multipart/form-data", "text/plain":
			return strings.ToLower(value)
		}
	}
	return FormEnctype(form)
}

func FindForm(control *dom.Node) *dom.Node {
	for p := control.Parent; p != nil; p = p.Parent {
		if p.Element() && p.Data == "form" {
			return p
		}
	}
	return nil
}

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
	for _, invalid := range s.validateForm(form) {
		if invalid == n {
			form.UserInteracted = true
			return
		}
	}
}

func (s *Session) activateLabeledControl(hit *dom.Node, x, y float32) bool {
	label := labelForPointer(hit)
	if label == nil {
		return false
	}
	control := labeledControl(label)
	if control == nil || dom.IsFormControlDisabled(control) {
		return false
	}
	s.FocusNode(control)
	ev := newPointerClickEvent(x, y, 0)
	if !dom.DispatchEvent(control, ev) {
		return true
	}
	s.runClickDefault(control)
	return true
}

func labelForPointer(target *dom.Node) *dom.Node {
	if target == nil {
		return nil
	}
	for label := target; label != nil; label = label.Parent {
		if !label.Element() || label.Data != "label" {
			continue
		}
		for node := target; node != nil && node != label; node = node.Parent {
			if !node.Element() {
				continue
			}
			switch node.Data {
			case "button", "input", "select", "summary", "textarea":
				return nil
			case "a", "area", "audio", "video":
				if node.HasAttribute("href") || node.HasAttribute("controls") {
					return nil
				}
			}
		}
		return label
	}
	return nil
}

func labeledControl(label *dom.Node) *dom.Node {
	if label == nil || !label.Element() || label.Data != "label" || label.Doc == nil {
		return nil
	}
	if label.HasAttribute("for") {
		id := label.GetAttribute("for")
		if id == "" {
			return nil
		}
		target := label.Doc.ElementByID(id)
		if labelableControl(target) {
			return target
		}
		return nil
	}
	var found *dom.Node
	var visit func(*dom.Node) bool
	visit = func(node *dom.Node) bool {
		for ; node != nil; node = node.NextSibling {
			if node.Element() && labelableControl(node) {
				found = node
				return false
			}
			if node.Data != "template" && !visit(node.FirstChild) {
				return false
			}
		}
		return true
	}
	visit(label.FirstChild)
	return found
}

func labelableControl(node *dom.Node) bool {
	if node == nil || !node.Element() {
		return false
	}
	switch node.Data {
	case "button", "meter", "output", "progress", "select", "textarea":
		return true
	case "input":
		return !strings.EqualFold(node.GetAttribute("type"), "hidden")
	default:
		return false
	}
}

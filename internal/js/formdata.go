package js

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
	"golang.org/x/net/html/charset"
)

// FormData API implementation.

// formDataEntry represents one entry in a FormData object.
type formDataEntry struct {
	name    string
	value   string
	file    *goja.Object
	charset bool
}

type formDataList struct {
	entries []formDataEntry
}

// setupFormData installs the FormData constructor.
func (r *Runtime) setupFormData() {
	ctor := func(call goja.ConstructorCall) *goja.Object {
		list := &formDataList{}
		var form *dom.Node
		var submitter *dom.Node
		if len(call.Arguments) > 0 {
			if !goja.IsNull(call.Arguments[0]) && !goja.IsUndefined(call.Arguments[0]) {
				form = r.getNode(call.Arguments[0])
				if form == nil || !form.Element() || form.Data != "form" {
					panic(r.vm.NewTypeError("FormData constructor argument must be a form element"))
				}
			}
		}
		if form != nil && len(call.Arguments) > 1 && !goja.IsNull(call.Arguments[1]) && !goja.IsUndefined(call.Arguments[1]) {
			submitter = r.getNode(call.Arguments[1])
			if !isSubmitButton(submitter) || r.formOwner(submitter) != form && !formHasListedControl(r, form, submitter) {
				panic(r.vm.NewTypeError("FormData submitter must be a submit button owned by the form"))
			}
		}
		obj := r.newFormDataObject(list)
		if form != nil {
			r.constructFormEntryList(form, submitter, "UTF-8", list, obj)
		}
		return obj
	}

	val := r.vm.ToValue(ctor)
	_ = r.vm.Set("FormData", val)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("FormData", val)
		}
	}
}

func (r *Runtime) newFormDataObject(list *formDataList) *goja.Object {
	obj := r.vm.NewObject()
	r.formDataValues[obj] = list

	// append(name, value)
	_ = obj.Set("append", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		name := call.Arguments[0].String()
		filename := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			filename = call.Argument(2).String()
		}
		entry := r.makeFormDataEntry(name, call.Argument(1), filename)
		list.entries = append(list.entries, entry)
		return goja.Undefined()
	})

	// set(name, value)
	_ = obj.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		name := call.Arguments[0].String()
		entry := r.makeFormDataEntry(name, call.Argument(1), "")
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			entry = r.makeFormDataEntry(name, call.Argument(1), call.Argument(2).String())
		}

		// Remove all existing entries with this name.
		filtered := make([]formDataEntry, 0, len(list.entries))
		for _, e := range list.entries {
			if e.name != name {
				filtered = append(filtered, e)
			}
		}
		filtered = append(filtered, entry)
		list.entries = filtered
		return goja.Undefined()
	})

	// get(name)
	_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		name := call.Arguments[0].String()
		for _, e := range list.entries {
			if e.name == name {
				return r.formDataValue(e)
			}
		}
		return goja.Null()
	})

	// getAll(name)
	_ = obj.Set("getAll", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.vm.NewArray()
		}
		name := call.Arguments[0].String()
		arr := r.vm.NewArray()
		i := 0
		for _, e := range list.entries {
			if e.name == name {
				_ = arr.Set(fmt.Sprintf("%d", i), r.formDataValue(e))
				i++
			}
		}
		return arr
	})

	// has(name)
	_ = obj.Set("has", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.vm.ToValue(false)
		}
		name := call.Arguments[0].String()
		for _, e := range list.entries {
			if e.name == name {
				return r.vm.ToValue(true)
			}
		}
		return r.vm.ToValue(false)
	})

	// delete(name)
	_ = obj.Set("delete", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		name := call.Arguments[0].String()
		filtered := make([]formDataEntry, 0, len(list.entries))
		for _, e := range list.entries {
			if e.name != name {
				filtered = append(filtered, e)
			}
		}
		list.entries = filtered
		return goja.Undefined()
	})

	// forEach(callback)
	_ = obj.Set("forEach", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		fn, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			return goja.Undefined()
		}
		for _, e := range list.entries {
			_, _ = fn(goja.Undefined(), r.formDataValue(e), r.vm.ToValue(e.name), obj)
		}
		return goja.Undefined()
	})

	// entries()
	_ = obj.Set("entries", func(call goja.FunctionCall) goja.Value {
		arr := r.vm.NewArray()
		i := 0
		for _, e := range list.entries {
			entry := r.vm.NewArray()
			_ = entry.Set("0", e.name)
			_ = entry.Set("1", r.formDataValue(e))
			_ = arr.Set(fmt.Sprintf("%d", i), entry)
			i++
		}
		return arr
	})

	// keys()
	_ = obj.Set("keys", func(call goja.FunctionCall) goja.Value {
		arr := r.vm.NewArray()
		i := 0
		for _, e := range list.entries {
			_ = arr.Set(fmt.Sprintf("%d", i), e.name)
			i++
		}
		return arr
	})

	// values()
	_ = obj.Set("values", func(call goja.FunctionCall) goja.Value {
		arr := r.vm.NewArray()
		i := 0
		for _, e := range list.entries {
			_ = arr.Set(fmt.Sprintf("%d", i), r.formDataValue(e))
			i++
		}
		return arr
	})

	_ = obj.SetSymbol(goja.SymIterator, func(call goja.FunctionCall) goja.Value {
		index := 0
		iterator := r.vm.NewObject()
		_ = iterator.Set("next", func(goja.FunctionCall) goja.Value {
			result := r.vm.NewObject()
			if index >= len(list.entries) {
				_ = result.Set("done", true)
				_ = result.Set("value", goja.Undefined())
				return result
			}
			entry := list.entries[index]
			index++
			pair := r.vm.NewArray(entry.name, r.formDataValue(entry))
			_ = result.Set("done", false)
			_ = result.Set("value", pair)
			return result
		})
		_ = iterator.SetSymbol(goja.SymIterator, func(goja.FunctionCall) goja.Value { return iterator })
		return iterator
	})
	return obj
}

func (r *Runtime) makeFormDataEntry(name string, value goja.Value, filename string) formDataEntry {
	if object, ok := value.(*goja.Object); ok {
		if file := r.fileValues[object]; file != nil {
			if file.isFile && filename == "" {
				return formDataEntry{name: name, file: object}
			}
			if filename == "" {
				filename = "blob"
			}
			return formDataEntry{name: name, file: r.makeBlob(r.vm.NewObject(), file.data, file.typeValue, true, filename, timeNowMillis())}
		}
	}
	return formDataEntry{name: name, value: value.String()}
}

func (r *Runtime) formDataValue(entry formDataEntry) goja.Value {
	if entry.file != nil {
		return entry.file
	}
	return r.vm.ToValue(entry.value)
}

func (r *Runtime) setFormDataCharset(list *formDataList, encoding string) {
	for i := range list.entries {
		if list.entries[i].charset {
			list.entries[i].value = encoding
		}
	}
}

func (r *Runtime) dispatchFormDataEvent(form *dom.Node, formData *goja.Object) {
	event := dom.NewFormDataEvent("formdata", true, false, formData)
	dom.DispatchEvent(form, &event.Event)
}

func (r *Runtime) constructFormEntryList(form, submitter *dom.Node, encoding string, list *formDataList, formData *goja.Object) {
	if form == nil || list == nil || r.constructingFormEntryList[form] {
		return
	}
	if r.constructingFormEntryList == nil {
		r.constructingFormEntryList = make(map[*dom.Node]bool)
	}
	r.constructingFormEntryList[form] = true
	defer delete(r.constructingFormEntryList, form)
	list.entries = r.formDataEntries(form, submitter)
	r.setFormDataCharset(list, encoding)
	if formData != nil {
		r.dispatchFormDataEvent(form, formData)
	}
}

func (r *Runtime) formDataForSubmission(form, submitter *dom.Node, encoding string) *formDataList {
	list := &formDataList{}
	formData := r.newFormDataObject(list)
	r.constructFormEntryList(form, submitter, encoding, list, formData)
	return list
}

func formSubmissionEncoding(form *dom.Node) string {
	if form != nil {
		for _, label := range strings.Fields(form.GetAttribute("accept-charset")) {
			if encoding, name := charset.Lookup(label); encoding != nil {
				return htmlEncodingLabel(name)
			}
		}
		if form.Doc != nil && form.Doc.CharacterSet != "" {
			if _, name := charset.Lookup(form.Doc.CharacterSet); name != "" {
				return htmlEncodingLabel(name)
			}
		}
	}
	return "UTF-8"
}

func htmlEncodingLabel(name string) string {
	if strings.EqualFold(name, "utf-8") {
		return "UTF-8"
	}
	return name
}

// encodeFormActionQuery applies the form's selected character encoding to
// unescaped non-ASCII query characters in the action URL. The URL parser still
// handles resolution and URL syntax; this preserves the form submission
// algorithm's legacy encoding override for the action query.
func encodeFormActionQuery(action, label string) string {
	queryStart := strings.IndexByte(action, '?')
	if queryStart < 0 {
		return action
	}
	queryEnd := strings.IndexByte(action[queryStart+1:], '#')
	if queryEnd >= 0 {
		queryEnd += queryStart + 1
	} else {
		queryEnd = len(action)
	}
	var encoded strings.Builder
	encoded.Grow(len(action))
	encoded.WriteString(action[:queryStart+1])
	query := action[queryStart+1 : queryEnd]
	for i := 0; i < len(query); {
		if query[i] == '%' && i+2 < len(query) && isHexByte(query[i+1]) && isHexByte(query[i+2]) {
			encoded.WriteString(query[i : i+3])
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(query[i:])
		if r <= 0x7f {
			encoded.WriteByte(query[i])
			i++
			continue
		}
		bytes, err := encodeFormSubmissionString(string(r), label)
		if err != nil {
			bytes = []byte(string(r))
		}
		for _, b := range bytes {
			const hex = "0123456789ABCDEF"
			encoded.WriteByte('%')
			encoded.WriteByte(hex[b>>4])
			encoded.WriteByte(hex[b&0x0f])
		}
		i += size
	}
	encoded.WriteString(action[queryEnd:])
	return encoded.String()
}

func (r *Runtime) formDataEntries(form, submitter *dom.Node) []formDataEntry {
	var entries []formDataEntry
	for _, control := range r.listedFormControls(form) {
		if dom.IsFormControlDisabled(control) {
			continue
		}
		if state := r.elementInternals[control]; state != nil {
			name, hasName := nullNamespaceAttribute(control, "name")
			if goja.IsNull(state.formValue) || goja.IsUndefined(state.formValue) {
				continue
			}
			if object, ok := state.formValue.(*goja.Object); ok {
				if formData := r.formDataValues[object]; formData != nil {
					for _, entry := range formData.entries {
						entry.name = normalizeFormDataLineBreaks(entry.name)
						if entry.file == nil {
							entry.value = normalizeFormDataLineBreaks(entry.value)
						}
						entries = append(entries, entry)
					}
					continue
				}
			}
			if !hasName || name == "" {
				continue
			}
			entry := r.makeFormDataEntry(name, state.formValue, "")
			entry.name = normalizeFormDataLineBreaks(entry.name)
			if entry.file == nil {
				entry.value = normalizeFormDataLineBreaks(entry.value)
			}
			entries = append(entries, entry)
			continue
		}
		name := control.GetAttribute("name")
		if name == "" {
			continue
		}
		typ := strings.ToLower(control.GetAttribute("type"))
		if control.Data == "input" {
			if typ == "file" {
				files := r.inputFiles[control]
				if len(files) == 0 {
					files = []*goja.Object{r.makeBlob(r.vm.NewObject(), nil, "", true, "", timeNowMillis())}
				}
				for _, file := range files {
					entries = append(entries, formDataEntry{name: name, file: file})
				}
				continue
			}
			switch typ {
			case "button", "reset", "image":
				if control != submitter {
					continue
				}
			case "submit":
				if control != submitter {
					continue
				}
			case "checkbox", "radio":
				wrapper, _ := r.wrapNode(control).(*goja.Object)
				if wrapper == nil || !wrapper.Get("checked").ToBoolean() {
					continue
				}
			}
		} else if control.Data == "button" {
			if !isSubmitButton(control) || control != submitter {
				continue
			}
		} else if control.Data == "select" {
			for _, option := range dom.SelectOptions(control) {
				if option.OptionSelected() && !option.OptionDisabled() {
					entries = append(entries, formDataEntry{name: name, value: optionValue(option)})
				}
			}
			continue
		} else if control.Data != "textarea" {
			continue
		}
		wrapper, _ := r.wrapNode(control).(*goja.Object)
		if wrapper == nil {
			continue
		}
		value := wrapper.Get("value")
		if control.Data == "textarea" {
			value = r.vm.ToValue(control.TextareaSubmissionValue())
		} else if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			value = r.vm.ToValue(control.InputControlValue())
		}
		isCharset := control.Data == "input" && typ == "hidden" && asciiEqualFold(name, "_charset_")
		entries = append(entries, formDataEntry{name: name, value: value.String(), charset: isCharset})
	}
	// An activated image submitter contributes its click coordinates instead
	// of an ordinary name/value pair. Script-triggered click() has no pointer
	// location, so both coordinates are zero.
	if submitter != nil && submitter.Data == "input" && strings.EqualFold(submitter.GetAttribute("type"), "image") && r.formOwner(submitter) == form {
		name := submitter.GetAttribute("name")
		entries = append(entries,
			formDataEntry{name: name + ".x", value: "0"},
			formDataEntry{name: name + ".y", value: "0"},
		)
	}
	return entries
}

func (r *Runtime) encodeMultipartFormData(list *formDataList, label string) (string, []byte, error) {
	var token [18]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", nil, fmt.Errorf("could not create multipart boundary: %w", err)
	}
	boundary := "----GoosieFormBoundary" + hex.EncodeToString(token[:])
	var body bytes.Buffer
	for _, entry := range list.entries {
		body.WriteString("--")
		body.WriteString(boundary)
		body.WriteString("\r\nContent-Disposition: form-data; name=\"")
		encodedName, err := encodeFormSubmissionString(encodeMultipartName(entry.name), label)
		if err != nil {
			return "", nil, err
		}
		body.Write(encodedName)
		body.WriteByte('"')
		if entry.file == nil {
			body.WriteString("\r\n\r\n")
			encodedValue, err := encodeFormSubmissionString(normalizeFormDataLineBreaks(entry.value), label)
			if err != nil {
				return "", nil, err
			}
			body.Write(encodedValue)
			body.WriteString("\r\n")
			continue
		}
		file := entry.file
		metadata := r.fileValues[file]
		filename := ""
		contentType := "application/octet-stream"
		var data []byte
		if metadata != nil {
			filename, contentType, data = metadata.name, metadata.typeValue, metadata.data
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		body.WriteString("; filename=\"")
		encodedFilename, err := encodeFormSubmissionString(encodeMultipartFilename(filename), label)
		if err != nil {
			return "", nil, err
		}
		body.Write(encodedFilename)
		body.WriteString("\"\r\nContent-Type: ")
		body.WriteString(contentType)
		body.WriteString("\r\n\r\n")
		body.Write(data)
		body.WriteString("\r\n")
	}
	body.WriteString("--")
	body.WriteString(boundary)
	body.WriteString("--\r\n")
	return boundary, body.Bytes(), nil
}

func encodeFormSubmissionString(value, label string) ([]byte, error) {
	encoding, _ := charset.Lookup(label)
	if encoding == nil {
		encoding, _ = charset.Lookup("UTF-8")
	}
	if encoding == nil {
		return []byte(value), nil
	}
	if strings.EqualFold(label, "ISO-2022-JP") {
		value = encodeISO2022JPRomanCharacters(value)
	}
	return encoding.NewEncoder().Bytes([]byte(value))
}

func encodeISO2022JPRomanCharacters(value string) string {
	const roman = "\x1b(J"
	const ascii = "\x1b(B"
	encoding, _ := charset.Lookup("ISO-2022-JP")
	var out strings.Builder
	inRoman := false
	for _, r := range value {
		if r == '\uff65' {
			// The web encoding maps the halfwidth middle dot to the JIS
			// X 0208 middle dot when producing ISO-2022-JP.
			r = '\u30fb'
		}
		if r == '\u00a5' || r == '\u203e' {
			if !inRoman {
				out.WriteString(roman)
				inRoman = true
			}
			if r == '\u00a5' {
				out.WriteByte('\\')
			} else {
				out.WriteByte('~')
			}
			continue
		}
		if inRoman {
			if r <= 0x7f {
				out.WriteString(ascii)
				inRoman = false
			} else if encoding != nil {
				encoded, _ := encoding.NewEncoder().Bytes([]byte(string(r)))
				if bytes.Contains(encoded, []byte("\x1b$B")) {
					// An encoder-generated JIS X 0208 shift exits Roman mode.
					inRoman = false
				}
			}
		}
		out.WriteRune(r)
	}
	if inRoman {
		out.WriteString(ascii)
	}
	return out.String()
}

func encodeMultipartName(value string) string {
	value = normalizeFormDataLineBreaks(value)
	return strings.NewReplacer("\r", "%0D", "\n", "%0A", "\"", "%22").Replace(value)
}

func encodeMultipartFilename(value string) string {
	return strings.NewReplacer("\r", "%0D", "\n", "%0A", "\"", "%22").Replace(value)
}

func normalizeFormDataLineBreaks(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}

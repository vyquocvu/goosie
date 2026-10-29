package js

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// setupHTMLFormCollectionInterfaces installs the collection interfaces used
// by HTMLFormElement.elements and Document.forms.
func (r *Runtime) setupHTMLFormCollectionInterfaces() {
	if r.htmlFormControlsCollectionProto != nil {
		return
	}
	// HTMLCollection is iterable, but unlike NodeList it does not expose
	// Array-style values(), entries(), or forEach() methods.
	r.htmlCollectionProto = r.vm.NewObject()
	_ = r.vm.Set("__htmlCollectionProto__", r.htmlCollectionProto)
	_, _ = r.vm.RunString(`Object.defineProperty(__htmlCollectionProto__, Symbol.iterator, {value: Array.prototype[Symbol.iterator], configurable: true, writable: true})`)
	_ = r.vm.Set("__htmlCollectionProto__", nil)
	r.htmlFormControlsCollectionProto = r.newChildProto(r.htmlCollectionProto)
	r.radioNodeListProto = r.newChildProto(r.ensureCollectionProto())
	r.defineInterfaceToStringTag("HTMLCollection", r.htmlCollectionProto)
	r.defineInterfaceToStringTag("HTMLFormControlsCollection", r.htmlFormControlsCollectionProto)
	r.defineInterfaceToStringTag("RadioNodeList", r.radioNodeListProto)
	r.defineConstructor("NodeList", r.ensureCollectionProto())
	r.defineConstructor("RadioNodeList", r.radioNodeListProto)
	r.defineConstructor("HTMLCollection", r.htmlCollectionProto)
	r.defineConstructor("HTMLFormControlsCollection", r.htmlFormControlsCollectionProto)
	_ = r.documentProto.Set("getElementsByName", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		documentNode := r.getNode(call.This)
		if documentNode == nil || documentNode.Type != dom.NodeDocument || documentNode.Doc == nil || len(call.Arguments) == 0 {
			return r.newLiveCollectionMatching(func() []*dom.Node { return nil }, r.ensureCollectionProto())
		}
		name := call.Argument(0).String()
		return r.newLiveCollectionMatching(func() []*dom.Node {
			var nodes []*dom.Node
			documentNode.Doc.Node.Walk(func(node *dom.Node) bool {
				if node.Element() && node.GetAttribute("name") == name {
					nodes = append(nodes, node)
				}
				return true
			})
			return nodes
		}, r.ensureCollectionProto())
	}))

	_ = r.documentProto.DefineAccessorProperty("forms", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument {
			return r.newLiveHTMLCollectionMatching(func() []*dom.Node { return nil })
		}
		if r.documentFormCollections == nil {
			r.documentFormCollections = make(map[*dom.Node]*goja.Object)
		}
		if collection := r.documentFormCollections[node]; collection != nil {
			return collection
		}
		matches := func() []*dom.Node {
			return collectByTagInSubtree(node, "form")
		}
		collection := r.newLiveHTMLCollectionMatching(matches).(*goja.Object)
		proxy := r.vm.NewProxy(collection, &goja.ProxyTrapConfig{
			Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
				if isCanonicalIndex(property) {
					index, _ := strconv.Atoi(property)
					forms := matches()
					if index >= 0 && index < len(forms) {
						return r.wrapNode(forms[index])
					}
					return goja.Undefined()
				}
				if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
					return value
				}
				if form := findNamedCollectionItem(matches(), property); form != nil && form.Data == "form" {
					return r.wrapNode(form)
				}
				return goja.Undefined()
			},
			GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
				forms := matches()
				if index >= 0 && index < len(forms) {
					return r.wrapNode(forms[index])
				}
				return goja.Undefined()
			},
			Has: func(target *goja.Object, property string) bool {
				if isCanonicalIndex(property) {
					index, _ := strconv.Atoi(property)
					return index >= 0 && index < len(matches())
				}
				if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
					return true
				}
				form := findNamedCollectionItem(matches(), property)
				return form != nil && form.Data == "form"
			},
			HasIdx: func(_ *goja.Object, index int) bool {
				return index >= 0 && index < len(matches())
			},
		})
		collection = r.vm.ToValue(proxy).(*goja.Object)
		r.documentFormCollections[node] = collection
		return collection
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) installHTMLFormElementInterface(proto *goja.Object) {
	r.installStringReflection(proto, "acceptCharset", "accept-charset")
	r.installStringReflection(proto, "name", "name")
	r.installStringReflection(proto, "target", "target")
	r.installStringReflection(proto, "rel", "rel")
	r.installFormActionReflection(proto)
	r.installEnumReflection(proto, "enctype", "enctype", "application/x-www-form-urlencoded", []string{"application/x-www-form-urlencoded", "multipart/form-data", "text/plain"})
	r.installEnumReflection(proto, "method", "method", "get", []string{"get", "post", "dialog"})
	r.installBoolReflection(proto, "noValidate", "novalidate")
	r.installAutocompleteReflection(proto, true)
	checkValidity := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		form := r.getNode(call.This)
		return r.vm.ToValue(r.checkFormValidity(form))
	})
	_ = proto.Set("checkValidity", checkValidity)
	_ = proto.Set("reportValidity", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		form := r.getNode(call.This)
		return r.vm.ToValue(r.checkFormValidityWithReport(form, true))
	}))
	_ = proto.Set("requestSubmit", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		form := r.getNode(call.This)
		if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) || goja.IsUndefined(call.Argument(0)) {
			r.requestSubmit(form, nil)
			return goja.Undefined()
		}
		submitter := r.getNode(call.Argument(0))
		if !isSubmitButton(submitter) {
			panic(r.vm.NewTypeError("The specified element is not a submit button."))
		}
		if r.formOwner(submitter) != form && !formHasListedControl(r, form, submitter) {
			panic(r.domError("NotFoundError", "The specified element is not owned by this form."))
		}
		r.requestSubmit(form, submitter)
		return goja.Undefined()
	}))
	_ = proto.Set("submit", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		form := r.getNode(call.This)
		if form != nil && form.IsConnected() {
			r.navigateFormTarget(form, nil)
		}
		return goja.Undefined()
	}))
	_ = proto.Set("reset", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.resetForm(r.getNode(call.This))
		return goja.Undefined()
	}))
	_ = proto.DefineAccessorProperty("elements", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		form := r.getNode(call.This)
		if form == nil {
			return goja.Null()
		}
		if r.formElementCollections == nil {
			r.formElementCollections = make(map[*dom.Node]*goja.Object)
		}
		if collection := r.formElementCollections[form]; collection != nil {
			return collection
		}
		matches := func() []*dom.Node {
			return r.listedFormControls(form)
		}
		collection := r.newLiveFormControlsCollection(matches).(*goja.Object)
		r.formElementCollections[form] = collection
		return collection
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		form := r.getNode(call.This)
		return r.vm.ToValue(len(r.listedFormControls(form)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) installFieldsetInterface(proto *goja.Object) {
	r.installFormControlValidation(proto)
	_ = proto.DefineAccessorProperty("type", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if fieldset := r.getNode(call.This); fieldset != nil && fieldset.Data == "fieldset" {
			return r.vm.ToValue("fieldset")
		}
		return goja.Undefined()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("elements", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		fieldset := r.getNode(call.This)
		if fieldset == nil || fieldset.Data != "fieldset" {
			return r.newLiveHTMLCollectionMatching(func() []*dom.Node { return nil })
		}
		if r.fieldsetElements == nil {
			r.fieldsetElements = make(map[*dom.Node]*goja.Object)
		}
		if collection := r.fieldsetElements[fieldset]; collection != nil {
			return collection
		}
		matches := func() []*dom.Node {
			var elements []*dom.Node
			for child := fieldset.FirstChild; child != nil; child = child.NextSibling {
				child.Walk(func(node *dom.Node) bool {
					if isListedFormElement(node) || r.isFormAssociatedCustomElement(node) {
						elements = append(elements, node)
					}
					return true
				})
			}
			return elements
		}
		collection, _ := r.newLiveHTMLCollectionMatching(matches).(*goja.Object)
		r.fieldsetElements[fieldset] = collection
		return collection
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// resetForm runs the reset event gate before applying the reset algorithms for
// the built-in controls whose live state is modeled by this runtime.
func (r *Runtime) resetForm(form *dom.Node) {
	r.resetFormWithCallbackTiming(form, false)
}

// resetFormWithCallbackTiming applies the reset algorithm and delivers
// form-associated custom element reactions either before returning (the
// form.reset() method) or at the next microtask checkpoint (reset-button
// activation).
func (r *Runtime) resetFormWithCallbackTiming(form *dom.Node, deferCallbacks bool) {
	if form == nil || form.Data != "form" || r.formResetInProgress[form] {
		return
	}
	if r.formResetInProgress == nil {
		r.formResetInProgress = make(map[*dom.Node]bool)
	}
	r.formResetInProgress[form] = true
	defer delete(r.formResetInProgress, form)
	event := dom.NewTrustedEvent("reset", false, true)
	if !dom.DispatchEvent(form, event) {
		return
	}
	for _, control := range r.listedFormControls(form) {
		switch control.Data {
		case "input":
			typ := strings.ToLower(control.GetAttribute("type"))
			if typ == "checkbox" || typ == "radio" {
				control.ResetInputChecked()
				if typ == "radio" && control.InputChecked() {
					r.reconcileRadioGroupsForTree(control)
				}
			} else {
				control.ResetInputControlValue()
			}
			if typ == "file" {
				r.clearInputFiles(control)
			}
		case "textarea":
			control.ResetInputControlValue()
		case "select":
			control.SelectNoSelection = false
			for _, option := range dom.SelectOptions(control) {
				option.ResetOptionSelectedness()
			}
			dom.NormalizeSelectSelectedness(control)
		case "output":
			value := control.ResetOutputControlValue()
			r.setNodeTextContent(control, value)
		}
	}
	var resetCallbacks []*dom.Node
	for _, control := range r.listedFormControls(form) {
		if r.isFormAssociatedCustomElement(control) {
			resetCallbacks = append(resetCallbacks, control)
		}
	}
	if deferCallbacks {
		r.queueMicrotask(func() error {
			for _, control := range resetCallbacks {
				r.invokeLifecycleCallback(control, "formResetCallback")
			}
			return nil
		})
	} else {
		for _, control := range resetCallbacks {
			r.invokeLifecycleCallback(control, "formResetCallback")
		}
	}
	r.notifyMutation()
}

func formHasListedControl(r *Runtime, form, candidate *dom.Node) bool {
	for _, control := range r.listedFormControls(form) {
		if control == candidate {
			return true
		}
	}
	return false
}

func (r *Runtime) requestSubmit(form, submitter *dom.Node) {
	r.requestSubmitWithBeforeNavigation(form, submitter, nil)
}

func (r *Runtime) requestSubmitWithBeforeNavigation(form, submitter *dom.Node, beforeNavigation func()) bool {
	if form == nil || form.Data != "form" || r.formSubmissionInProgress[form] || r.constructingFormEntryList[form] {
		return false
	}
	if r.formSubmissionInProgress == nil {
		r.formSubmissionInProgress = make(map[*dom.Node]bool)
	}
	r.formSubmissionInProgress[form] = true
	defer delete(r.formSubmissionInProgress, form)
	if !form.IsConnected() {
		return false
	}
	noValidate := form.HasAttribute("novalidate") || submitter != nil && submitter.HasAttribute("formnovalidate")
	if !noValidate && !r.checkFormValidityWithReport(form, true) {
		return false
	}
	event := dom.NewSubmitEvent("submit", true, true, submitter)
	if !dom.DispatchEvent(form, &event.Event) {
		return false
	}
	if beforeNavigation != nil {
		beforeNavigation()
	}
	r.navigateFormTarget(form, submitter)
	return true
}

func (r *Runtime) navigateFormTarget(form, submitter *dom.Node) {
	r.navigateFormTargetWithTarget(form, submitter, "", false)
}

func (r *Runtime) navigateFormTargetWithTarget(form, submitter *dom.Node, targetOverride string, hasTargetOverride bool) {
	if form == nil || r.constructingFormEntryList[form] {
		return
	}
	target := form.GetAttribute("target")
	if submitter != nil && submitter.HasAttribute("formtarget") {
		target = submitter.GetAttribute("formtarget")
	}
	if hasTargetOverride {
		target = targetOverride
	}
	if form.Doc == nil {
		return
	}
	destination := r
	var frame *dom.Node
	if target == "" || strings.EqualFold(target, "_self") {
		var ownerFrame *dom.Node
		if form.Doc != nil {
			ownerFrame = r.iframeFrameForDocument(&form.Doc.Node)
		}
		if ownerFrame != nil {
			// Parent-realm wrappers can call methods on controls in a child
			// document. In that case this Runtime owns the iframe mapping, so
			// navigate the frame containing the form itself.
			destination = r
			frame = ownerFrame
		} else if r.iframeParent != nil && r.iframeParentFrame != nil {
			destination = r.iframeParent
			frame = r.iframeParentFrame
		} else {
			return
		}
	} else if strings.HasPrefix(target, "_") {
		return
	} else {
		form.Doc.Node.Walk(func(node *dom.Node) bool {
			if node.Element() && node.Data == "iframe" && node.GetAttribute("name") == target {
				frame = node
				return false
			}
			return true
		})
	}
	if frame == nil {
		return
	}
	if r.activeSubmitter != nil && r.formOwner(r.activeSubmitter) == form {
		r.activationFormFrames = append(r.activationFormFrames, frame)
	}
	action := form.GetAttribute("action")
	if submitter != nil && submitter.HasAttribute("formaction") {
		action = submitter.GetAttribute("formaction")
	}
	encoding := formSubmissionEncoding(form)
	if action == "" {
		if pageURL := r.documentURL(); pageURL != nil {
			action = pageURL.String()
		}
	} else {
		action = encodeFormActionQuery(action, encoding)
		action = r.resolveURLString(action)
	}
	parsed, err := url.Parse(action)
	if err != nil {
		return
	}
	method := strings.ToLower(form.GetAttribute("method"))
	if method == "" {
		method = "get"
	}
	enctype := strings.ToLower(form.GetAttribute("enctype"))
	if enctype == "" {
		enctype = "application/x-www-form-urlencoded"
	}
	if submitter != nil {
		if submitter.HasAttribute("formmethod") {
			method = strings.ToLower(submitter.GetAttribute("formmethod"))
		}
		if submitter.HasAttribute("formenctype") {
			enctype = strings.ToLower(submitter.GetAttribute("formenctype"))
		}
	}
	formData := r.formDataForSubmission(form, submitter, encoding)
	if !form.IsConnected() {
		return
	}
	if method != "get" && method != "post" {
		return
	}
	// Navigation tasks are superseded per browsing context. Two submissions
	// from the same form to different named frames are independent; repeated
	// submissions to one frame replace that frame's planned URL.
	navigationID := destination.nextIFrameNavigationID(frame)
	if destination.formNavigationFrames == nil {
		destination.formNavigationFrames = make(map[*dom.Node]*dom.Node)
	}
	destination.formNavigationFrames[form] = frame
	if method == "get" {
		encodedData := make([]string, 0, len(formData.entries))
		for _, entry := range formData.entries {
			encodedData = append(encodedData, url.QueryEscape(normalizeFormDataLineBreaks(entry.name))+"="+url.QueryEscape(r.formDataSubmissionValue(entry)))
		}
		parsed.RawQuery = strings.Join(encodedData, "&")
		parsed.ForceQuery = true
		// Form GET navigation is a planned navigation. Queue it after the
		// current script task so callers can install load handlers immediately
		// after requestSubmit() returns. A later GET to the same frame replaces
		// the planned destination without dispatching an intermediate load.
		if destination.plannedIFrameGETURLs == nil {
			destination.plannedIFrameGETURLs = make(map[*dom.Node]string)
		}
		if destination.iframeFormRequests != nil {
			delete(destination.iframeFormRequests, frame)
		}
		pending := destination.pendingIFrames[:0]
		for _, queued := range destination.pendingIFrames {
			if queued != frame {
				pending = append(pending, queued)
			}
		}
		destination.pendingIFrames = pending
		destination.plannedIFrameGETURLs[frame] = parsed.String()
		destination.QueueTask(func() {
			if destination.iframeNavigationIDs[frame] != navigationID {
				return
			}
			targetURL := destination.plannedIFrameGETURLs[frame]
			delete(destination.plannedIFrameGETURLs, frame)
			if targetURL == "" || !frame.IsConnected() {
				return
			}
			if wrapper, ok := destination.wrapNode(frame).(*goja.Object); ok {
				_ = wrapper.Set("src", targetURL)
				destination.flushIFrameLoads()
				if destination.iframeNavigationIDs[frame] == navigationID {
					delete(destination.formNavigationFrames, form)
				}
			}
		})
		return
	} else if method == "post" {
		contentType := enctype
		var body []byte
		switch enctype {
		case "multipart/form-data":
			boundary, encoded, err := r.encodeMultipartFormData(formData, encoding)
			if err != nil {
				return
			}
			contentType += "; boundary=" + boundary
			body = encoded
		case "text/plain":
			var text strings.Builder
			for _, entry := range formData.entries {
				text.WriteString(normalizeFormDataLineBreaks(entry.name))
				text.WriteByte('=')
				text.WriteString(normalizeFormDataLineBreaks(r.formDataSubmissionValue(entry)))
				text.WriteString("\r\n")
			}
			body = []byte(text.String())
		default:
			pairs := make([]string, 0, len(formData.entries))
			for _, entry := range formData.entries {
				pairs = append(pairs, url.QueryEscape(normalizeFormDataLineBreaks(entry.name))+"="+url.QueryEscape(r.formDataSubmissionValue(entry)))
			}
			contentType = "application/x-www-form-urlencoded"
			body = []byte(strings.Join(pairs, "&"))
		}
		if destination.iframeFormRequests == nil {
			destination.iframeFormRequests = make(map[*dom.Node]iframeFormRequest)
		}
		delete(destination.plannedIFrameGETURLs, frame)
		if destination.iframeLocations == nil {
			destination.iframeLocations = make(map[*dom.Node]string)
		}
		destination.iframeFormRequests[frame] = iframeFormRequest{url: parsed.String(), contentType: contentType, body: body}
		destination.iframeLocations[frame] = parsed.String()
		pending := destination.pendingIFrames[:0]
		for _, queued := range destination.pendingIFrames {
			if queued != frame {
				pending = append(pending, queued)
			}
		}
		destination.pendingIFrames = pending
		destination.pendingIFrames = append(destination.pendingIFrames, frame)
		destination.QueueTask(func() {
			if destination.iframeNavigationIDs[frame] == navigationID {
				destination.flushIFrameLoads()
				if destination.iframeNavigationIDs[frame] == navigationID {
					delete(destination.formNavigationFrames, form)
				}
			}
		})
		return
	} else {
		return
	}
}

func (r *Runtime) nextIFrameNavigationID(frame *dom.Node) uint64 {
	if r.iframeNavigationIDs == nil {
		r.iframeNavigationIDs = make(map[*dom.Node]uint64)
	}
	r.iframeNavigationIDs[frame]++
	return r.iframeNavigationIDs[frame]
}

func (r *Runtime) formDataSubmissionValue(entry formDataEntry) string {
	if entry.file != nil {
		if file := r.fileValues[entry.file]; file != nil {
			return normalizeFormDataLineBreaks(file.name)
		}
	}
	return normalizeFormDataLineBreaks(entry.value)
}

func isSubmitButton(node *dom.Node) bool {
	if node == nil || node.Namespace != dom.NSHTML {
		return false
	}
	typ := strings.ToLower(node.GetAttribute("type"))
	switch node.Data {
	case "button":
		return typ != "button" && typ != "reset"
	case "input":
		return typ == "submit" || typ == "image"
	default:
		return false
	}
}

func isResetButton(node *dom.Node) bool {
	if node == nil || node.Namespace != dom.NSHTML {
		return false
	}
	typ := strings.ToLower(node.GetAttribute("type"))
	switch node.Data {
	case "button":
		return typ == "reset"
	case "input":
		return typ == "reset"
	default:
		return false
	}
}

func (r *Runtime) clickHTMLElement(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || !node.Element() || dom.IsFormControlDisabled(node) {
		return goja.Undefined()
	}
	owner := r.formOwner(node)
	previousSubmitter, previousActivationFrames := r.activeSubmitter, r.activationFormFrames
	if owner != nil {
		r.activeSubmitter = node
		r.activationFormFrames = nil
	}
	clickEvent := dom.NewEvent("click", true, true)
	if r.testDriverTrustedClick {
		clickEvent = dom.NewTrustedEvent("click", true, true)
	}
	clickAllowed := r.dispatchClickActivation(node, clickEvent)
	activationFrames := r.activationFormFrames
	r.activeSubmitter, r.activationFormFrames = previousSubmitter, previousActivationFrames
	if !clickAllowed {
		return goja.Undefined()
	}
	if label := labelForClick(node); label != nil {
		if control := r.labelControl(label); control != nil && !dom.IsFormControlDisabled(control) {
			controlClick := dom.NewEvent("click", true, true)
			if r.testDriverTrustedClick {
				controlClick = dom.NewTrustedEvent("click", true, true)
			}
			if r.dispatchClickActivation(control, controlClick) && r.opts.OnFocusChange != nil {
				r.opts.OnFocusChange(control, true)
			}
		}
	}
	popoverInvoker := node
	if popoverInvoker.Data != "button" && popoverInvoker.Data != "input" {
		for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Data == "button" || ancestor.Data == "input" {
				popoverInvoker = ancestor
				break
			}
		}
	}
	if (popoverInvoker.Data == "button" || popoverInvoker.Data == "input") && popoverInvoker.HasAttribute("popovertarget") {
		formActionTakesPrecedence := r.formOwner(popoverInvoker) != nil && (isSubmitButton(popoverInvoker) || isResetButton(popoverInvoker))
		if !formActionTakesPrecedence {
			r.activatePopoverTarget(popoverInvoker, popoverInvoker.GetAttribute("popovertarget"), popoverInvoker.GetAttribute("popovertargetaction"))
			return goja.Undefined()
		}
	}
	if popoverInvoker.Data == "button" && popoverInvoker.HasAttribute("commandfor") {
		if strings.EqualFold(strings.TrimSpace(popoverInvoker.GetAttribute("command")), "toggle") {
			if target := r.popoverTargetForInvoker(popoverInvoker, popoverInvoker.GetAttribute("commandfor")); target != nil && target.Data == "details" {
				r.SetDetailsOpen(target, !target.HasAttribute("open"), popoverInvoker)
				return goja.Undefined()
			}
		}
		if action, ok := popoverCommandAction(popoverInvoker.GetAttribute("command")); ok {
			formActionTakesPrecedence := r.formOwner(popoverInvoker) != nil && (isSubmitButton(popoverInvoker) || isResetButton(popoverInvoker))
			if !formActionTakesPrecedence {
				r.activatePopoverTarget(popoverInvoker, popoverInvoker.GetAttribute("commandfor"), action)
				return goja.Undefined()
			}
		}
	}
	if node.Data == "summary" {
		for details := node.Parent; details != nil; details = details.Parent {
			if !details.Element() || details.Data != "details" {
				continue
			}
			firstSummary := (*dom.Node)(nil)
			for child := details.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() && child.Data == "summary" {
					firstSummary = child
					break
				}
			}
			if firstSummary == node {
				r.SetDetailsOpen(details, !details.HasAttribute("open"), nil)
				r.notifyMutation()
			}
			break
		}
	}
	if node.Data == "details" {
		hasSummary := false
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && child.Data == "summary" {
				hasSummary = true
				break
			}
		}
		if !hasSummary {
			r.SetDetailsOpen(node, !node.HasAttribute("open"), nil)
		}
	}
	if (node.Data == "a" || node.Data == "area") && !dom.IsContentEditable(node) {
		href := strings.TrimSpace(node.GetAttribute("href"))
		if node.Data == "a" && len(href) >= len("javascript:") && strings.EqualFold(href[:len("javascript:")], "javascript:") {
			r.runJavaScriptURL(node, href[len("javascript:"):])
			return goja.Undefined()
		}
		r.activateLink(node)
	}
	if isSubmitButton(node) {
		owner = r.formOwner(node)
		if owner != nil {
			r.requestSubmitWithBeforeNavigation(owner, node, func() {
				for _, frame := range activationFrames {
					r.cancelPlannedFormNavigation(frame)
				}
			})
		}
	} else if isResetButton(node) {
		if owner := r.formOwner(node); owner != nil {
			r.resetFormWithCallbackTiming(owner, true)
		}
	}
	return goja.Undefined()
}

func popoverCommandAction(command string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "show-popover":
		return "show", true
	case "hide-popover":
		return "hide", true
	case "toggle-popover":
		return "toggle", true
	default:
		return "", false
	}
}

func (r *Runtime) activatePopoverTarget(invoker *dom.Node, targetID, action string) {
	if r.domDoc == nil || targetID == "" {
		return
	}
	target := r.popoverTargetForInvoker(invoker, targetID)
	if target == nil || !target.HasAttribute("popover") {
		return
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "show":
		if !target.IsPopoverOpen {
			if invoker != nil {
				if r.popoverInvokers == nil {
					r.popoverInvokers = make(map[*dom.Node]*dom.Node)
				}
				r.popoverInvokers[target] = invoker
			}
			if !r.setPopoverOpenFrom(target, true, invoker) && r.popoverInvokers != nil {
				delete(r.popoverInvokers, target)
			}
		}
	case "hide":
		if target.IsPopoverOpen {
			r.setPopoverOpenFrom(target, false, invoker)
		}
	default:
		opening := !target.IsPopoverOpen
		if opening && invoker != nil {
			if r.popoverInvokers == nil {
				r.popoverInvokers = make(map[*dom.Node]*dom.Node)
			}
			r.popoverInvokers[target] = invoker
		}
		if !r.setPopoverOpenFrom(target, opening, invoker) && opening && r.popoverInvokers != nil {
			delete(r.popoverInvokers, target)
		}
	}
}

// popoverTargetForInvoker resolves popovertarget in the invoker's tree scope.
// Shadow-tree IDs are intentionally separate from the document's ID index.
func (r *Runtime) popoverTargetForInvoker(invoker *dom.Node, id string) *dom.Node {
	if invoker == nil || id == "" {
		return r.domDoc.ElementByID(id)
	}
	for ancestor := invoker; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.ShadowRoot == nil || !containsTreeNode(ancestor.ShadowRoot.FirstChild, invoker) {
			continue
		}
		return findElementByIDInTree(ancestor.ShadowRoot.FirstChild, id)
	}
	return r.domDoc.ElementByID(id)
}

func containsTreeNode(root, target *dom.Node) bool {
	for node := root; node != nil; node = node.NextSibling {
		if node == target || containsTreeNode(node.FirstChild, target) {
			return true
		}
	}
	return false
}

func findElementByIDInTree(root *dom.Node, id string) *dom.Node {
	for node := root; node != nil; node = node.NextSibling {
		if node.Element() && node.GetAttribute("id") == id {
			return node
		}
		if found := findElementByIDInTree(node.FirstChild, id); found != nil {
			return found
		}
	}
	return nil
}

func shadowTreeHost(node *dom.Node) *dom.Node {
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.ShadowRoot != nil && containsTreeNode(ancestor.ShadowRoot.FirstChild, node) {
			return ancestor
		}
	}
	return nil
}

func labelForClick(target *dom.Node) *dom.Node {
	if target == nil {
		return nil
	}
	for label := target; label != nil; label = label.Parent {
		if !label.Element() || label.Data != "label" {
			continue
		}
		for node := target; node != nil && node != label; node = node.Parent {
			if node.Element() {
				switch node.Data {
				case "button", "input", "select", "summary", "textarea":
					return nil
				case "a", "area", "audio", "video":
					if node.HasAttribute("href") || node.HasAttribute("controls") {
						return nil
					}
				}
			}
		}
		return label
	}
	return nil
}

func (r *Runtime) activateLinkFragment(href string) {
	if href == "" {
		return
	}
	oldURL, base := r.locationState()
	if base == nil || !base.IsAbs() {
		return
	}
	reference, err := url.Parse(href)
	if err != nil {
		return
	}
	resolved := base.ResolveReference(reference)
	if !strings.EqualFold(base.Scheme, resolved.Scheme) || !strings.EqualFold(base.Host, resolved.Host) || base.EscapedPath() != resolved.EscapedPath() || base.RawQuery != resolved.RawQuery {
		return
	}
	oldFragment := base.EscapedFragment()
	newFragment := resolved.EscapedFragment()
	if oldFragment == newFragment {
		return
	}
	newURL := resolved.String()
	r.setLocation(newURL)
	r.revealFragmentTarget(newFragment)
	event := dom.NewHashChangeEvent("hashchange", false, false, oldURL, newURL)
	r.fireWindowListeners(&event.Event, true)
	if !event.Event.PropagationStopped() {
		r.fireWindowListeners(&event.Event, false)
	}
}

func (r *Runtime) activateLink(node *dom.Node) {
	if node == nil || !node.HasAttribute("href") {
		return
	}
	href := strings.TrimSpace(node.GetAttribute("href"))
	contextRuntime, frame := r.browsingContextForNode(node)
	if contextRuntime == nil {
		contextRuntime = r
	}
	targetName := strings.TrimSpace(node.GetAttribute("target"))
	if targetName == "" && node.Doc != nil {
		node.Doc.Node.Walk(func(candidate *dom.Node) bool {
			if candidate.Element() && candidate.Namespace == dom.NSHTML && candidate.Data == "base" && candidate.HasAttribute("target") {
				targetName = strings.TrimSpace(candidate.GetAttribute("target"))
				return false
			}
			return true
		})
	}
	if strings.EqualFold(targetName, "_blank") {
		_, destination, ok := r.resolveURLForNode(href, node)
		if !ok || destination == nil {
			return
		}
		rel := node.GetAttribute("rel")
		noOpener := relContainsToken(rel, "noopener") || relContainsToken(rel, "noreferrer") || !relContainsToken(rel, "opener")
		var opener goja.Value
		if !noOpener {
			if node.Doc == r.domDoc {
				opener = r.vm.GlobalObject()
			} else if frame != nil {
				opener = contextRuntime.iframeWindow(frame)
			} else {
				opener = r.vm.GlobalObject()
			}
		}
		name := targetName
		r.openBrowsingContext(destination.String(), name, opener)
		activationRuntime := r
		if owner, sourceFrame := r.browsingContextForNode(node); owner != nil && sourceFrame != nil {
			activationRuntime = owner
			if child := owner.iframeChildRuntimes[sourceFrame]; child != nil {
				activationRuntime = child
			}
		}
		activationRuntime.SetTransientUserActivation(false)
		return
	}
	if targetName != "" && !strings.HasPrefix(targetName, "_") {
		if targetRuntime, targetFrame := r.findNamedBrowsingContext(targetName); targetFrame != nil {
			_, destination, ok := r.resolveURLForNode(href, node)
			if ok && destination != nil {
				targetRuntime.scheduleIFrameNavigation(targetFrame, destination.String())
			}
			return
		}
		_, destination, ok := r.resolveURLForNode(href, node)
		if !ok || destination == nil {
			return
		}
		var opener goja.Value
		if !relContainsToken(node.GetAttribute("rel"), "noopener") && !relContainsToken(node.GetAttribute("rel"), "noreferrer") {
			if node.Doc == r.domDoc {
				opener = r.vm.GlobalObject()
			} else if frame != nil {
				opener = contextRuntime.iframeWindow(frame)
			} else {
				opener = r.vm.GlobalObject()
			}
		}
		r.openBrowsingContext(destination.String(), targetName, opener)
		return
	}
	if frame != nil {
		destinationRuntime, destinationFrame := contextRuntime, frame
		switch {
		case targetName == "" || strings.EqualFold(targetName, "_self"):
		case (strings.EqualFold(targetName, "_parent") || strings.EqualFold(targetName, "_top")) && contextRuntime.iframeOpenedContexts[frame]:
		case (strings.EqualFold(targetName, "_parent") || strings.EqualFold(targetName, "_top")) && contextRuntime.iframeParent != nil:
			destinationRuntime = contextRuntime.iframeParent
			destinationFrame = contextRuntime.iframeParentFrame
		default:
			return
		}
		_, target, ok := r.resolveURLForNode(href, node)
		current := r.documentURLForNode(node)
		if ok && target != nil && current != nil {
			withoutFragment := *target
			withoutFragment.Fragment, withoutFragment.RawFragment = "", ""
			currentWithoutFragment := *current
			currentWithoutFragment.Fragment, currentWithoutFragment.RawFragment = "", ""
			if withoutFragment.String() != currentWithoutFragment.String() {
				destinationRuntime.scheduleIFrameNavigation(destinationFrame, target.String())
			}
		}
		return
	}
	r.activateLinkFragment(href)
}

func (r *Runtime) findNamedBrowsingContext(name string) (*Runtime, *dom.Node) {
	if r == nil {
		return nil, nil
	}
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	seen := make(map[*Runtime]bool)
	var search func(*Runtime) (*Runtime, *dom.Node)
	search = func(runtime *Runtime) (*Runtime, *dom.Node) {
		if runtime == nil || seen[runtime] {
			return nil, nil
		}
		seen[runtime] = true
		if runtime.domDoc != nil {
			var found *dom.Node
			runtime.domDoc.Node.Walk(func(node *dom.Node) bool {
				if node.Element() && node.Namespace == dom.NSHTML && node.Data == "iframe" && node.GetAttribute("name") == name {
					found = node
					return false
				}
				return true
			})
			if found != nil {
				return runtime, found
			}
		}
		for _, child := range runtime.iframeChildRuntimes {
			if owner, frame := search(child); frame != nil {
				return owner, frame
			}
		}
		return nil, nil
	}
	return search(root)
}

func (r *Runtime) browsingContextForNode(node *dom.Node) (*Runtime, *dom.Node) {
	if node == nil || node.Doc == nil {
		return nil, nil
	}
	return r.browsingContextForDocument(node.Doc)
}

func (r *Runtime) browsingContextForDocument(doc *dom.Document) (*Runtime, *dom.Node) {
	if r == nil || doc == nil {
		return nil, nil
	}
	if doc == r.domDoc {
		if r.iframeParent != nil && r.iframeParentFrame != nil {
			return r.iframeParent, r.iframeParentFrame
		}
		return r, nil
	}
	for frame, candidate := range r.iframeDocuments {
		if candidate == doc {
			return r, frame
		}
		if child := r.iframeChildRuntimes[frame]; child != nil {
			if owner, childFrame := child.browsingContextForDocument(doc); owner != nil {
				return owner, childFrame
			}
		}
	}
	return nil, nil
}

func relContainsToken(rel, token string) bool {
	for _, value := range strings.FieldsFunc(rel, func(r rune) bool {
		return r == '\t' || r == '\n' || r == '\f' || r == '\r' || r == ' '
	}) {
		if strings.EqualFold(value, token) {
			return true
		}
	}
	return false
}

func (r *Runtime) revealFragmentTarget(fragment string) {
	if r == nil || r.domDoc == nil || fragment == "" {
		return
	}
	identifier, err := url.PathUnescape(fragment)
	if err != nil {
		identifier = fragment
	}
	target := r.domDoc.ElementByID(identifier)
	if target == nil {
		var walk func(*dom.Node)
		walk = func(parent *dom.Node) {
			for node := parent; node != nil && target == nil; node = node.NextSibling {
				if node.Element() && node.Data == "a" && node.GetAttribute("name") == identifier {
					target = node
					return
				}
				walk(node.FirstChild)
			}
		}
		walk(&r.domDoc.Node)
	}
	if target == nil {
		return
	}
	for ancestor := target.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Element() && ancestor.Data == "details" && !ancestor.HasAttribute("open") {
			r.SetDetailsOpen(ancestor, true, nil)
		}
	}
	if geometry := r.layoutGeometryProvider(); geometry != nil {
		info, ok := geometry(target)
		if !ok {
			for ancestor := target.Parent; ancestor != nil && !ok; ancestor = ancestor.Parent {
				if ancestor.Element() && ancestor.Data == "details" {
					info, ok = geometry(ancestor)
				}
			}
		}
		if ok {
			r.SetWindowScrollY(info.Y + r.WindowScrollY())
		}
	}
}

// testDriverClick dispatches the user input sequence used by the in-process
// WPT testdriver adapter. Script-visible HTMLElement.click() remains a
// synthetic, untrusted click.
func (r *Runtime) testDriverClick(target goja.Value) {
	node := r.getNode(target)
	if node == nil || !node.Element() || dom.IsFormControlDisabled(node) {
		return
	}
	for _, typ := range []string{"pointerdown", "mousedown", "pointerup", "mouseup"} {
		r.dispatchWithWindowStep(node, dom.NewTrustedEvent(typ, true, true))
		if typ == "pointerdown" {
			if r.testDriverPointerDown == nil {
				r.testDriverPointerDown = make(map[string]*dom.Node)
			}
			r.testDriverPointerDown["mouse"] = node
		} else if typ == "pointerup" {
			r.handleTestDriverPointerUp(node, "mouse", 0)
		}
	}
	r.testDriverTrustedClick = true
	defer func() { r.testDriverTrustedClick = false }()
	r.clickHTMLElement(goja.FunctionCall{This: target})
}

func (r *Runtime) dispatchClickActivation(node *dom.Node, event *dom.Event) bool {
	if node == nil || event == nil {
		return true
	}
	var radioMembers []*dom.Node
	var radioChecked []bool
	var changed bool
	var checkboxChecked bool
	checkbox := node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "checkbox")
	radio := node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "radio")
	switch {
	case checkbox:
		checkboxChecked = node.InputChecked()
		changed = true
		node.SetInputChecked(!checkboxChecked, true)
		node.InputIndeterminate = false
	case radio:
		radioMembers = r.radioButtonGroupMembers(node)
		radioChecked = make([]bool, len(radioMembers))
		for i, member := range radioMembers {
			radioChecked[i] = member.InputChecked()
		}
		changed = !node.InputChecked()
		if changed {
			r.setInputCheckedness(node, true)
		}
	}

	allowed := r.dispatchWithWindowStep(node, event)
	if allowed && node.Data == "input" && r.HasTransientUserActivation() {
		switch strings.ToLower(strings.TrimSpace(node.GetAttribute("type"))) {
		case "color", "date", "datetime-local", "file", "month", "time", "week":
			// Activating a control with a picker consumes the current transient
			// activation, even when this runtime has no native picker UI to open.
			r.SetTransientUserActivation(false)
		}
	}
	if !allowed {
		switch {
		case checkbox:
			node.SetInputChecked(checkboxChecked, true)
			node.InputIndeterminate = false
		case radio:
			for i, member := range radioMembers {
				member.SetInputChecked(radioChecked[i], true)
			}
		}
	}
	// Checkbox and radio activation still changes checkedness while detached,
	// but the HTML activation algorithm only fires the input/change events for
	// a control that remains connected after click dispatch.
	if changed && allowed && node.IsConnected() {
		r.dispatchWithWindowStep(node, dom.NewTrustedEvent("input", true, false))
		r.dispatchWithWindowStep(node, dom.NewTrustedEvent("change", true, false))
	}
	return allowed
}

func (r *Runtime) runJavaScriptURL(anchor *dom.Node, source string) {
	if anchor == nil || source == "" {
		return
	}
	runtime := r
	if anchor.Doc != nil {
		if frame := r.iframeFrameForDocument(&anchor.Doc.Node); frame != nil {
			if child := r.iframeChildRuntimes[frame]; child != nil {
				runtime = child
			}
		}
	}
	if !runtime.opts.ScriptingDisabled {
		_ = runtime.Run(source, "javascript-url")
	}
}

func (r *Runtime) cancelPlannedFormNavigation(frame *dom.Node) {
	if frame == nil {
		return
	}
	r.nextIFrameNavigationID(frame)
	delete(r.plannedIFrameGETURLs, frame)
	delete(r.iframeFormRequests, frame)
	pending := r.pendingIFrames[:0]
	for _, queued := range r.pendingIFrames {
		if queued != frame {
			pending = append(pending, queued)
		}
	}
	r.pendingIFrames = pending
}

func (r *Runtime) installLabelInterface(proto *goja.Object) {
	_ = proto.DefineAccessorProperty("control", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.wrapNode(r.labelControl(r.getNode(call.This)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("form", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		control := r.labelControl(r.getNode(call.This))
		if control == nil {
			return goja.Null()
		}
		return r.wrapNode(r.formOwner(control))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) labelControl(label *dom.Node) *dom.Node {
	if label == nil || label.Data != "label" || label.Doc == nil {
		return nil
	}
	if id, hasFor := nullNamespaceAttribute(label, "for"); hasFor {
		if id == "" {
			return nil
		}
		var target *dom.Node
		label.Doc.Node.Walk(func(node *dom.Node) bool {
			if node.Element() && node.GetAttribute("id") == id {
				target = node
				return false
			}
			return true
		})
		if target != nil && (isLabelableElement(target) || r.isFormAssociatedCustomElement(target)) {
			return target
		}
		return nil
	}
	var found *dom.Node
	var visit func(*dom.Node) bool
	visit = func(root *dom.Node) bool {
		for node := root; node != nil; node = node.NextSibling {
			if node != label && node.Element() && (isLabelableElement(node) || r.isFormAssociatedCustomElement(node)) {
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

func isLabelableElement(node *dom.Node) bool {
	if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
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

func (r *Runtime) checkFormValidity(form *dom.Node) bool {
	return r.checkFormValidityWithReport(form, false)
}

func (r *Runtime) checkFormValidityWithReport(form *dom.Node, report bool) bool {
	if form == nil || form.Data != "form" {
		return false
	}
	if r.formValidationInProgress[form] {
		return false
	}
	if r.formValidationInProgress == nil {
		r.formValidationInProgress = make(map[*dom.Node]bool)
	}
	r.formValidationInProgress[form] = true
	defer delete(r.formValidationInProgress, form)
	valid := true
	var firstInvalid *dom.Node
	dispatchInvalid := func(control *dom.Node) {
		if dom.DispatchEvent(control, dom.NewEvent("invalid", false, true)) && firstInvalid == nil {
			firstInvalid = control
		}
	}
	controls := r.listedFormControls(form)
	for _, control := range controls {
		if !r.formControlWillValidate(control) {
			continue
		}
		if r.isFormAssociatedCustomElement(control) {
			if state := r.elementInternals[control]; state != nil && !state.valid {
				valid = false
				dispatchInvalid(control)
			}
			continue
		}
		r.updateRangeValidity(control)
		if control.CustomValidityMessage != "" || r.inputTypeMismatch(control) || r.inputPatternMismatch(control) || control.RangeUnderflow || control.RangeOverflow || control.StepMismatch || dom.IsTooLong(control) || dom.IsTooShort(control) {
			valid = false
			dispatchInvalid(control)
			continue
		}
		if !control.HasAttribute("required") {
			continue
		}
		wrapper, _ := r.wrapNode(control).(*goja.Object)
		if wrapper == nil {
			continue
		}
		typ := strings.ToLower(control.GetAttribute("type"))
		if typ == "" {
			typ = "text"
		}
		invalid := false
		if control.Data == "select" {
			invalid = dom.SelectValueMissing(control)
		} else {
			switch typ {
			case "checkbox":
				invalid = !wrapper.Get("checked").ToBoolean()
			case "radio":
				// Required radio validity is group-scoped, not limited to the
				// controls currently enumerated for this form. The shared helper
				// applies the radio-button-group rules for tree scope, name, and
				// form owner, matching the individual ValidityState path.
				invalid = !r.radioGroupHasChecked(control)
			default:
				if typ != "button" && typ != "reset" && typ != "submit" && typ != "image" && typ != "hidden" {
					value := wrapper.Get("value")
					invalid = value == nil || goja.IsNull(value) || value.String() == ""
				}
			}
		}
		if invalid {
			valid = false
			dispatchInvalid(control)
		}
	}
	if report && firstInvalid != nil {
		r.focusInvalidControl(firstInvalid)
	}
	return valid
}

func (r *Runtime) focusInvalidControl(control *dom.Node) {
	if r == nil || control == nil || r.opts.OnFocusChange == nil {
		return
	}
	focusTarget := control
	if state := r.elementInternals[control]; state != nil {
		if anchor := r.getNode(state.validationAnchor); anchor != nil {
			focusTarget = anchor
		}
	}
	r.opts.OnFocusChange(focusTarget, true)
}

func (r *Runtime) formControlWillValidate(control *dom.Node) bool {
	if control == nil || dom.IsFormControlDisabled(control) || control.HasAttribute("readonly") {
		return false
	}
	for parent := control.Parent; parent != nil; parent = parent.Parent {
		if parent.Namespace == dom.NSHTML && parent.Data == "datalist" {
			return false
		}
	}
	if r.isFormAssociatedCustomElement(control) {
		for parent := control.Parent; parent != nil; parent = parent.Parent {
			if parent.Data == "datalist" {
				return false
			}
			if parent.Data == "fieldset" && parent.HasAttribute("disabled") {
				var firstLegend *dom.Node
				for child := parent.FirstChild; child != nil; child = child.NextSibling {
					if child.Element() && child.Namespace == dom.NSHTML && child.Data == "legend" {
						firstLegend = child
						break
					}
				}
				if firstLegend == nil || !containsNodeIncludingShadow(firstLegend, control) {
					return false
				}
			}
		}
		return true
	}
	switch control.Data {
	case "input":
		typ := strings.ToLower(control.GetAttribute("type"))
		switch typ {
		case "hidden", "button", "reset":
			return false
		}
		return true
	case "button":
		typ := strings.ToLower(control.GetAttribute("type"))
		return typ != "button" && typ != "reset"
	case "select", "textarea":
		return true
	default:
		return false
	}
}

func (r *Runtime) installFormActionReflection(proto *goja.Object) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("action") || node.GetAttribute("action") == "" {
			if documentURL := r.documentURL(); documentURL != nil {
				return r.vm.ToValue(documentURL.String())
			}
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.resolveURLString(node.GetAttribute("action")))
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			node.SetAttribute("action", call.Argument(0).String())
			r.notifyMutation()
		}
		return goja.Undefined()
	})
	_ = proto.DefineAccessorProperty("action", getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) newHTMLFormElementProxy(target *goja.Object, form *dom.Node) *goja.Object {
	controls := func() []*dom.Node { return r.listedFormControls(form) }
	pastNames := make(map[string][]*dom.Node)
	radioLists := make(map[string]goja.Value)
	itemsForName := func(name string) []*dom.Node {
		current := controls()
		for _, node := range current {
			if !node.Element() {
				continue
			}
			for _, key := range []string{node.GetAttribute("name"), node.GetAttribute("id")} {
				if key != "" {
					seen := false
					for _, old := range pastNames[key] {
						seen = seen || old == node
					}
					if !seen {
						pastNames[key] = append(pastNames[key], node)
					}
				}
			}
		}
		items := findNamedCollectionItems(current, name)
		if len(items) != 0 {
			return items
		}
		for _, old := range pastNames[name] {
			for _, node := range current {
				if old == node {
					items = append(items, old)
					break
				}
			}
		}
		return items
	}
	namedValue := func(name string) goja.Value {
		items := itemsForName(name)
		if len(items) == 0 {
			return nil
		}
		if len(items) == 1 {
			return r.wrapNode(items[0])
		}
		if list := radioLists[name]; list != nil {
			return list
		}
		list := r.newLiveRadioNodeList(func() []*dom.Node { return itemsForName(name) })
		radioLists[name] = list
		return list
	}
	getIndex := func(index int) goja.Value {
		nodes := controls()
		if index >= 0 && index < len(nodes) {
			return r.wrapNode(nodes[index])
		}
		return goja.Undefined()
	}
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				return getIndex(index)
			}
			if formTargetHasOwn(target, property) && !formNamedOverridesInternal(property) {
				return target.Get(property)
			}
			if value := namedValue(property); value != nil {
				return value
			}
			return target.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value { return getIndex(index) },
		Has: func(target *goja.Object, property string) bool {
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				return index >= 0 && index < len(controls())
			}
			return target.Get(property) != nil && !goja.IsUndefined(target.Get(property)) || namedValue(property) != nil
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(controls()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := controls()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapNode(nodes[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(_ *goja.Object) *goja.Object {
			keys := make([]interface{}, 0, len(controls()))
			for index := range controls() {
				keys = append(keys, strconv.Itoa(index))
			}
			return r.vm.NewArray(keys...)
		},
		DeletePropertyIdx: func(_ *goja.Object, _ int) bool { return false },
		GetOwnPropertyDescriptor: func(target *goja.Object, property string) goja.PropertyDescriptor {
			if formTargetHasOwn(target, property) {
				return goja.PropertyDescriptor{}
			}
			if value := namedValue(property); value != nil {
				return goja.PropertyDescriptor{Value: value, Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_FALSE, Configurable: goja.FLAG_TRUE}
			}
			return goja.PropertyDescriptor{}
		},
		Set: func(target *goja.Object, property string, value goja.Value, _ goja.Value) bool {
			if (!formTargetHasOwn(target, property) || formNamedOverridesInternal(property)) && namedValue(property) != nil {
				return false
			}
			return target.Set(property, value) == nil
		},
		DeleteProperty: func(target *goja.Object, property string) bool {
			if (!formTargetHasOwn(target, property) || formNamedOverridesInternal(property)) && namedValue(property) != nil {
				return false
			}
			return target.Delete(property) == nil
		},
		DefineProperty: func(target *goja.Object, property string, descriptor goja.PropertyDescriptor) bool {
			if (!formTargetHasOwn(target, property) || formNamedOverridesInternal(property)) && namedValue(property) != nil {
				return false
			}
			if descriptor.IsAccessor() {
				return target.DefineAccessorProperty(property, descriptor.Getter, descriptor.Setter, descriptor.Configurable, descriptor.Enumerable) == nil
			}
			return target.DefineDataProperty(property, descriptor.Value, descriptor.Writable, descriptor.Configurable, descriptor.Enumerable) == nil
		},
	})
	return r.vm.ToValue(proxy).(*goja.Object)
}

func formNamedOverridesInternal(property string) bool {
	switch property {
	case "__nid__", "nodeType", "showPopover", "hidePopover", "togglePopover":
		return true
	default:
		return false
	}
}

func formTargetHasOwn(target *goja.Object, name string) bool {
	for _, own := range target.GetOwnPropertyNames() {
		if own == name {
			return true
		}
	}
	return false
}

func (r *Runtime) installFormOwnerReflection(proto *goja.Object) {
	_ = proto.DefineAccessorProperty("form", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if owner := r.formOwner(r.getNode(call.This)); owner != nil {
			return r.wrapNode(owner)
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) installAutocompleteReflection(proto *goja.Object, form bool) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		value := strings.ToLower(strings.Join(strings.Fields(node.GetAttribute("autocomplete")), " "))
		if form {
			if value == "on" || value == "off" {
				return r.vm.ToValue(value)
			}
			return r.vm.ToValue("on")
		}
		if node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "hidden") {
			return r.vm.ToValue("")
		}
		if validAutocompleteTokens(value) {
			return r.vm.ToValue(value)
		}
		return r.vm.ToValue("")
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			node.SetAttribute("autocomplete", call.Argument(0).String())
			r.notifyMutation()
		}
		return goja.Undefined()
	})
	_ = proto.DefineAccessorProperty("autocomplete", getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func validAutocompleteTokens(value string) bool {
	if value == "on" || value == "off" {
		return true
	}
	tokens := strings.Fields(value)
	if len(tokens) == 0 || len(tokens) > 5 {
		return false
	}
	i := 0
	if strings.HasPrefix(tokens[i], "section-") && len(tokens[i]) > len("section-") {
		i++
	}
	if i < len(tokens) && (tokens[i] == "shipping" || tokens[i] == "billing") {
		i++
	}
	if i < len(tokens) && autocompleteContactHint(tokens[i]) {
		i++
	}
	if i >= len(tokens) || !autocompleteField(tokens[i]) {
		return false
	}
	i++
	if i < len(tokens) && tokens[i] == "webauthn" {
		i++
	}
	return i == len(tokens)
}

func autocompleteContactHint(value string) bool {
	switch value {
	case "home", "work", "mobile", "fax", "pager":
		return true
	default:
		return false
	}
}

func autocompleteField(value string) bool {
	switch value {
	case "name", "honorific-prefix", "given-name", "additional-name", "family-name", "honorific-suffix", "nickname", "username", "new-password", "current-password", "one-time-code", "organization-title", "organization", "street-address", "address-line1", "address-line2", "address-line3", "address-level4", "address-level3", "address-level2", "address-level1", "country", "country-name", "postal-code", "cc-name", "cc-given-name", "cc-additional-name", "cc-family-name", "cc-number", "cc-exp", "cc-exp-month", "cc-exp-year", "cc-csc", "cc-type", "transaction-currency", "transaction-amount", "language", "bday", "bday-day", "bday-month", "bday-year", "sex", "url", "photo", "tel", "tel-country-code", "tel-national", "tel-area-code", "tel-local", "tel-local-prefix", "tel-local-suffix", "tel-extension", "email", "impp", "webauthn":
		return true
	default:
		return false
	}
}

func (r *Runtime) listedFormControls(form *dom.Node) []*dom.Node {
	if form == nil || form.Doc == nil {
		return nil
	}
	var result []*dom.Node
	walkFormTreeScope(form.Doc, r.shadowRootForNode(form), func(node *dom.Node) bool {
		if node == form || (!isListedFormElement(node) && !r.isFormAssociatedCustomElement(node)) {
			return true
		}
		if r.formOwner(node) == form {
			result = append(result, node)
		}
		return true
	})
	return result
}

func isListedFormElement(node *dom.Node) bool {
	if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
		return false
	}
	switch node.Data {
	case "button", "fieldset", "input", "object", "output", "select", "textarea":
		if node.Data == "input" && node.GetAttribute("type") == "image" {
			return false
		}
		return true
	default:
		return false
	}
}

func isFormAssociatedElement(node *dom.Node) bool {
	if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
		return false
	}
	if node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "image") {
		return true
	}
	return isListedFormElement(node)
}

func (r *Runtime) formOwner(control *dom.Node) *dom.Node {
	if !isFormAssociatedElement(control) {
		if r.isFormAssociatedCustomElement(control) {
			return r.customFormOwner(control)
		}
		return nil
	}
	if id, hasFormAttribute := nullNamespaceAttribute(control, "form"); hasFormAttribute {
		if id == "" {
			return nil
		}
		var owner *dom.Node
		r.walkRadioGroupTreeScope(control, func(node *dom.Node) bool {
			formID, hasID := nullNamespaceAttribute(node, "id")
			if node.Element() && node.Namespace == dom.NSHTML && node.Data == "form" && hasID && formID == id {
				owner = node
				return false
			}
			return true
		})
		return owner
	}
	shadowRoot := r.shadowRootForNode(control)
	if control.FormOwner != nil && (shadowRoot == nil || nodeSubtreeContains(shadowRoot.FirstChild, control.FormOwner)) {
		return control.FormOwner
	}
	for parent := control.Parent; parent != nil; parent = parent.Parent {
		if shadowRoot != nil && parent == shadowRoot.Host {
			break
		}
		if parent.Element() && parent.Namespace == dom.NSHTML && parent.Data == "form" {
			return parent
		}
	}
	return nil
}

func (r *Runtime) walkRadioGroupTreeScope(control *dom.Node, visit func(*dom.Node) bool) {
	if control == nil || visit == nil {
		return
	}
	if shadowRoot := r.shadowRootForNode(control); shadowRoot != nil {
		walkFormTreeScope(control.Doc, shadowRoot, visit)
		return
	}
	if control.IsConnected() && control.Doc != nil {
		walkFormTreeScope(control.Doc, nil, visit)
		return
	}
	root := control
	for root.Parent != nil {
		root = root.Parent
	}
	root.Walk(visit)
}

// setInputCheckedness applies the checked IDL setter and radio-group behavior.
// The checked attribute remains the defaultChecked state and is only consulted
// for reset while the live checkedness is dirty.
func (r *Runtime) setInputCheckedness(control *dom.Node, checked bool) bool {
	if control == nil || control.Data != "input" {
		return false
	}
	changed := control.InputChecked() != checked
	if checked && strings.EqualFold(control.GetAttribute("type"), "radio") {
		name := control.GetAttribute("name")
		if name != "" {
			owner := r.formOwner(control)
			r.walkRadioGroupTreeScope(control, func(candidate *dom.Node) bool {
				if candidate != control && candidate.Data == "input" && strings.EqualFold(candidate.GetAttribute("type"), "radio") && candidate.GetAttribute("name") == name && r.formOwner(candidate) == owner && candidate.InputChecked() {
					candidate.SetInputChecked(false, true)
					changed = true
				}
				return true
			})
		}
	}
	control.SetInputChecked(checked, true)
	return changed
}

func (r *Runtime) radioButtonGroupMembers(control *dom.Node) []*dom.Node {
	if control == nil || control.Data != "input" || !strings.EqualFold(control.GetAttribute("type"), "radio") {
		return nil
	}
	name := control.GetAttribute("name")
	if name == "" {
		return []*dom.Node{control}
	}
	owner := r.formOwner(control)
	var members []*dom.Node
	r.walkRadioGroupTreeScope(control, func(candidate *dom.Node) bool {
		if candidate.Element() && candidate.Namespace == dom.NSHTML && candidate.Data == "input" &&
			strings.EqualFold(candidate.GetAttribute("type"), "radio") && candidate.GetAttribute("name") == name && r.formOwner(candidate) == owner {
			members = append(members, candidate)
		}
		return true
	})
	return members
}

func (r *Runtime) radioGroupHasChecked(control *dom.Node) bool {
	for _, member := range r.radioButtonGroupMembers(control) {
		if member.InputChecked() {
			return true
		}
	}
	return false
}

func (r *Runtime) markRadioGroupAttributeMutation(node *dom.Node, name, namespace string) {
	if r == nil || namespace != "" || node == nil {
		return
	}
	name = strings.ToLower(name)
	if node.Data == "input" && (name == "type" || strings.EqualFold(node.GetAttribute("type"), "radio") && (name == "checked" || name == "name" || name == "form")) {
		r.markRadioGroupTreeDirty(node)
	} else if node.Data == "form" && name == "id" {
		r.markRadioGroupTreeDirty(node)
	}
}

func (r *Runtime) markRadioGroupTreeDirty(node *dom.Node) {
	if r == nil || node == nil {
		return
	}
	if node.IsConnected() {
		r.radioGroupsDirty = true
		return
	}
	r.reconcileRadioGroupsForTree(node)
}

func (r *Runtime) markRadioGroupsDirtyForNodes(nodes []*dom.Node) {
	if r == nil {
		return
	}
	clearParserFormOwners(nodes)
	for _, node := range nodes {
		hasRadioRelevantNode := false
		walkParserFormOwnerTree([]*dom.Node{node}, func(current *dom.Node) {
			if current.Element() && current.Namespace == dom.NSHTML &&
				(current.Data == "form" && current.HasAttribute("id") || current.Data == "input" && strings.EqualFold(current.GetAttribute("type"), "radio")) {
				hasRadioRelevantNode = true
			}
		})
		if hasRadioRelevantNode {
			r.markRadioGroupTreeDirty(node)
		}
	}
}

func clearParserFormOwners(nodes []*dom.Node) {
	walkParserFormOwnerTree(nodes, func(node *dom.Node) {
		node.FormOwner = nil
	})
}

// walkParserFormOwnerTree visits each node at most once. Parser-driven DOM
// mutations can temporarily produce cyclic child/parent links, so invalidation
// walks must not assume the graph is a tree.
func walkParserFormOwnerTree(nodes []*dom.Node, visit func(*dom.Node)) {
	pending := append([]*dom.Node(nil), nodes...)
	seen := make(map[*dom.Node]struct{})
	for len(pending) > 0 {
		last := len(pending) - 1
		current := pending[last]
		pending = pending[:last]
		if current == nil {
			continue
		}
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		visit(current)
		pending = append(pending, current.NextSibling, current.FirstChild)
		if current.ShadowRoot != nil {
			pending = append(pending, current.ShadowRoot.FirstChild)
		}
	}
}

func (r *Runtime) reconcileRadioGroupsForTree(control *dom.Node) {
	if r == nil || control == nil {
		return
	}
	type key struct {
		form *dom.Node
		name string
	}
	groups := make(map[key]*dom.Node)
	r.walkRadioGroupTreeScope(control, func(node *dom.Node) bool {
		if !node.Element() || node.Namespace != dom.NSHTML || node.Data != "input" || !strings.EqualFold(node.GetAttribute("type"), "radio") || !node.InputChecked() {
			return true
		}
		name := node.GetAttribute("name")
		if name == "" {
			return true
		}
		form := r.formOwner(node)
		// Inserting already-checked radios into a detached tree does not run
		// checkedness normalization for radios with no form owner. Their tree
		// root still matters for validity and for later checked IDL writes, but
		// insertion alone must not clear another radio's checkedness. A
		// detached form does have a form owner and retains group normalization.
		if form == nil {
			return true
		}
		group := key{form: form, name: name}
		if earlier := groups[group]; earlier != nil {
			earlier.SetInputChecked(false, true)
		}
		groups[group] = node
		return true
	})
}

func (r *Runtime) reconcileDirtyRadioGroups() {
	if r == nil || !r.radioGroupsDirty {
		return
	}
	r.radioGroupsDirty = false
	r.reconcileConnectedRadioGroups()
}

func (r *Runtime) reconcileConnectedRadioGroups() {
	if r == nil || r.domDoc == nil {
		return
	}
	type key struct {
		treeScope *dom.ShadowRoot
		form      *dom.Node
		name      string
	}
	var reconcileScope func(*dom.ShadowRoot)
	reconcileScope = func(shadowRoot *dom.ShadowRoot) {
		groups := make(map[key]*dom.Node)
		var nestedRoots []*dom.ShadowRoot
		walkFormTreeScope(r.domDoc, shadowRoot, func(node *dom.Node) bool {
			if node.ShadowRoot != nil {
				nestedRoots = append(nestedRoots, node.ShadowRoot)
			}
			if !node.Element() || node.Namespace != dom.NSHTML || node.Data != "input" || !strings.EqualFold(node.GetAttribute("type"), "radio") || !node.IsConnected() || !node.InputChecked() {
				return true
			}
			name := node.GetAttribute("name")
			if name == "" {
				return true
			}
			group := key{treeScope: shadowRoot, form: r.formOwner(node), name: name}
			if earlier := groups[group]; earlier != nil {
				earlier.SetInputChecked(false, true)
			}
			groups[group] = node
			return true
		})
		for _, nested := range nestedRoots {
			reconcileScope(nested)
		}
	}
	reconcileScope(nil)
}

func nullNamespaceAttribute(node *dom.Node, name string) (string, bool) {
	if node == nil {
		return "", false
	}
	for _, attr := range node.Attr {
		if attr.NamespaceURI == "" && attr.Namespace == dom.NSHTML && attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (r *Runtime) shadowRootForNode(target *dom.Node) *dom.ShadowRoot {
	if target == nil {
		return nil
	}
	for parent := target.Parent; parent != nil; parent = parent.Parent {
		root := parent.ShadowRoot
		if root != nil && nodeSubtreeContains(root.FirstChild, target) {
			return root
		}
	}
	return nil
}

func nodeSubtreeContains(root, target *dom.Node) bool {
	for node := root; node != nil; node = node.NextSibling {
		if node == target || nodeSubtreeContains(node.FirstChild, target) {
			return true
		}
	}
	return false
}

func walkFormTreeScope(doc *dom.Document, shadowRoot *dom.ShadowRoot, visit func(*dom.Node) bool) {
	if shadowRoot != nil {
		var walk func(*dom.Node) bool
		walk = func(parent *dom.Node) bool {
			for node := parent; node != nil; node = node.NextSibling {
				if !visit(node) || !walk(node.FirstChild) {
					return false
				}
			}
			return true
		}
		walk(shadowRoot.FirstChild)
		return
	}
	if doc != nil {
		doc.Node.Walk(visit)
	}
}

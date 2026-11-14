package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type documentWriteState struct {
	input            strings.Builder
	generated        []*dom.Node
	object           *goja.Object
	lastAppliedInput string
	hasAppliedInput  bool
	executedScripts  map[int]bool
}

func documentWriteParseLimits() dom.ParseLimits {
	return dom.ParseLimits{Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096}
}

func stateInput(state *documentWriteState) string {
	if state == nil {
		return ""
	}
	return state.input.String()
}

func (r *Runtime) runtimeForDocumentWrite(doc *dom.Document) *Runtime {
	if r == nil || doc == nil {
		return r
	}
	if owner := r.runtimeForDocument(doc); owner != nil {
		return owner
	}
	return r
}

func (r *Runtime) documentOpen(doc *dom.Document, object *goja.Object) goja.Value {
	if doc == nil {
		return goja.Undefined()
	}
	if owner := r.runtimeForDocumentWrite(doc); owner != nil && owner != r {
		childObject, _ := owner.vm.Get("document").(*goja.Object)
		owner.documentOpen(doc, childObject)
		return r.wrapNode(&doc.Node)
	}
	r.throwIfDynamicMarkupInsertionCounter(doc)
	r.removeDocumentChildren(doc)
	if r.documentWrites == nil {
		r.documentWrites = make(map[*dom.Document]*documentWriteState)
	}
	r.documentWrites[doc] = &documentWriteState{object: object, executedScripts: make(map[int]bool)}
	return r.wrapNode(&doc.Node)
}

func (r *Runtime) documentClose(doc *dom.Document) goja.Value {
	if owner := r.runtimeForDocumentWrite(doc); owner != nil && owner != r {
		return owner.documentClose(doc)
	}
	if doc != nil {
		r.throwIfDynamicMarkupInsertionCounter(doc)
		if state := r.documentWrites[doc]; state != nil {
			input := state.input.String()
			needsEOFParse := !state.hasAppliedInput || input != state.lastAppliedInput || doc.HTML == nil || doc.Body == nil
			if needsEOFParse {
				if parsed, err := dom.ParseBounded(input, documentWriteParseLimits()); err == nil {
					applyWrittenDocument(doc, parsed, state)
					if !state.hasAppliedInput || input != state.lastAppliedInput {
						r.activateWrittenParserContent(doc, state)
					}
					state.lastAppliedInput, state.hasAppliedInput = input, true
				}
			}
			delete(r.documentWrites, doc)
		}
	}
	return goja.Undefined()
}

func (r *Runtime) writeToDocument(doc *dom.Document, object *goja.Object, call goja.FunctionCall, newline bool) goja.Value {
	if doc == nil {
		return goja.Undefined()
	}
	owner := r.runtimeForDocumentWrite(doc)
	if owner == r {
		if frame := r.iframeForDocument(doc); frame != nil && r.iframeChildRuntimes[frame] == nil {
			var text strings.Builder
			for _, arg := range call.Arguments {
				if arg != nil {
					text.WriteString(arg.String())
				}
			}
			if newline {
				text.WriteByte('\n')
			}
			written := text.String()
			call = goja.FunctionCall{Arguments: []goja.Value{r.vm.ToValue(written)}}
			state := r.documentWrites[doc]
			if strings.Contains(strings.ToLower(stateInput(state)+written), "<script") {
				r.installExternalIFrameRuntime(frame, doc, "about:blank")
				if child := r.iframeChildRuntimes[frame]; child != nil {
					childObject, _ := child.vm.Get("document").(*goja.Object)
					if state != nil {
						delete(r.documentWrites, doc)
						state.object = childObject
						if child.documentWrites == nil {
							child.documentWrites = make(map[*dom.Document]*documentWriteState)
						}
						child.documentWrites[doc] = state
					}
					return child.writeToDocument(doc, childObject, call, false)
				}
			}
			newline = false
		}
	}
	if owner != nil && owner != r {
		childObject, _ := owner.vm.Get("document").(*goja.Object)
		return owner.writeToDocument(doc, childObject, call, newline)
	}
	r.throwIfDynamicMarkupInsertionCounter(doc)
	if r.documentWrites == nil {
		r.documentWrites = make(map[*dom.Document]*documentWriteState)
	}
	state := r.documentWrites[doc]
	if state == nil {
		r.documentOpen(doc, object)
		if owner := r.runtimeForDocumentWrite(doc); owner != nil && owner != r {
			childObject, _ := owner.vm.Get("document").(*goja.Object)
			return owner.writeToDocument(doc, childObject, call, newline)
		}
		state = r.documentWrites[doc]
		if state == nil {
			return goja.Undefined()
		}
	}
	for _, arg := range call.Arguments {
		if arg != nil {
			state.input.WriteString(arg.String())
		}
	}
	if newline {
		state.input.WriteByte('\n')
	}
	var parserScripts []*dom.Node
	parsed, err := dom.ParseOpenBoundedWithParserCallbacksAndScripts(state.input.String(), documentWriteParseLimits(), func(node *dom.Node, attrs []dom.Attribute) *dom.Node {
		if r.customElementNodeRegistries == nil {
			r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
		}
		registry := r.customElementRegistryForDocument(doc)
		if node.Doc != nil && node.Doc.IsTemplateContentsOwnerDocument() {
			registry = nil
		}
		for _, attr := range attrs {
			if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" && strings.EqualFold(attr.Name, "customelementregistry") {
				registry = nil
				if r.customElementNullRegistryLocks == nil {
					r.customElementNullRegistryLocks = make(map[*dom.Node]bool)
				}
				r.customElementNullRegistryLocks[node] = true
				break
			}
		}
		r.customElementNodeRegistries[node] = registry
		return r.UpgradeParserElementWithAttributesInDocument(node, attrs, doc)
	}, func(node *dom.Node, attr dom.Attribute) {
		r.ParserElementAttributeChangedInDocument(node, attr, doc)
	}, func(node *dom.Node) {
		r.ParserNodeInsertedInDocument(node, doc)
	}, func(_ *dom.Document, node *dom.Node) {
		parserScripts = append(parserScripts, node)
	})
	if err != nil {
		return goja.Undefined()
	}
	applyWrittenDocument(doc, parsed, state)
	r.activateWrittenParserContent(doc, state)
	state.lastAppliedInput, state.hasAppliedInput = state.input.String(), true
	r.notifyMutation()
	r.runWrittenParserScripts(parserScripts, state)
	return goja.Undefined()
}

func (r *Runtime) runWrittenParserScripts(scripts []*dom.Node, state *documentWriteState) {
	if state == nil {
		return
	}
	if state.executedScripts == nil {
		state.executedScripts = make(map[int]bool)
	}
	for i, node := range scripts {
		if node == nil || state.executedScripts[i] {
			continue
		}
		state.executedScripts[i] = true
		if err := r.RunParserScript(node.TextContent(), "document.write"); err != nil {
			continue
		}
	}
}

func (r *Runtime) activateWrittenParserContent(doc *dom.Document, state *documentWriteState) {
	if doc == nil || state == nil {
		return
	}
	for _, root := range state.generated {
		var visit func(*dom.Node)
		visit = func(node *dom.Node) {
			if node == nil {
				return
			}
			if node.Element() {
				r.UpgradeParserElement(node)
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(root)
		visit = func(node *dom.Node) {
			if node == nil {
				return
			}
			if node.Element() {
				r.ParserElementInserted(node)
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(root)
	}
}

func (r *Runtime) throwIfDynamicMarkupInsertionCounter(doc *dom.Document) {
	if r != nil && doc != nil && r.dynamicMarkupInsertionCounters[doc] > 0 {
		panic(r.domError("InvalidStateError", "Dynamic markup insertion is not allowed during synchronous custom element reactions."))
	}
}

func (r *Runtime) removeDocumentChildren(doc *dom.Document) {
	if doc == nil {
		return
	}
	var old []*dom.Node
	for child := doc.Node.FirstChild; child != nil; child = child.NextSibling {
		old = append(old, child)
	}
	wasConnected := captureConnectedState(old)
	for _, child := range old {
		doc.Node.RemoveChild(child)
	}
	r.notifyDisconnected(old, wasConnected)
	r.releaseNodes(old)
	doc.HTML, doc.Head, doc.Body, doc.DocType = nil, nil, nil, nil
	doc.QuirksMode, doc.LimitedQuirksMode = false, false
	doc.MarkQueryDirty()
}

func applyWrittenDocument(dst, parsed *dom.Document, state *documentWriteState) {
	dst.CharacterSet = parsed.CharacterSet
	for _, n := range state.generated {
		if n.Parent == &dst.Node {
			dst.Node.RemoveChild(n)
		}
	}
	state.generated = nil
	manualDoctype, manualElement := false, false
	for n := dst.Node.FirstChild; n != nil; n = n.NextSibling {
		manualDoctype = manualDoctype || n.Type == dom.NodeDoctype
		manualElement = manualElement || n.Element()
	}
	if parsed.DocType != nil || !manualDoctype {
		dst.QuirksMode = parsed.QuirksMode
		dst.LimitedQuirksMode = parsed.LimitedQuirksMode
	}
	for child := parsed.Node.FirstChild; child != nil; {
		next := child.NextSibling
		if (child.Type == dom.NodeDoctype && (manualDoctype || manualElement)) || (child.Element() && manualElement) {
			child = next
			continue
		}
		parsed.Node.RemoveChild(child)
		dst.Node.AppendChild(child)
		state.generated = append(state.generated, child)
		if child.Type == dom.NodeDoctype {
			dst.DocType = child
		}
		if child.Element() {
			dst.HTML = child
			child.Walk(func(n *dom.Node) bool {
				if n.Element() && n.Data == "head" && dst.Head == nil {
					dst.Head = n
				}
				if n.Element() && n.Data == "body" && dst.Body == nil {
					dst.Body = n
				}
				return true
			})
		}
		child = next
	}
	dst.MarkQueryDirty()
}

func (r *Runtime) documentWrite(call goja.FunctionCall, appendNewline bool) goja.Value {
	if r.domDoc == nil {
		return goja.Undefined()
	}
	r.throwIfDynamicMarkupInsertionCounter(r.domDoc)
	if state := r.documentWrites[r.domDoc]; state != nil {
		obj, _ := r.vm.Get("document").(*goja.Object)
		return r.writeToDocument(r.domDoc, obj, call, appendNewline)
	}

	var text strings.Builder
	for _, arg := range call.Arguments {
		text.WriteString(arg.String())
	}
	written := text.String()
	if appendNewline {
		written += "\n"
	}

	if written == "" {
		return goja.Undefined()
	}

	if r.readyState() == ReadyLoading {
		if r.parserWriteDepth > 0 {
			r.parserWriteBuffer.WriteString(written)
			if r.flushCompleteParserWrite() {
				r.notifyMutation()
			}
			return goja.Undefined()
		}
		r.writeDuringParsing(written)
	} else {
		r.writeAfterParsing(written)
	}

	r.notifyMutation()
	return goja.Undefined()
}

func (r *Runtime) flushCompleteParserWrite() bool {
	if r == nil || r.domDoc == nil || r.parserWriteBuffer.Len() == 0 {
		return false
	}
	r.ensureDocumentStructure()
	target := r.domDoc.Body
	if target == nil {
		target = r.domDoc.HTML
	}
	if target == nil {
		return false
	}
	input := r.parserWriteBuffer.String()
	complete, err := dom.HTMLFragmentInputComplete(target, input, documentWriteParseLimits())
	if err != nil || !complete {
		return false
	}
	r.parserWriteBuffer.Reset()
	r.writeDuringParsing(input)
	return true
}

func (r *Runtime) RunParserScript(src, sourceURL string) error {
	if r.scriptExecutionDepth == 0 {
		if err := r.drainMicrotasks(); err != nil {
			return err
		}
	}
	r.parserWriteDepth++
	err := r.Run(stripHTMLScriptComments(src), sourceURL)
	r.parserWriteDepth--
	if r.parserWriteDepth == 0 && r.parserWriteBuffer.Len() != 0 {
		written := r.parserWriteBuffer.String()
		r.parserWriteBuffer.Reset()
		if r.readyState() == ReadyLoading {
			r.writeDuringParsing(written)
			r.notifyMutation()
		}
	}
	return err
}

func stripHTMLScriptComments(src string) string {
	var out strings.Builder
	out.Grow(len(src))
	for len(src) > 0 {
		end := strings.IndexByte(src, '\n')
		line := src
		if end >= 0 {
			line, src = src[:end], src[end+1:]
		} else {
			src = ""
		}
		trimmed := strings.TrimLeft(line, " \t\r")
		if strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, "-->") {
			line = line[:len(line)-len(trimmed)]
		}
		out.WriteString(line)
		if end >= 0 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func (r *Runtime) RunParserDeferScript(src, sourceURL string) error {
	previous := r.inDeferScript
	r.inDeferScript = true
	defer func() { r.inDeferScript = previous }()
	return r.RunParserScript(src, sourceURL)
}

func (r *Runtime) ensureDocumentStructure() {
	if r.domDoc.HTML == nil {
		htmlElem := r.domDoc.NewElement("html")
		r.domDoc.Node.AppendChild(htmlElem)
		r.domDoc.HTML = htmlElem
	}
	if r.domDoc.Head == nil {
		headElem := r.domDoc.NewElement("head")
		r.domDoc.HTML.AppendChild(headElem)
		r.domDoc.Head = headElem
	}
	if r.domDoc.Body == nil {
		bodyElem := r.domDoc.NewElement("body")
		r.domDoc.HTML.AppendChild(bodyElem)
		r.domDoc.Body = bodyElem
	}
}

func (r *Runtime) writeDuringParsing(html string) {
	r.ensureDocumentStructure()

	target := r.domDoc.Body
	if target == nil {
		target = r.domDoc.HTML
	}
	if target == nil {
		target = &r.domDoc.Node
	}

	r.parseAndAppend(html, target)

	r.updateDocumentRefs()
}

func (r *Runtime) writeAfterParsing(html string) {
	r.clearDocument()
	r.ensureDocumentStructure()
	if r.domDoc.Body != nil {
		r.parseAndAppend(html, r.domDoc.Body)
	}
	r.updateDocumentRefs()
}

func (r *Runtime) updateDocumentRefs() {
	doc := r.vm.Get("document")
	if doc == nil {
		return
	}
	docObj, ok := doc.(*goja.Object)
	if !ok {
		return
	}
	if r.domDoc.Body != nil {
		_ = docObj.Set("body", r.wrapNode(r.domDoc.Body))
	}
	if r.domDoc.HTML != nil {
		_ = docObj.Set("documentElement", r.wrapNode(r.domDoc.HTML))
	}
	if r.domDoc.Head != nil {
		_ = docObj.Set("head", r.wrapNode(r.domDoc.Head))
	}
}

func (r *Runtime) clearDocument() {
	var old []*dom.Node
	for c := r.domDoc.Node.FirstChild; c != nil; c = c.NextSibling {
		old = append(old, c)
	}
	wasConnected := captureConnectedState(old)
	for _, c := range old {
		r.domDoc.Node.RemoveChild(c)
	}
	r.notifyDisconnected(old, wasConnected)
	r.releaseNodes(old)

	r.domDoc.HTML = nil
	r.domDoc.Head = nil
	r.domDoc.Body = nil
	r.domDoc.DocType = nil
	r.domDoc.MarkQueryDirty()
}

func (r *Runtime) parseAndAppend(html string, target *dom.Node) {
	if !strings.Contains(html, "<") {
		textNode := r.domDoc.NewText(html)
		r.admitInsert("document.write", target, []*dom.Node{textNode}, func() {
			target.AppendChild(textNode)
		})
		return
	}

	if target == nil || !target.Element() {
		return
	}
	parsed, err := r.parseDocumentWriteFragment(target, html)
	if err != nil {
		textNode := r.domDoc.NewText(html)
		r.admitInsert("document.write", target, []*dom.Node{textNode}, func() {
			target.AppendChild(textNode)
		})
		return
	}

	var newChildren []*dom.Node
	for c := parsed.FirstChild; c != nil; c = c.NextSibling {
		newChildren = append(newChildren, c)
	}

	if len(newChildren) > 0 {
		r.admitInsert("document.write", target, newChildren, func() {
			for _, c := range newChildren {
				parsed.RemoveChild(c)
				target.AppendChild(c)
			}
		})
	}
}

func (r *Runtime) parseDocumentWriteFragment(target *dom.Node, html string) (*dom.Node, error) {
	return dom.ParseFragmentBoundedWithParserCallbacks(target, html, dom.ParseLimits{
		Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096,
	}, func(node *dom.Node, attrs []dom.Attribute) *dom.Node {
		if r.customElementNodeRegistries == nil {
			r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
		}
		r.customElementNodeRegistries[node] = r.documentWriteRegistry(target)
		return r.UpgradeParserElementWithAttributes(node, attrs)
	}, func(node *dom.Node, attr dom.Attribute) {
		r.ParserElementAttributeChanged(node, attr)
	}, func(node *dom.Node) {
		r.ParserNodeInserted(node)
		r.customElementNodeRegistries[node] = r.documentWriteRegistry(target)
		if target.IsConnected() {
			if def := r.customElementDefinitionForNode(node); def != nil {
				r.withDynamicMarkupInsertionCounter(r.domDoc, func() { r.invokeConnectedCallback(node) })
			}
		}
	})
}

func (r *Runtime) documentWriteRegistry(target *dom.Node) *CustomElementRegistry {
	registry := r.customElementRegistryForNode(target)
	if registry == nil && target != nil {
		registry = r.customElementRegistryForDocument(target.Doc)
	}
	return registry
}

func (r *Runtime) setupDocumentWrite() {
	doc := r.vm.Get("document")
	if doc == nil {
		return
	}
	docObj, ok := doc.(*goja.Object)
	if !ok {
		return
	}

	if r.opts.DOM == nil {
		_ = docObj.Set("write", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		_ = docObj.Set("writeln", func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		})
		_ = docObj.Set("open", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = docObj.Set("close", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		return
	}
	_ = docObj.Set("open", func(call goja.FunctionCall) goja.Value {
		return r.documentOpenCall(r.domDoc, docObj, call)
	})
	_ = docObj.Set("close", func(goja.FunctionCall) goja.Value {
		return r.documentClose(r.domDoc)
	})

	_ = docObj.Set("write", func(call goja.FunctionCall) goja.Value {
		return r.documentWrite(call, false)
	})
	_ = docObj.Set("writeln", func(call goja.FunctionCall) goja.Value {
		return r.documentWrite(call, true)
	})
}

func (r *Runtime) documentOpenCall(doc *dom.Document, object *goja.Object, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) > 1 {
		if frame := r.iframeForDocument(doc); frame != nil && r.iframeChildRuntimes[frame] == nil {
			return r.iframeWindowOpenCall(frame, call)
		}
		owner := r.runtimeForDocument(doc)
		if owner == nil {
			owner = r
		}
		return owner.openWindowCall(call)
	}
	return r.documentOpen(doc, object)
}

func (r *Runtime) runtimeForDocument(doc *dom.Document) *Runtime {
	if r == nil || doc == nil {
		return nil
	}
	if r.domDoc == doc {
		return r
	}
	for frame, candidate := range r.iframeDocuments {
		if candidate == doc {
			if child := r.iframeChildRuntimes[frame]; child != nil {
				return child
			}
			return r
		}
		if child := r.iframeChildRuntimes[frame]; child != nil {
			if runtime := child.runtimeForDocument(doc); runtime != nil {
				return runtime
			}
		}
	}
	return nil
}

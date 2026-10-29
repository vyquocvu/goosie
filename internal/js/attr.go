package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type jsAttribute struct {
	object    *goja.Object
	name      string
	namespace string
	value     string
	owner     *dom.Node
	document  *dom.Document
}

func (r *Runtime) newJSAttribute(name, namespace string, ownerDocuments ...*dom.Document) goja.Value {
	ownerDocument := r.domDoc
	if len(ownerDocuments) > 0 {
		ownerDocument = ownerDocuments[0]
	}
	attribute := &jsAttribute{name: name, namespace: namespace, document: ownerDocument}
	obj := r.vm.NewObject()
	if r.attrProto != nil {
		_ = obj.SetPrototype(r.attrProto)
	}
	attribute.object = obj
	_ = obj.Set("__xmlAttributeNode", true)
	_ = obj.Set("name", name)
	_ = obj.Set("nodeName", name)
	_ = obj.Set("nodeType", 2)
	_ = obj.Set("namespaceURI", namespaceValue(r.vm, namespace))
	localName := name
	prefix := ""
	if colon := strings.IndexByte(name, ':'); colon >= 0 {
		prefix, localName = name[:colon], name[colon+1:]
	}
	_ = obj.Set("localName", localName)
	_ = obj.Set("prefix", namespaceValue(r.vm, prefix))
	_ = obj.DefineAccessorProperty("value", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(attribute.currentValue())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := ""
		if len(call.Arguments) > 0 {
			value = call.Arguments[0].String()
		}
		r.setJSAttributeValue(attribute, value)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	for _, alias := range []string{"nodeValue", "textContent"} {
		_ = obj.DefineAccessorProperty(alias, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(attribute.currentValue())
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			value := ""
			if len(call.Arguments) > 0 && !goja.IsNull(call.Arguments[0]) && !goja.IsUndefined(call.Arguments[0]) {
				value = call.Arguments[0].String()
			}
			r.setJSAttributeValue(attribute, value)
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = obj.DefineAccessorProperty("ownerElement", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		attribute.currentValue()
		if attribute.owner == nil {
			return goja.Null()
		}
		return r.wrapNode(attribute.owner)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("ownerDocument", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		ownerDocument := attribute.currentOwnerDocument()
		if ownerDocument == nil {
			return goja.Null()
		}
		return r.wrapNode(&ownerDocument.Node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("specified", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(true)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	if r.attributeValues == nil {
		r.attributeValues = make(map[*goja.Object]*jsAttribute)
	}
	r.attributeValues[obj] = attribute
	return obj
}

func (a *jsAttribute) currentOwnerDocument() *dom.Document {
	if a == nil {
		return nil
	}
	if a.owner != nil && a.owner.Doc != nil {
		return a.owner.Doc
	}
	return a.document
}

func (a *jsAttribute) currentValue() string {
	if a == nil {
		return ""
	}
	if a.owner == nil {
		return a.value
	}
	for _, attr := range a.owner.Attr {
		if attr.NamespaceURI == a.namespace && strings.EqualFold(attr.Name, a.name) {
			a.value = attr.Value
			return a.value
		}
	}
	a.owner = nil
	return a.value
}

func attributeNodeKey(node *dom.Node, namespace, name string) string {
	if node != nil && node.Namespace == dom.NSHTML {
		name = strings.ToLower(name)
	}
	return namespace + "\x00" + name
}

func (r *Runtime) jsAttributeForNode(node *dom.Node, attr dom.Attribute) goja.Value {
	key := attributeNodeKey(node, attr.NamespaceURI, attr.Name)
	if r.attributeNodes[node] == nil {
		r.attributeNodes[node] = make(map[string]*goja.Object)
	}
	if obj := r.attributeNodes[node][key]; obj != nil {
		if state := r.attributeValues[obj]; state != nil {
			state.owner = node
			state.value = attr.Value
			return obj
		}
	}
	obj := r.newJSAttribute(attr.Name, attr.NamespaceURI, node.Doc).(*goja.Object)
	state := r.attributeValues[obj]
	state.owner, state.value = node, attr.Value
	r.attributeNodes[node][key] = obj
	return obj
}

func (r *Runtime) jsGetAttributeNode(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) == 0 {
		return goja.Null()
	}
	name := call.Argument(0).String()
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Name, name) {
			return r.jsAttributeForNode(node, attr)
		}
	}
	return goja.Null()
}

func (r *Runtime) jsGetAttributeNodeNS(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'getAttributeNodeNS': 2 arguments required"))
	}
	namespace := ""
	if value := call.Argument(0); value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
		namespace = value.String()
	}
	localName := call.Argument(1).String()
	for _, attr := range node.Attr {
		if attr.NamespaceURI == namespace && domAttributeLocalName(attr.Name) == localName {
			return r.jsAttributeForNode(node, attr)
		}
	}
	return goja.Null()
}

func namespaceValue(vm *goja.Runtime, value string) goja.Value {
	if value == "" {
		return goja.Null()
	}
	return vm.ToValue(value)
}

func (r *Runtime) setJSAttributeValue(attribute *jsAttribute, value string) {
	if attribute == nil {
		return
	}
	old, had := attribute.value, true
	old = attribute.currentValue()
	attribute.value = value
	if attribute.owner == nil {
		return
	}
	attribute.owner.SetAttributeNS(attribute.namespace, attribute.name, value)
	r.notifyAttributeMutation(attribute.owner, attribute.namespace, domAttributeLocalName(attribute.name), &old)
	if old == value {
		return
	}
	r.RefreshInputPatternMismatch(attribute.owner)
	r.notifyCustomElementAttributeChanged(attribute.owner, domAttributeLocalName(attribute.name), old, had, value, true, attribute.namespace)
	r.notifyMutation()
}

func (r *Runtime) jsSetAttributeNode(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'setAttributeNode': 1 argument required"))
	}
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok || r.attributeValues[obj] == nil {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Attr'"))
	}
	attribute := r.attributeValues[obj]
	if attribute.owner != nil && attribute.owner != node {
		panic(r.domError("InUseAttributeError", "The attribute is already in use by another element"))
	}
	oldValue, hadOld := "", false
	for _, existing := range node.Attr {
		if existing.NamespaceURI == attribute.namespace && strings.EqualFold(domAttributeLocalName(existing.Name), domAttributeLocalName(attribute.name)) {
			oldValue, hadOld = existing.Value, true
			break
		}
	}
	if hadOld && oldValue == attribute.value && strings.EqualFold(node.GetAttribute(attribute.name), attribute.value) {
		attribute.owner = node
		attribute.document = node.Doc
		return goja.Null()
	}
	node.SetAttributeNS(attribute.namespace, attribute.name, attribute.value)
	r.RefreshInputPatternMismatch(node)
	attribute.owner = node
	attribute.document = node.Doc
	if r.attributeNodes[node] == nil {
		r.attributeNodes[node] = make(map[string]*goja.Object)
	}
	r.attributeNodes[node][attributeNodeKey(node, attribute.namespace, attribute.name)] = obj
	var old *string
	if hadOld {
		old = &oldValue
	}
	r.notifyAttributeMutation(node, attribute.namespace, domAttributeLocalName(attribute.name), old)
	r.notifyCustomElementAttributeChanged(node, domAttributeLocalName(attribute.name), oldValue, hadOld, attribute.value, true, attribute.namespace)
	r.notifyMutation()
	return goja.Null()
}

func (r *Runtime) jsRemoveAttributeNode(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'removeAttributeNode': 1 argument required"))
	}
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok || r.attributeValues[obj] == nil {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Attr'"))
	}
	attribute := r.attributeValues[obj]
	if attribute.owner != node {
		panic(r.domError("NotFoundError", "The attribute is not attached to this element"))
	}
	oldValue, hadOld := "", false
	for _, existing := range node.Attr {
		if existing.NamespaceURI == attribute.namespace && strings.EqualFold(domAttributeLocalName(existing.Name), domAttributeLocalName(attribute.name)) {
			oldValue, hadOld = existing.Value, true
			break
		}
	}
	if !node.RemoveAttributeNS(attribute.namespace, domAttributeLocalName(attribute.name)) {
		panic(r.domError("NotFoundError", "The attribute is not attached to this element"))
	}
	r.RefreshInputPatternMismatch(node)
	attribute.owner = nil
	attribute.document = node.Doc
	delete(r.attributeNodes[node], attributeNodeKey(node, attribute.namespace, attribute.name))
	old := oldValue
	r.notifyAttributeMutation(node, attribute.namespace, domAttributeLocalName(attribute.name), &old)
	r.notifyCustomElementAttributeChanged(node, domAttributeLocalName(attribute.name), oldValue, hadOld, "", false, attribute.namespace)
	r.notifyMutation()
	return obj
}

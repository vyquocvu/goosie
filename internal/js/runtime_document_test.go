package js

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestRuntimeForDocumentReturnsOnlyAnOwningBrowsingContext(t *testing.T) {
	parentDocument := dom.NewDocument()
	iframeDocument := dom.NewDocument()
	childDocument := dom.NewDocument()
	unrelatedDocument := dom.NewDocument()
	frame := parentDocument.NewElement("iframe")
	child := &Runtime{domDoc: childDocument}
	parent := &Runtime{
		domDoc:              parentDocument,
		iframeDocuments:     map[*dom.Node]*dom.Document{frame: iframeDocument},
		iframeChildRuntimes: map[*dom.Node]*Runtime{frame: child},
	}

	if got := parent.runtimeForDocument(childDocument); got != child {
		t.Fatalf("runtimeForDocument(child document) = %p, want child runtime %p", got, child)
	}
	if got := parent.runtimeForDocument(unrelatedDocument); got != nil {
		t.Fatalf("runtimeForDocument(unrelated document) = %p, want nil", got)
	}
}

func TestImportChildDOMExceptionPreservesSourceRealm(t *testing.T) {
	parent, err := New(Options{DOM: dom.NewDocument()})
	if err != nil {
		t.Fatalf("create parent runtime: %v", err)
	}
	defer parent.Close()
	child, err := New(Options{DOM: dom.NewDocument()})
	if err != nil {
		t.Fatalf("create child runtime: %v", err)
	}
	defer child.Close()

	constructor, ok := goja.AssertConstructor(child.vm.Get("DOMException"))
	if !ok {
		t.Fatal("child DOMException is not constructible")
	}
	exception, err := constructor(nil, child.vm.ToValue("guarded"), child.vm.ToValue("InvalidStateError"))
	if err != nil {
		t.Fatalf("construct child DOMException: %v", err)
	}
	imported := parent.importChildValue(child, exception, make(map[*goja.Object]*goja.Object), 0)
	if err := parent.vm.Set("crossRealmException", imported); err != nil {
		t.Fatalf("set imported exception: %v", err)
	}
	childConstructor := parent.importChildValue(child, child.vm.Get("DOMException"), make(map[*goja.Object]*goja.Object), 0)
	if err := parent.vm.Set("ChildDOMException", childConstructor); err != nil {
		t.Fatalf("set imported child constructor: %v", err)
	}
	result, err := parent.vm.RunString(`!(crossRealmException instanceof DOMException) && crossRealmException instanceof ChildDOMException && crossRealmException.name === 'InvalidStateError' && crossRealmException.message === 'guarded' && crossRealmException.code === 11`)
	if err != nil {
		t.Fatalf("inspect imported exception: %v", err)
	}
	if !result.ToBoolean() {
		t.Fatalf("imported DOMException lost its source-realm brand or fields: %s", result.String())
	}
}

func TestImportParentValueResolvesChildNodeProxyBeforeCollidingNodeID(t *testing.T) {
	parentDocument := dom.NewDocument()
	parent, err := New(Options{DOM: parentDocument})
	if err != nil {
		t.Fatalf("create parent runtime: %v", err)
	}
	defer parent.Close()

	childDocument := dom.NewDocument()
	child, err := New(Options{DOM: childDocument})
	if err != nil {
		t.Fatalf("create child runtime: %v", err)
	}
	defer child.Close()

	childNode := childDocument.NewElement("custom-child")
	source, ok := child.wrapNode(childNode).(*goja.Object)
	if !ok {
		t.Fatal("child node wrapper is not an object")
	}
	childID := int(source.Get("__nid__").ToInteger())
	unrelatedParentNode := parentDocument.NewElement("unrelated")
	parent.nodeRegistry[childID] = unrelatedParentNode

	proxy := parent.importChildValue(child, source, make(map[*goja.Object]*goja.Object), 0)
	roundTrip := child.importParentValue(parent, proxy)
	roundTripObject, ok := roundTrip.(*goja.Object)
	if !ok || !roundTripObject.StrictEquals(source) {
		t.Fatalf("round-trip child node proxy = %v, want original child wrapper", roundTrip)
	}
}

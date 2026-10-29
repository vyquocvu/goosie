package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestIframeSrcdocParserMutationNodesKeepIdentity(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/parent"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body><script>
const frame = document.createElement('iframe');
frame.onload = () => {
  const body = frame.contentDocument.body;
  window.innerTargetIdentity = frame.contentWindow.innerTargetIdentity;
  window.innerAddedIdentity = frame.contentWindow.innerAddedIdentity;
  window.parserBodyIdentity = frame.contentWindow.parserBody === body;
  window.mutationTargetIdentity = frame.contentWindow.constructorRecord.target === body;
  window.mutationAddedNodeIdentity = frame.contentWindow.constructorRecord.addedNodes[0] === body.firstChild.nextSibling;
  const childRecords = frame.contentWindow.parserRecords;
  const bElement = body.firstChild.nextSibling;
  window.secondMutationTargetIdentity = childRecords[1].target === bElement;
  window.secondMutationAddedNodeIdentity = childRecords[1].addedNodes[0] === bElement.firstChild;
  const childCustomElement = frame.contentWindow.parserCustomElement;
  const documentCustomElement = frame.contentDocument.querySelector('some-element');
  window.customElementWrapperIdentity = childCustomElement === documentCustomElement;
  window.customElementAttributesCollectionIdentity = childCustomElement.attributes === documentCustomElement.attributes;
  window.customElementAttributesLength = childCustomElement.attributes.length;
  window.customElementAttributeName = childCustomElement.attributes[0].name;
};
frame.srcdoc = '<!doctype html><body><script>window.parserBody=document.body;const records=[];window.parserRecords=records;class SomeElement extends HTMLElement{constructor(){super();window.parserCustomElement=this;window.constructorRecord=records[0];window.innerTargetIdentity=records[0].target===document.body;window.innerAddedIdentity=records[0].addedNodes[0]===document.body.firstChild.nextSibling;}}customElements.define("some-element",SomeElement);const observer=new MutationObserver(r=>records.push(...r));observer.observe(document.body,{childList:true,subtree:true});<\/script><b><some-element data-x="value"></b></body>';
document.body.appendChild(frame);
</script></body></html>`
	session, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/parent", nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for name, value := range map[string]bool{
		"parserBodyIdentity":                        true,
		"innerTargetIdentity":                       true,
		"innerAddedIdentity":                        true,
		"mutationTargetIdentity":                    true,
		"mutationAddedNodeIdentity":                 true,
		"secondMutationTargetIdentity":              true,
		"secondMutationAddedNodeIdentity":           true,
		"customElementWrapperIdentity":              true,
		"customElementAttributesCollectionIdentity": true,
	} {
		got, ok := rt.Global(name)
		if !ok || got.Bool != value {
			t.Errorf("%s = %+v (present=%v), want %v", name, got, ok, value)
		}
	}
	if got, ok := rt.Global("customElementAttributesLength"); !ok || got.Num != 1 {
		t.Errorf("customElementAttributesLength = %+v (present=%v), want 1", got, ok)
	}
	if got, ok := rt.Global("customElementAttributeName"); !ok || got.Str != "data-x" {
		t.Errorf("customElementAttributeName = %+v (present=%v), want data-x", got, ok)
	}
}

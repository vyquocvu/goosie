package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestShadowRootDoesNotInheritDocumentFactoriesOrReadLegacyRegistryOption(t *testing.T) {
	r, err := js.New(js.Options{DOM: domtest.Parse(`<!doctype html><html><body></body></html>`)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const host = document.createElement('div');
		const root = host.attachShadow({mode: 'closed'});
		if (root.ownerDocument !== document) throw new Error('ShadowRoot ownerDocument did not return the host document');
		if (root.createElement !== undefined || root.createElementNS !== undefined || root.importNode !== undefined) {
			throw new Error('ShadowRoot exposed Document-only factory methods');
		}
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const frameDocument = frame.contentDocument;
		const frameRoot = frameDocument.createElement('div').attachShadow({mode: 'open'});
		if (frameRoot.ownerDocument !== frameDocument) throw new Error('ShadowRoot ownerDocument did not preserve the iframe Document identity');
		if (document.createElement('span').nodeType !== 1) throw new Error('Document.createElement unavailable');
		let legacyGetterRead = false;
		const legacyOptions = {
			mode: 'closed',
			get registry() { legacyGetterRead = true; throw new Error('legacy registry getter was read'); }
		};
		document.createElement('section').attachShadow(legacyOptions);
		if (legacyGetterRead) throw new Error('attachShadow read the obsolete registry option');
	`, "shadow-root-document-factories.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

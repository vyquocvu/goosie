package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestCustomElementsWhenDefinedPromiseSemantics(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var pending = customElements.whenDefined('x-ready');
		var samePending = customElements.whenDefined('x-ready');
		if (!(pending instanceof Promise)) throw new Error('whenDefined must return a Promise');
		if (pending !== samePending) throw new Error('pending whenDefined promise must be cached');
		var resolvedConstructor = null;
		var definition = class {};
		pending.then(function(value) { resolvedConstructor = value; });
		if (resolvedConstructor !== null) throw new Error('pending promise resolved synchronously');
		customElements.define('x-ready', definition);
		if (resolvedConstructor !== null) throw new Error('definition resolution ran synchronously');
		var alreadyDefined = customElements.whenDefined('x-ready');
		if (!(alreadyDefined instanceof Promise)) throw new Error('defined name must still return a Promise');
		var resolvedAlreadyDefined = null;
		alreadyDefined.then(function(value) { resolvedAlreadyDefined = value; });

		var invalid = customElements.whenDefined('badname');
		var invalidReason = null;
		var invalidIsDOMException = false;
		invalid.catch(function(error) {
			invalidReason = error.name;
			invalidIsDOMException = error instanceof DOMException;
		});
		if (invalidReason !== null) throw new Error('invalid-name rejection ran synchronously');
		Promise.resolve().then(function() {
			if (resolvedConstructor !== definition) throw new Error('pending promise resolved with the wrong constructor');
			if (resolvedAlreadyDefined !== definition) throw new Error('defined promise resolved with the wrong constructor');
			if (invalidReason !== 'SyntaxError') throw new Error('invalid name rejection = ' + invalidReason);
			if (!invalidIsDOMException) throw new Error('invalid name rejection must use DOMException');
		});
	`, "custom-elements-when-defined.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIframeInnerHTMLUsesIframeHTMLInterfacePrototype(t *testing.T) {
	r, err := js.New(js.Options{DOM: domtest.Parse(`<!doctype html><html><head></head><body></body></html>`), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		class ParentElement extends HTMLElement {}
		customElements.define('x-parent-element', ParentElement);
		var parserFrames = [];
		for (var index = 0; index < 2; index++) {
			var frame = document.createElement('iframe');
			document.body.appendChild(frame);
			if (index === 1 && typeof frame.contentWindow.Function !== 'function') throw new Error('iframe Function missing');
			parserFrames.push(frame);
		}
		window.testFrames = parserFrames;
	`, "iframe-parser-interface-setup.js"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	r.FlushIFrameLoads()
	if err := r.Run(`
		for (var index = 0; index < testFrames.length; index++) {
			var childFrame = testFrames[index];
			var childDocument = childFrame.contentDocument;
			childDocument.body.innerHTML = '<x-parent-element></x-parent-element>';
			var instance = childDocument.querySelector('x-parent-element');
			if (!(instance instanceof childFrame.contentWindow.HTMLElement)) throw new Error('iframe element is not an instance of iframe HTMLElement, frame=' + index);
			if (instance instanceof ParentElement) throw new Error('iframe parser used the parent custom element registry, frame=' + index);
		}
	`, "iframe-parser-interface-owner-registry.js"); err != nil {
		t.Fatalf("iframe parse: %v", err)
	}
}

func TestCustomElementAdoptedAwayAndBackDuringConstructionKeepsPrototype(t *testing.T) {
	r, err := js.New(js.Options{DOM: domtest.Parse(`<!doctype html><html><head></head><body></body></html>`), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var detachedDocument = document.implementation.createHTMLDocument('detached');
		class AdoptBackElement extends HTMLElement {
			constructor() {
				super();
				detachedDocument.adoptNode(this);
				document.adoptNode(this);
			}
		}
		customElements.define('x-adopt-back', AdoptBackElement);
		var adoptedBack = document.createElement('x-adopt-back');
		if (!(adoptedBack instanceof AdoptBackElement)) throw new Error('adopted-back construction lost custom prototype');
		if (adoptedBack.ownerDocument !== document) throw new Error('adopted-back construction did not restore ownerDocument');
	`, "custom-element-adopt-away-and-back.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestScopedCustomElementRegistryCreateElementCandidateUpgradeOnAppend(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		function test(callback) { callback({}); }
		test(() => {
			class CDElement extends HTMLElement {}
			const registry = new CustomElementRegistry();
			const cdElement = document.createElement('c-d', {customElementRegistry: registry});
			if (cdElement instanceof CDElement) throw new Error('candidate upgraded before definition');
			registry.define('c-d', CDElement);
			if (cdElement instanceof CDElement) throw new Error('disconnected candidate upgraded during define');
			document.body.appendChild(cdElement);
			if (!(cdElement instanceof CDElement)) throw new Error('connected candidate failed to upgrade');
			const shadowHost = document.createElement('div');
			const shadow = shadowHost.attachShadow({mode: 'open', customElementRegistry: registry});
			const globalElement = document.createElement('x-global-moved');
			shadow.appendChild(globalElement);
			if (globalElement.customElementRegistry !== customElements) throw new Error('moving a global-registry element into a scoped shadow root changed its registry');
			shadow.removeChild(globalElement);
			if (globalElement.customElementRegistry !== customElements) throw new Error('removing a global-registry element from a scoped shadow root changed its registry');
			const scopedElement = document.createElement('x-scoped-moved', {customElementRegistry: registry});
			shadow.appendChild(scopedElement);
			document.body.appendChild(scopedElement);
			if (scopedElement.customElementRegistry !== registry) throw new Error('moving a scoped-registry element out of its shadow root changed its registry');
		});
	`, "scoped-create-element-candidate.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementRegistryNullAttributeIsParserScoped(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body><div id="null-root" customelementregistry><div id="null-child"><x-null-candidate id="candidate"></x-null-candidate></div></div><div id="global-root"></div></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const nullRoot = document.getElementById('null-root');
		const nullChild = document.getElementById('null-child');
		const candidate = document.getElementById('candidate');
		if (nullRoot.customElementRegistry !== null || nullChild.customElementRegistry !== null || candidate.customElementRegistry !== null) throw new Error('parsed customelementregistry attribute did not select null for descendants');
		if (document.getElementById('global-root').customElementRegistry !== customElements) throw new Error('unmarked parsed element did not use the global registry');
		const cloned = nullRoot.cloneNode(true);
		if (cloned.customElementRegistry !== null || cloned.firstElementChild.customElementRegistry !== null) throw new Error('clone did not retain null registry association');
		class NullCandidate extends HTMLElement {}
		customElements.define('x-null-candidate', NullCandidate);
		if (candidate instanceof NullCandidate) throw new Error('null-registry parser candidate upgraded through the global registry');
		const dynamic = document.createElement('div');
		dynamic.setAttribute('customelementregistry', '');
		if (dynamic.customElementRegistry !== customElements) throw new Error('dynamic attribute mutation changed the registry');
		const parsedBySetter = document.createElement('section');
		parsedBySetter.setHTMLUnsafe('<x-null-candidate customelementregistry></x-null-candidate>');
		if (parsedBySetter.firstElementChild.customElementRegistry !== null || parsedBySetter.firstElementChild instanceof NullCandidate) throw new Error('setHTMLUnsafe ignored parser null-registry association');
	`, "null-registry-attribute.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementRegistryXHTMLParserAndSetHTMLUnsafe(t *testing.T) {
	doc, err := js.ParseXMLDocumentBounded(`<html xmlns="http://www.w3.org/1999/xhtml"><body><div id="container"><a-b customelementregistry=""></a-b></div></body></html>`, "application/xhtml+xml", dom.ParseLimits{Nodes: 256, Depth: 32, Attributes: 64, AttributeBytes: 4096})
	if err != nil {
		t.Fatalf("ParseXMLDocumentBounded: %v", err)
	}
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	r.SetDOM(doc)
	if err := r.Run(`
		const container = document.getElementById('container');
		if (container.firstElementChild.customElementRegistry !== null) throw new Error('XHTML parser did not assign null registry');
		container.setHTMLUnsafe('<a-b customelementregistry></a-b>');
		if (container.firstElementChild.customElementRegistry !== null) throw new Error('XHTML setHTMLUnsafe did not use HTML fragment parsing');
		container.setHTMLUnsafe('<a-b></a-b>');
		if (container.firstElementChild.customElementRegistry !== customElements) throw new Error('XHTML setHTMLUnsafe did not use document registry by default');
		let strictXMLInnerHTML = false;
		try { container.innerHTML = '<a-b customelementregistry></a-b>'; }
		catch (error) { strictXMLInnerHTML = error.name === 'SyntaxError'; }
		if (!strictXMLInnerHTML) throw new Error('XML innerHTML stopped requiring well-formed markup');
	`, "xhtml-custom-element-registry.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementRegistryGetName(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var definition = class extends HTMLElement {};
		var unregistered = class extends HTMLElement {};
		customElements.define('x-name-lookup', definition);
		var lookupName = customElements.getName(definition);
		var missingName = customElements.getName(unregistered);
		var getNameIsOnPrototype = typeof CustomElementRegistry.prototype.getName === 'function';
		var invalidValuesThrow = [undefined, null, 'x-name-lookup', 1, {}, []].every(function(value) {
			try { customElements.getName(value); return false; }
			catch (error) { return error instanceof TypeError; }
		});
	`, "custom-elements-get-name.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for name, want := range map[string]string{
		"lookupName":           "x-name-lookup",
		"missingName":          "null",
		"getNameIsOnPrototype": "true",
		"invalidValuesThrow":   "true",
	} {
		got, err := r.Evaluate(name)
		if err != nil {
			t.Fatalf("Evaluate %s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestCustomElementDefineRejectsCustomAndUnknownExtendsNames(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const invalidExtends = [
			'', 'a-', 'a-a', 'aa-', 'aa-a', 'a-.-_', 'bgsound', 'blink', 'isindex',
			'multicol', 'nextid', 'spacer', 'elementnametobeunknownelement'
		];
		for (let i = 0; i < invalidExtends.length; i++) {
			const extendsName = invalidExtends[i];
			try {
				customElements.define('x-invalid-extends-' + i, class {}, {extends: extendsName});
				throw new Error('accepted unsupported extends name: ' + extendsName);
			} catch (error) {
				if (!(error instanceof DOMException) || error.name !== 'NotSupportedError') throw error;
			}
		}
		for (const localName of ['address', 'ins', 'del', 'param', 'div', 'button']) {
			customElements.define('x-supported-extends-' + localName, class {}, {extends: localName});
		}
	`, "custom-element-define-extends-validation.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestScopedCustomElementRegistryConstructsAndKeepsDefinitionsIsolated(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const scoped = new CustomElementRegistry();
		class GlobalABElement extends HTMLElement {}
		class ScopedABElement extends HTMLElement {}
		customElements.define('a-b', GlobalABElement);
		scoped.define('a-b', ScopedABElement);
		if (!(document.createElementNS('http://www.w3.org/1999/xhtml', 'a-b') instanceof GlobalABElement)) throw new Error('createElementNS default registry did not upgrade custom element');
		if (!(document.createElementNS('http://www.w3.org/1999/xhtml', 'a-b', {customElementRegistry: scoped}) instanceof ScopedABElement)) throw new Error('createElementNS scoped registry did not upgrade custom element');
		if (!(document.createElementNS('http://www.w3.org/1999/xhtml', 'a-b', {customElementRegistry: window.customElements}) instanceof GlobalABElement)) throw new Error('createElementNS explicit global registry did not upgrade custom element');
		const otherScoped = new CustomElementRegistry();
		const noDefinitionNS = document.createElementNS('http://www.w3.org/1999/xhtml', 'a-b', {customElementRegistry: otherScoped});
		if (Object.getPrototypeOf(noDefinitionNS).constructor.name !== 'HTMLElement') throw new Error('createElementNS used a different registry definition');
		if (scoped === customElements) throw new Error('scoped registry aliases the global registry');
		if (document.customElementRegistry !== customElements) throw new Error('document does not expose its default registry');
		if (document.implementation.createHTMLDocument().customElementRegistry !== null) throw new Error('detached document exposes a registry');
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		if (frame.contentDocument.customElementRegistry !== frame.contentWindow.customElements) throw new Error('iframe document registry is not its window registry');
		const SharedElement = class extends HTMLElement {};
		customElements.define('x-global-registry-test', SharedElement);
		scoped.define('x-scoped-registry-test', SharedElement);
		if (scoped.get('x-scoped-registry-test') !== SharedElement) throw new Error('scoped definition lookup failed');
		if (customElements.get('x-scoped-registry-test') !== undefined) throw new Error('scoped definition leaked into the global registry');
		if (scoped.get('x-global-registry-test') !== undefined) throw new Error('global definition leaked into the scoped registry');
		class ScopedCreatedElement extends HTMLElement {}
		scoped.define('x-scoped-create-element', ScopedCreatedElement);
		const scopedCreated = document.createElement('x-scoped-create-element', {customElementRegistry: scoped});
		if (!(scopedCreated instanceof ScopedCreatedElement)) throw new Error('document.createElement did not use the scoped registry');
		if (scopedCreated.customElementRegistry !== scoped) throw new Error('element did not retain its scoped registry');
		const scopedCreatedNS = document.createElementNS('http://www.w3.org/1999/xhtml', 'x-scoped-create-element', {customElementRegistry: scoped});
		if (!(scopedCreatedNS instanceof ScopedCreatedElement) || scopedCreatedNS.customElementRegistry !== scoped) throw new Error('document.createElementNS did not use the scoped registry');
		const scopedHTMLContainer = document.createElement('div', {customElementRegistry: scoped});
		scopedHTMLContainer.innerHTML = '<x-scoped-create-element></x-scoped-create-element>';
		const scopedHTMLChild = scopedHTMLContainer.querySelector('x-scoped-create-element');
		if (!(scopedHTMLChild instanceof ScopedCreatedElement) || scopedHTMLChild.customElementRegistry !== scoped) throw new Error('innerHTML did not use the container scoped registry: reg=' + scopedHTMLChild.customElementRegistry + ', ctor=' + Object.getPrototypeOf(scopedHTMLChild).constructor.name);
		const scopedUnsafeHTMLContainer = document.createElement('div', {customElementRegistry: scoped});
		scopedUnsafeHTMLContainer.setHTMLUnsafe('<x-scoped-create-element></x-scoped-create-element>');
		const scopedUnsafeHTMLChild = scopedUnsafeHTMLContainer.querySelector('x-scoped-create-element');
		if (!(scopedUnsafeHTMLChild instanceof ScopedCreatedElement) || scopedUnsafeHTMLChild.customElementRegistry !== scoped) throw new Error('setHTMLUnsafe did not use the container scoped registry');
		const scopedUnsafeShadowHost = document.createElement('div');
		const scopedUnsafeShadow = scopedUnsafeShadowHost.attachShadow({mode: 'open', customElementRegistry: scoped});
		scopedUnsafeShadow.setHTMLUnsafe('<x-scoped-create-element></x-scoped-create-element>');
		const scopedUnsafeShadowChild = scopedUnsafeShadow.querySelector('x-scoped-create-element');
		if (!(scopedUnsafeShadowChild instanceof ScopedCreatedElement) || scopedUnsafeShadowChild.customElementRegistry !== scoped) throw new Error('ShadowRoot.setHTMLUnsafe did not use the shadow registry');
		const nullHTMLContainer = document.createElement('div', {customElementRegistry: null});
		nullHTMLContainer.innerHTML = '<x-global-registry-test></x-global-registry-test>';
		const nullHTMLChild = nullHTMLContainer.querySelector('x-global-registry-test');
		if (nullHTMLChild.customElementRegistry !== null || nullHTMLChild instanceof SharedElement) throw new Error('innerHTML escaped a null registry boundary');
		class ScopedInnerSome extends HTMLElement {}
		class ScopedInnerOther extends HTMLElement {}
		class OtherRegistryElement extends HTMLElement {}
		scoped.define('x-inner-some', ScopedInnerSome);
		scoped.define('x-inner-other', ScopedInnerOther);
		otherScoped.define('x-inner-other', OtherRegistryElement);
		const movingHost = document.createElement('div');
		const movingShadow = movingHost.attachShadow({mode: 'open', customElementRegistry: otherScoped});
		const movingElement = document.createElement('x-inner-some', {customElementRegistry: scoped});
		movingElement.innerHTML = '<x-inner-other></x-inner-other>';
		if (!(movingElement.querySelector('x-inner-other') instanceof ScopedInnerOther)) throw new Error('innerHTML did not use the disconnected element registry');
		movingShadow.appendChild(movingElement);
		movingElement.innerHTML = '<x-inner-other></x-inner-other>';
		if (!(movingElement.querySelector('x-inner-other') instanceof ScopedInnerOther)) throw new Error('innerHTML registry changed after moving to another scoped shadow root');
		movingElement.remove();
		movingElement.innerHTML = '<x-inner-other></x-inner-other>';
		if (!(movingElement.querySelector('x-inner-other') instanceof ScopedInnerOther)) throw new Error('innerHTML registry changed after detaching the element');
		const scopedBuiltinNS = document.createElementNS('http://www.w3.org/1999/xhtml', 'div', {customElementRegistry: scoped});
		if (!(scopedBuiltinNS instanceof HTMLDivElement) || scopedBuiltinNS.customElementRegistry !== scoped) throw new Error('createElementNS did not retain the registry on a builtin');
		for (const [localName, constructor] of Object.entries({div: HTMLDivElement, form: HTMLFormElement, span: HTMLSpanElement, table: HTMLTableElement, unknown: HTMLUnknownElement})) {
			const builtin = document.createElementNS('http://www.w3.org/1999/xhtml', localName, {customElementRegistry: scoped});
			if (!(builtin instanceof constructor) || builtin.customElementRegistry !== scoped) throw new Error('createElementNS builtin mismatch: ' + localName);
		}
		const nullRegistryNS = document.createElementNS('http://www.w3.org/1999/xhtml', 'x-null-create-ns', {customElementRegistry: null});
		if (nullRegistryNS.customElementRegistry !== null) throw new Error('createElementNS did not retain a null registry');
		class ScopedCandidateNS extends HTMLElement {}
		const scopedCandidateNS = document.createElementNS('http://www.w3.org/1999/xhtml', 'x-scoped-candidate-ns', {customElementRegistry: scoped});
		scoped.define('x-scoped-candidate-ns', ScopedCandidateNS);
		if (scopedCandidateNS instanceof ScopedCandidateNS) throw new Error('disconnected createElementNS candidate upgraded early');
		document.body.appendChild(scopedCandidateNS);
		if (!(scopedCandidateNS instanceof ScopedCandidateNS)) throw new Error('connected createElementNS candidate did not upgrade');
		class ScopedCandidateElement extends HTMLElement {}
		const scopedCandidate = document.createElement('c-d', {customElementRegistry: scoped});
		scoped.define('c-d', ScopedCandidateElement);
		if (scopedCandidate instanceof ScopedCandidateElement) throw new Error('disconnected createElement candidate upgraded early');
		document.body.appendChild(scopedCandidate);
		if (!(scopedCandidate instanceof ScopedCandidateElement)) throw new Error('connected createElement candidate did not upgrade');
		const foreignRegistryNS = document.createElementNS(null, 'x-foreign-create-ns', {customElementRegistry: scoped});
		if (foreignRegistryNS.namespaceURI !== null || foreignRegistryNS.customElementRegistry !== scoped) throw new Error('createElementNS did not retain registry in a non-HTML namespace');
		const emptyDocumentNS = new Document().createElementNS(null, 'time', {customElementRegistry: scoped});
		if (emptyDocumentNS.namespaceURI !== null || emptyDocumentNS.customElementRegistry !== scoped) throw new Error('non-HTML Document.createElementNS lost its scoped registry');
		const emptyDocumentElement = new Document().createElement('time', {customElementRegistry: scoped});
		if (emptyDocumentElement.namespaceURI !== null || emptyDocumentElement.customElementRegistry !== scoped) throw new Error('non-HTML Document.createElement lost its scoped registry');
		const freshDocument = new Document();
		const freshContainer = freshDocument.createElementNS('http://www.w3.org/1999/xhtml', 'div');
		freshContainer.innerHTML = '<a-b id="ab1"></a-b><a-b id="ab2"></a-b>';
		const freshCandidates = freshContainer.querySelectorAll('a-b');
		if (freshCandidates.length !== 2 || freshCandidates[0].id !== 'ab1' || freshCandidates[1].id !== 'ab2') throw new Error('detached Document fragment parsing or selector mismatch: len=' + freshCandidates.length + ', first=' + (freshCandidates[0] && freshCandidates[0].id) + ', second=' + (freshCandidates[1] && freshCandidates[1].id) + ', html=' + freshContainer.innerHTML + ', child=' + (freshContainer.firstChild && freshContainer.firstChild.nodeName) + ', local=' + (freshContainer.firstChild && freshContainer.firstChild.localName) + ', ns=' + (freshContainer.firstChild && freshContainer.firstChild.namespaceURI) + ', matches=' + (freshContainer.firstChild && freshContainer.firstChild.matches('a-b')));
		const detached = document.implementation.createHTMLDocument();
		class ScopedDocumentElement extends HTMLElement {}
		scoped.define('x-initialized-document', ScopedDocumentElement);
		class ImportedScopedElement extends HTMLElement {}
		scoped.define('x-imported-scoped', ImportedScopedElement);
		const scopedImportedSource = detached.createElement('x-imported-scoped');
		const importedScoped = document.importNode(scopedImportedSource, {customElementRegistry: scoped});
		if (!(importedScoped instanceof ImportedScopedElement) || importedScoped.customElementRegistry !== scoped) throw new Error('importNode did not apply its scoped registry fallback');
		const importedExistingScoped = document.importNode(scopedCreated, {customElementRegistry: scoped});
		if (!(importedExistingScoped instanceof ScopedCreatedElement) || importedExistingScoped.customElementRegistry !== scoped) throw new Error('importNode did not preserve the source registry');
		const importRoot = document.createElement('div');
		importRoot.appendChild(document.createElement('span'));
		if (document.importNode(importRoot).hasChildNodes()) throw new Error('importNode default did not stay shallow');
		if (!document.importNode(importRoot, {}).hasChildNodes()) throw new Error('importNode options default did not deep clone');
		let nullImportRegistryError = '';
		try { document.importNode(importRoot, {customElementRegistry: null}); } catch (error) { nullImportRegistryError = error.name; }
		if (nullImportRegistryError !== 'TypeError') throw new Error('importNode null registry option error = ' + nullImportRegistryError);
		const detachedElement = detached.createElement('x-initialized-document');
		detached.body.appendChild(detachedElement);
		scoped.initialize(detached);
		if (detached.customElementRegistry !== scoped || detachedElement.customElementRegistry !== scoped) throw new Error('initialize did not associate a document and its descendants');
		if (!(detachedElement instanceof ScopedDocumentElement)) throw new Error('initialize did not upgrade an already-defined document candidate');
		const fragment = detached.createDocumentFragment();
		const fragmentElement = detached.createElement('x-initialized-fragment');
		fragment.appendChild(fragmentElement);
		if (fragmentElement.customElementRegistry !== null) throw new Error('uninitialized fragment descendant is not null');
		scoped.initialize(fragment);
		if (fragmentElement.customElementRegistry !== scoped) throw new Error('initialize did not associate fragment descendants');
		const host = detached.createElement('div');
		const shadow = host.attachShadow({mode: 'open', customElementRegistry: null});
		const shadowElement = detached.createElement('x-initialized-shadow');
		shadow.appendChild(shadowElement);
		if (shadow.customElementRegistry !== null || shadowElement.customElementRegistry !== null) throw new Error('explicitly null shadow registry was not retained');
		scoped.initialize(shadow);
		if (shadow.customElementRegistry !== scoped || shadowElement.customElementRegistry !== scoped) throw new Error('initialize did not associate shadow descendants');
		if (!(new CustomElementRegistry() instanceof CustomElementRegistry)) throw new Error('CustomElementRegistry constructor failed after initialize');
		let errorName = '';
		try { scoped.define('x-scoped-button', class extends HTMLButtonElement {}, {extends: 'button'}); }
		catch (error) { errorName = error.name; }
		if (errorName !== 'NotSupportedError') throw new Error('scoped customized built-in error = ' + errorName);
	`, "scoped-custom-element-registry.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementRegistryInitializeAndUpgradeRespectDeclarativeShadowRegistries(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><some-host id="host"><template shadowrootmode="open" shadowrootclonable="true" shadowrootcustomelementregistry><a-b><template shadowrootmode="open" shadowrootclonable="true" shadowrootcustomelementregistry><c-d><template shadowrootmode="open" shadowrootclonable="true"><a-b></a-b></template></c-d></template></a-b></template></some-host></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	err = r.Run(`
		customElements.define('some-host', class SomeHost extends HTMLElement {
			constructor() { super(); this.elementInternals = this.attachInternals(); }
		});
		class GlobalABElement extends HTMLElement { constructor() { super(); this.elementInternals = this.attachInternals(); } }
		class GlobalCDElement extends HTMLElement {}
		customElements.define('a-b', GlobalABElement);
		customElements.define('c-d', GlobalCDElement);
		const original = document.getElementById('host');
		const clone = original.cloneNode(true);
		const outer = clone.shadowRoot;
		const ab = outer.querySelector('a-b');
		if (outer.customElementRegistry !== null || ab.customElementRegistry !== null) throw new Error('declarative null registry was not retained');
		const scoped = new CustomElementRegistry();
		scoped.define('a-b', class ScopedABElement extends HTMLElement {
			constructor() { super(); this.elementInternals = this.attachInternals(); }
		});
		scoped.initialize(outer);
		if (outer.customElementRegistry !== scoped || ab.customElementRegistry !== scoped) throw new Error('initialize did not associate cloned shadow tree');
		scoped.upgrade(outer);
		if (Object.getPrototypeOf(ab).constructor.name !== 'ScopedABElement') throw new Error('outer candidate did not use its initialized registry');
		const nested = ab.shadowRoot;
		const cd = nested.querySelector('c-d');
		if (nested.customElementRegistry !== null || cd.customElementRegistry !== null) throw new Error('nested declarative registry was overwritten');
		if (Object.getPrototypeOf(cd).constructor.name !== 'HTMLElement') throw new Error('nested null-registry candidate was upgraded: ' + Object.getPrototypeOf(cd).constructor.name);
	`, "custom-element-initialize-declarative-shadow-registry.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementNamesFollowValidElementLocalNameCharacters(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const validNames = ['x-control-' + String.fromCharCode(1), 'x-plane-' + String.fromCodePoint(0xF0000)];
		for (let i = 0; i < validNames.length; i++) customElements.define(validNames[i], class {});
		for (const invalidName of ['x-tab-\t', 'x-space- ', 'x-slash-/', 'x-greater->', 'x-upper-A']) {
			try {
				customElements.define(invalidName, class {});
				throw new Error('accepted invalid custom element name: ' + invalidName);
			} catch (error) {
				if (!(error instanceof DOMException) || error.name !== 'SyntaxError') throw error;
			}
		}
	`, "custom-element-valid-local-name-characters.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementDefineRejectsNullCallbackAndIframeExposesTypeError(t *testing.T) {
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '<!doctype html><html><body></body></html>';
		document.body.appendChild(frame);
		if (typeof frame.contentDocument.defaultView.TypeError !== 'function') throw new Error('iframe defaultView.TypeError is missing');
		class NullCallbackElement {}
		NullCallbackElement.prototype.connectedCallback = null;
		let errorName = '';
		try { customElements.define('x-null-connected-callback', NullCallbackElement); }
		catch (error) { errorName = error.name; }
		if (errorName !== 'TypeError') throw new Error('null connectedCallback error = ' + errorName);
	`, "custom-element-define-null-callback.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHTMLElementConstructorValidatesNewTargetAndElementInterface(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	err = r.Run(`
		function throwsTypeError(thunk) {
			try { thunk(); return false; }
			catch (error) { return error instanceof TypeError; }
		}
		if (!throwsTypeError(() => new HTMLElement())) throw new Error('HTMLElement used itself as NewTarget');
		customElements.define('x-html-constructor-self', HTMLElement);
		if (!throwsTypeError(() => new HTMLElement())) throw new Error('HTMLElement remained constructible as its own registered NewTarget');
		class UnregisteredElement extends HTMLElement {}
		if (!throwsTypeError(() => new UnregisteredElement())) throw new Error('unregistered custom element was constructed');
		if (!throwsTypeError(() => Reflect.construct(HTMLElement, [], UnregisteredElement))) throw new Error('unregistered NewTarget was accepted');

		class WrongAutonomousInterface extends HTMLParagraphElement {}
		customElements.define('x-wrong-autonomous-interface', WrongAutonomousInterface);
		if (!throwsTypeError(() => new WrongAutonomousInterface())) throw new Error('native HTML interface was accepted for an autonomous element');

		class WrongCustomizedInterface extends HTMLElement {}
		customElements.define('x-wrong-customized-interface', WrongCustomizedInterface, {extends: 'button'});
		if (!throwsTypeError(() => new WrongCustomizedInterface())) throw new Error('HTMLElement was accepted for a customized built-in');

		class ValidAutonomousElement extends HTMLElement {}
		customElements.define('x-valid-newtarget', ValidAutonomousElement);
		const instance = new ValidAutonomousElement();
		if (!(instance instanceof HTMLElement) || instance.localName !== 'x-valid-newtarget') throw new Error('valid custom constructor failed');
	`, "html-constructor-newtarget-validation.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestProxyWrappedCustomElementConstructorInstanceOf(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	err = r.Run(`
		class ProxyElementBase extends HTMLElement {}
		let prototypeGets = 0;
		const ProxyElement = new Proxy(ProxyElementBase, {
			get(target, property, receiver) {
				if (property === 'prototype') prototypeGets++;
				return Reflect.get(target, property, receiver);
			}
		});
		customElements.define('x-proxy-constructor', ProxyElement);
		prototypeGets = 0;
		const instance = new ProxyElement();
		if (prototypeGets !== 1) throw new Error('prototype read count = ' + prototypeGets);
		if (!(instance instanceof ProxyElement)) throw new Error('instanceof proxy constructor failed');
		if (!(instance instanceof ProxyElementBase)) throw new Error('instanceof proxy target failed');
		if (!(instance instanceof HTMLElement)) throw new Error('instanceof HTMLElement failed');
		if (instance.localName !== 'x-proxy-constructor') throw new Error('wrong local name');
	`, "proxy-wrapped-custom-element-instanceof.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCreateElementNSSetsAutonomousCustomElementPrefixAfterConstruction(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var prefixDuringConstructor;
		var tagNameDuringConstructor;
		class PrefixEl extends HTMLElement {
			constructor() {
				super();
				prefixDuringConstructor = this.prefix;
				tagNameDuringConstructor = this.tagName;
			}
		}
		customElements.define('prefix-timing-el', PrefixEl);
		var prefixed = document.createElementNS('http://www.w3.org/1999/xhtml', 'p:prefix-timing-el');

		var innerPrefix;
		var outerPrefixDuringConstructor;
		class ReentrantEl extends HTMLElement {
			static callCount = 0;
			constructor() {
				super();
				if (ReentrantEl.callCount++ === 0) innerPrefix = new ReentrantEl().prefix;
				outerPrefixDuringConstructor = this.prefix;
			}
		}
		customElements.define('reentrant-prefix-el', ReentrantEl);
		var outer = document.createElementNS('http://www.w3.org/1999/xhtml', 'r:reentrant-prefix-el');
	`, "custom-elements-create-element-ns-prefix.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for name, want := range map[string]string{
		"prefixDuringConstructor":      "null",
		"tagNameDuringConstructor":     "PREFIX-TIMING-EL",
		"prefixed.prefix":              "p",
		"prefixed.tagName":             "P:PREFIX-TIMING-EL",
		"innerPrefix":                  "null",
		"outerPrefixDuringConstructor": "null",
		"outer.prefix":                 "r",
	} {
		got, err := r.Evaluate(name)
		if err != nil {
			t.Fatalf("Evaluate %s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestCustomElementAttributeChangedCallbackUsesDefinedCallback(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var changes = [];
		class ObservedElement extends HTMLElement {
			static observedAttributes = ['title', 'style'];
			attributeChangedCallback(name, oldValue, newValue, namespace) {
				changes.push([name, oldValue, newValue, namespace]);
			}
		}
		customElements.define('x-observed-mutations', ObservedElement);
		var observed = document.createElement('x-observed-mutations');
		observed.setAttribute('title', 'first');
		observed.title = 'second';
		observed.setAttribute('title', 'second');
		observed.removeAttribute('title');
		observed.style.fontSize = '10px';
		ObservedElement.prototype.attributeChangedCallback = function() { throw new Error('definition callback must be captured'); };
		observed.setAttribute('title', 'final');
		var log = changes.map(function(entry) { return entry[0] + ':' + entry[1] + '>' + entry[2] + ':' + entry[3]; }).join('|');
	`, "custom-elements-attribute-changed.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("log")
	if err != nil {
		t.Fatalf("Evaluate log: %v", err)
	}
	want := "title:null>first:null|title:first>second:null|title:second>null:null|style:null>font-size: 10px;:null|title:null>final:null"
	if got != want {
		t.Fatalf("attribute callback log = %q, want %q", got, want)
	}
}

func TestDOMTokenListOperationsQueueObservedAttributeReactions(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.tokenAttributeChanges = 0;
		class TokenListElement extends HTMLElement {
			static observedAttributes = ['class'];
			attributeChangedCallback() { window.tokenAttributeChanges++; }
		}
		customElements.define('x-token-list-reactions', TokenListElement);
		const element = document.createElement('x-token-list-reactions');
		element.setAttribute('class', 'a');
		window.tokenAttributeChanges = 0;
		element.classList.remove('missing');
		const afterNoopRemove = tokenAttributeChanges;
		element.classList = 'a';
		const afterSameValueSet = tokenAttributeChanges;
		element.classList.value = 'a';
		const afterTokenListValueSet = tokenAttributeChanges;
		const replaced = element.classList.replace('a', 'b');
		const afterReplace = tokenAttributeChanges;
		const missing = element.classList.replace('missing', 'c');
		window.tokenReactionSummary = [afterNoopRemove, afterSameValueSet, afterTokenListValueSet, replaced, afterReplace, missing, element.className].join(',');
	`, "dom-token-list-custom-element-reactions.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("window.tokenReactionSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "1,2,3,true,4,false,b" {
		t.Fatalf("DOMTokenList reaction summary = %q, want %q", got, "1,2,3,true,4,false,b")
	}
}

func TestDOMStringMapOperationsQueueObservedAttributeReactions(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.datasetAttributeChanges = 0;
		class DatasetElement extends HTMLElement {
			static observedAttributes = ['data-foo'];
			attributeChangedCallback() { window.datasetAttributeChanges++; }
		}
		customElements.define('x-dataset-reactions', DatasetElement);
		const element = document.createElement('x-dataset-reactions');
		element.dataset.foo = 'bar';
		const afterAdd = datasetAttributeChanges;
		element.dataset.foo = 'bar';
		const afterSameValueSet = datasetAttributeChanges;
		delete element.dataset.foo;
		window.datasetReactionSummary = [afterAdd, afterSameValueSet, datasetAttributeChanges, element.hasAttribute('data-foo')].join(',');
	`, "dom-string-map-custom-element-reactions.js")
	if err != nil {
		t.Fatalf("DOMStringMap script: %v", err)
	}
	got, err := r.Evaluate("window.datasetReactionSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "1,2,3,false" {
		t.Fatalf("DOMStringMap reaction summary = %q, want %q", got, "1,2,3,false")
	}
}

func TestSelectionDeleteFromDocumentQueuesDisconnectReactions(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.selectionDeleteDisconnects = 0;
		class SelectedElement extends HTMLElement {
			disconnectedCallback() { window.selectionDeleteDisconnects++; }
		}
		customElements.define('x-selected-delete', SelectedElement);
		const element = document.createElement('x-selected-delete');
		document.body.appendChild(element);
		const range = document.createRange();
		range.selectNode(element);
		const selection = window.getSelection();
		selection.addRange(range);
		selection.deleteFromDocument();
		window.selectionDeleteSummary = [selectionDeleteDisconnects,
			element.isConnected, selection.rangeCount, selection.isCollapsed].join(',');
	`, "selection-delete-custom-element-reaction.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("window.selectionDeleteSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "1,false,1,true" {
		t.Fatalf("Selection.deleteFromDocument summary = %q, want %q", got, "1,false,1,true")
	}
}

func TestContentEditableSetterQueuesObservedAttributeReaction(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.contentEditableChanges = 0;
		class EditableElement extends HTMLElement {
			static observedAttributes = ['contenteditable'];
			attributeChangedCallback() { window.contentEditableChanges++; }
		}
		customElements.define('x-editable-reactions', EditableElement);
		const element = document.createElement('x-editable-reactions');
		element.contentEditable = 'true';
		const afterAdd = contentEditableChanges;
		element.contentEditable = 'true';
		window.contentEditableReactionSummary = [afterAdd, contentEditableChanges, element.getAttribute('contenteditable')].join(',');
	`, "contenteditable-custom-element-reactions.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("window.contentEditableReactionSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "1,2,true" {
		t.Fatalf("contentEditable reaction summary = %q, want %q", got, "1,2,true")
	}
}

func TestReflectedSetterQueuesReactionForSameValue(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.titleAttributeChanges = 0;
		class TitleElement extends HTMLElement {
			static observedAttributes = ['title'];
			attributeChangedCallback() { window.titleAttributeChanges++; }
		}
		customElements.define('x-title-reactions', TitleElement);
		const element = document.createElement('x-title-reactions');
		element.title = '';
		const afterFirstSet = titleAttributeChanges;
		element.title = '';
		window.titleReactionSummary = [afterFirstSet, titleAttributeChanges, element.hasAttribute('title')].join(',');
	`, "reflected-setter-same-value-reaction.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("window.titleReactionSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "1,2,true" {
		t.Fatalf("reflected setter reaction summary = %q, want %q", got, "1,2,true")
	}
}

func TestNamedNodeMapAttributePropertyQueuesAttrValueReaction(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		window.attrValueChanges = [];
		class AttrElement extends HTMLElement {
			static observedAttributes = ['title'];
			attributeChangedCallback(name, oldValue, newValue) {
				attrValueChanges.push(name + ':' + oldValue + '>' + newValue);
			}
		}
		customElements.define('x-attr-value-reactions', AttrElement);
		const element = document.createElement('x-attr-value-reactions');
		element.setAttribute('title', 'before');
		element.attributes.title.value = 'after';
		window.attrValueReactionSummary = [element.attributes.length, element.attributes.title.value,
			attrValueChanges.join('|')].join(',');
	`, "named-node-map-attr-value-reaction.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("window.attrValueReactionSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "1,after,title:null>before|title:before>after" {
		t.Fatalf("NamedNodeMap Attr reaction summary = %q, want %q", got, "1,after,title:null>before|title:before>after")
	}
}

func TestCustomElementDefineValidationAndConstruction(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		if (!(customElements instanceof CustomElementRegistry)) throw new Error('customElements must implement CustomElementRegistry');
		if (!('define' in CustomElementRegistry.prototype)) throw new Error('define must be on the interface prototype');
		var invalidName;
		try { customElements.define('x-Upper', class extends HTMLElement {}); }
		catch (error) { invalidName = error; }
		if (!(invalidName instanceof DOMException) || invalidName.name !== 'SyntaxError' || invalidName.code !== 12) {
			throw new Error('uppercase name must throw SyntaxError DOMException');
		}
		class ReadyElement extends HTMLElement {
			constructor() { super(); this.constructed = true; }
		}
		customElements.define('x-ready-element', ReadyElement);
		var direct = new ReadyElement();
		if (!(direct instanceof ReadyElement) || !(direct instanceof HTMLElement)) throw new Error('direct instance prototype chain');
		if (direct.localName !== 'x-ready-element' || !direct.constructed) throw new Error('direct instance DOM identity');
		var created = document.createElement('X-READY-ELEMENT');
		if (!(created instanceof ReadyElement) || created.localName !== 'x-ready-element') throw new Error('createElement must upgrade registered custom elements');
		if (!('moveBefore' in Element.prototype)) throw new Error('Element.prototype.moveBefore must be exposed');
		var moveCallbacks = 0;
		class MovableElement extends HTMLElement { connectedMoveCallback() { moveCallbacks++; } }
		customElements.define('x-movable-element', MovableElement);
		var container = document.createElement('div');
		var first = document.createElement('x-movable-element');
		var second = document.createElement('x-movable-element');
		document.body.appendChild(container);
		container.appendChild(first);
		container.appendChild(second);
		container.moveBefore(second, first);
		if (container.firstChild !== second || moveCallbacks !== 1) throw new Error('moveBefore order or custom callback: first=' + (container.firstChild === second) + ', callbacks=' + moveCallbacks);
	`, "custom-elements-define.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementConstructionFailureIsReportedAndFallsBack(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		let reported;
		window.onerror = function(_message, _url, _line, _column, error) { reported = error; return true; };
		class InvalidElement extends HTMLElement { constructor() { return {}; } }
		customElements.define('x-invalid-element', InvalidElement);
		const element = document.createElement('x-invalid-element');
		if (!reported || reported.name !== 'TypeError') throw new Error('constructor failure was not reported as TypeError');
		if (!(element instanceof HTMLUnknownElement)) throw new Error('failed construction did not retain unknown-element fallback');
		reported = null;
		class MutatingElement extends HTMLElement { constructor() { super(); this.setAttribute('data-x', '1'); } }
		customElements.define('x-mutating-element', MutatingElement);
		const mutated = document.createElement('x-mutating-element');
		if (!reported || reported.name !== 'NotSupportedError') throw new Error('constructor mutation was not reported as NotSupportedError');
		if (!(mutated instanceof HTMLUnknownElement) || !mutated.hasAttribute('data-x')) throw new Error('mutated construction did not retain the node with unknown-element fallback');
		reported = null;
		class RestoringElement extends HTMLElement { constructor() { super(); this.setAttribute('data-x', '1'); this.removeAttribute('data-x'); } }
		customElements.define('x-restoring-element', RestoringElement);
		const restored = document.createElement('x-restoring-element');
		if (reported || !(restored instanceof RestoringElement)) throw new Error('restored construction should succeed without reporting');
		reported = null;
		class RecursiveElement extends HTMLElement { constructor(skip) { super(); if (!skip) new RecursiveElement(true); } }
		const recursiveCandidate = document.createElement('x-recursive-element');
		document.body.appendChild(recursiveCandidate);
		customElements.define('x-recursive-element', RecursiveElement);
		if (!reported || reported.name !== 'TypeError') throw new Error('reentrant HTMLElement construction was not reported as TypeError: ' + String(reported) + ' / ' + (reported && reported.name));
	`, "custom-element-construction-failure.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestFailedUpgradeKeepsCustomPrototypeAndSkipsReactions(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		let reported;
		window.onerror = function(_message, _url, _line, _column, error) { reported = error; return true; };
		const candidate = document.createElement('x-failed-upgrade');
		document.body.appendChild(candidate);
		const reactions = [];
		class FailedUpgrade extends HTMLElement {
			constructor() { super(); reactions.push('constructed'); throw new Error('construction failed'); }
			connectedCallback() { reactions.push('connected'); }
			attributeChangedCallback() { reactions.push('attribute'); }
			static get observedAttributes() { return ['id']; }
		}
		customElements.define('x-failed-upgrade', FailedUpgrade);
		if (Object.getPrototypeOf(candidate) !== FailedUpgrade.prototype) throw new Error('failed upgrade lost the custom prototype');
		if (reactions.join('|') !== 'constructed') throw new Error('failed upgrade reactions = ' + reactions.join('|'));
		if (!reported || reported.name !== 'Error') throw new Error('failed upgrade exception was not reported');
	`, "custom-element-failed-upgrade.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAdoptedUnresolvedCustomElementUpgradesWhenConnected(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		let connected = 0;
		class AdoptedElement extends HTMLElement { connectedCallback() { connected++; } }
		customElements.define('x-adopted-element', AdoptedElement);
		const detached = document.implementation.createHTMLDocument('detached');
		const element = detached.createElement('x-adopted-element');
		if (element instanceof AdoptedElement) throw new Error('detached document should leave the element unresolved');
		document.adoptNode(element);
		document.body.appendChild(element);
		if (!(element instanceof AdoptedElement)) throw new Error('connected adopted element was not upgraded');
		if (connected !== 1) throw new Error('connectedCallback count = ' + connected);
	`, "custom-element-adoption-upgrade.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementLifecycleCallbacksAreRetainedAndAdoptionReportsDocuments(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const reactions = [];
		class RetainedElement extends HTMLElement {
			connectedCallback() { reactions.push('connected:' + (this === element)); }
			disconnectedCallback() { reactions.push('disconnected:' + (this === element)); }
			adoptedCallback(oldDocument, newDocument) {
				reactions.push('adopted:' + (this === element) + ':' + (oldDocument === document) + ':' + (newDocument === detached));
			}
		}
		customElements.define('x-retained-lifecycle', RetainedElement);
		const element = document.createElement('x-retained-lifecycle');
		RetainedElement.prototype.connectedCallback = undefined;
		RetainedElement.prototype.disconnectedCallback = undefined;
		RetainedElement.prototype.adoptedCallback = undefined;
		document.body.appendChild(element);
		document.body.removeChild(element);
		const detached = document.implementation.createHTMLDocument('detached');
		detached.adoptNode(element);
		if (reactions.join('|') !== 'connected:true|disconnected:true|adopted:true:true:true') {
			throw new Error('retained lifecycle callbacks or adoption documents: ' + reactions.join('|'));
		}
	`, "custom-element-retained-lifecycle.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementConnectionReactionsTraverseLightAndShadowSubtrees(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const reactions = [];
		class NestedElement extends HTMLElement {
			connectedCallback() { reactions.push('connected'); }
			disconnectedCallback() { reactions.push('disconnected'); }
		}
		customElements.define('x-nested-lifecycle', NestedElement);
		const lightHost = document.createElement('div');
		const lightChild = document.createElement('x-nested-lifecycle');
		lightHost.appendChild(lightChild);
		document.body.appendChild(lightHost);
		if (reactions.join(',') !== 'connected') throw new Error('light subtree connect: ' + reactions);
		const shadowHost = document.createElement('div');
		const shadow = shadowHost.attachShadow({mode: 'open'});
		const shadowChild = document.createElement('x-nested-lifecycle');
		shadow.appendChild(shadowChild);
		if (reactions.length !== 1) throw new Error('detached shadow connected early: ' + reactions);
		document.body.appendChild(shadowHost);
		if (reactions.join(',') !== 'connected,connected') throw new Error('shadow subtree connect: ' + reactions);
		shadow.removeChild(shadowChild);
		if (reactions.join(',') !== 'connected,connected,disconnected') throw new Error('shadow subtree disconnect: ' + reactions);
	`, "custom-element-shadow-subtree-reactions.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementAdoptionReactionsPrecedeConnection(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const reactions = [];
		const detached = document.implementation.createHTMLDocument('detached');
		class AdoptedElement extends HTMLElement {
			adoptedCallback(oldDocument, newDocument) {
				reactions.push('adopted:' + (oldDocument === document) + ':' + (newDocument === detached));
			}
			connectedCallback() { reactions.push('connected'); }
		}
		customElements.define('x-adopted-order', AdoptedElement);
		const element = document.createElement('x-adopted-order');
		detached.body.appendChild(element);
		if (reactions.join('|') !== 'adopted:true:true|connected') {
			throw new Error('adoption reaction order: ' + reactions.join('|'));
		}
	`, "custom-element-adoption-order.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestUnresolvedCustomElementUpgradesAfterCrossDocumentMoveWithoutAdoptedCallback(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const targetDocument = frame.contentDocument;
		const reactions = [];
		class ChildElement extends frame.contentWindow.HTMLElement {
			constructor() { super(); reactions.push('constructed'); }
			adoptedCallback() { reactions.push('adopted'); }
			connectedCallback() { reactions.push('connected'); }
		}
		frame.contentWindow.customElements.define('x-child-document-element', ChildElement);
		const element = document.createElement('x-child-document-element');
		if (element instanceof ChildElement) throw new Error('source document unexpectedly upgraded the element');
		if (reactions.length !== 0) throw new Error('constructor ran before insertion: ' + reactions.join('|'));
		targetDocument.body.appendChild(element);
		if (reactions.join('|') !== 'constructed|connected') {
			throw new Error('newly upgraded cross-document element reactions: ' + reactions.join('|'));
		}
	`, "custom-element-cross-document-first-upgrade.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIframeInitialWindowExposesHTMLInterfaceConstructors(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const frame = document.createElement('iframe');
		const childWindow = frame.contentWindow;
		if (typeof childWindow.HTMLDivElement !== 'function') throw new Error('initial iframe WindowProxy omitted HTMLDivElement');
		if (!('HTMLDivElement' in childWindow)) throw new Error('initial iframe WindowProxy has no HTMLDivElement property');
		const childDiv = frame.contentDocument.createElement('div');
		if (Object.getPrototypeOf(childDiv) !== childWindow.HTMLDivElement.prototype) throw new Error('initial iframe div does not use its exposed HTMLDivElement prototype');
		if (!(childDiv instanceof childWindow.HTMLDivElement)) throw new Error('initial iframe div does not inherit from its exposed HTMLDivElement interface');
		if (childDiv instanceof HTMLDivElement) throw new Error('initial iframe div incorrectly uses the parent HTMLDivElement interface');
	`, "iframe-initial-html-constructors.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestInitialIframeWindowUsesFrameSpecificHTMLConstructorPrototypes(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const childWindow = frame.contentWindow;
		if (childWindow.HTMLElement === HTMLElement) throw new Error('initial WindowProxy reused the parent HTMLElement constructor');
		if (childWindow.HTMLElement.prototype === HTMLElement.prototype) throw new Error('initial WindowProxy reused the parent HTMLElement prototype');
		if (Object.getPrototypeOf(childWindow.HTMLElement.prototype) !== HTMLElement.prototype) throw new Error('frame HTMLElement prototype lost its inherited interface methods');
		if (childWindow.Proxy === Proxy) throw new Error('initial WindowProxy reused the parent Proxy constructor');
		class FrameElement extends childWindow.HTMLElement {}
		childWindow.customElements.define('x-frame-specific-interface', FrameElement);
		const element = frame.contentDocument.createElement('x-frame-specific-interface');
		if (!(element instanceof FrameElement) || !(element instanceof childWindow.HTMLElement)) throw new Error('frame-specific interface prototype did not participate in custom-element construction');
	`, "iframe-frame-specific-interface-constructors.js"); err != nil {
		t.Fatalf("Run frame-specific interface constructor case: %v", err)
	}
}

func TestDeepCloneUpgradesDescendantWithIframeRegistry(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		var frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.appendChild(frame);
	`, "custom-element-deep-clone-frame.js"); err != nil {
		t.Fatalf("create iframe: %v", err)
	}
	err = r.Run(`
		const childWindow = frame.contentWindow;
		const childDocument = frame.contentDocument;
		class ChildElement extends childWindow.HTMLElement {}
		childWindow.customElements.define('x-deep-clone-child', ChildElement);
		const instance = childDocument.createElement('x-deep-clone-child');
		const container = childDocument.createElement('div');
		container.appendChild(instance);
		const clone = container.cloneNode(true).firstChild;
		if (!(clone instanceof childWindow.HTMLElement)) throw new Error('deep clone is not an iframe HTMLElement');
		if (!(clone instanceof ChildElement)) throw new Error('deep clone is not an instance of its custom element');
		if (clone.customElementRegistry !== childWindow.customElements) throw new Error('deep clone lost its iframe registry');
	`, "custom-element-deep-clone-iframe-registry.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomizedBuiltInCloneKeepsCreationDefinitionAndCandidate(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(`<!doctype html><html><head></head><body></body></html>`))
	if err := r.Run(`
		class FirstDiv extends HTMLDivElement {}
		class SecondDiv extends HTMLDivElement {}
		class LateDiv extends HTMLDivElement {}
		customElements.define('x-first-div', FirstDiv, {extends: 'div'});
		customElements.define('x-second-div', SecondDiv, {extends: 'div'});

		const upgraded = document.createElement('div', {is: 'x-first-div'});
		upgraded.setAttribute('is', 'x-second-div');
		const upgradedClone = upgraded.cloneNode(false);
		if (!(upgradedClone instanceof FirstDiv)) throw new Error('clone lost its established custom-element definition');

		const unresolved = document.createElement('div', {is: 'x-late-div'});
		unresolved.setAttribute('is', 'x-second-div');
		const unresolvedClone = unresolved.cloneNode(false);
		customElements.define('x-late-div', LateDiv, {extends: 'div'});
		document.body.appendChild(unresolved);
		document.body.appendChild(unresolvedClone);
		if (!(unresolved instanceof LateDiv) || !(unresolvedClone instanceof LateDiv)) {
			throw new Error('creation-time candidate was lost: original=' + (unresolved instanceof LateDiv) + ', clone=' + (unresolvedClone instanceof LateDiv));
		}
	`, "customized-built-in-clone-state.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomizedBuiltInImportKeepsCreationCandidate(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(`<!doctype html><html><head></head><body></body></html>`))
	if err := r.Run(`
		class LateDiv extends HTMLDivElement {}
		const source = document.createElement('div', {is: 'x-import-late-div'});
		source.setAttribute('is', 'x-import-other-div');
		const imported = document.importNode(source);
		customElements.define('x-import-late-div', LateDiv, {extends: 'div'});
		document.body.appendChild(imported);
		if (!(imported instanceof LateDiv)) throw new Error('importNode lost the source creation-time customized-built-in candidate');
	`, "customized-built-in-import-state.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestShadowRootAppendChildMovesChildrenAsFragment(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const reactions = [];
		const destination = document.implementation.createHTMLDocument('destination');
		class ShadowElement extends HTMLElement {
			connectedCallback() { reactions.push('connected'); }
			disconnectedCallback() { reactions.push('disconnected'); }
			adoptedCallback(oldDocument, newDocument) {
				reactions.push('adopted:' + (oldDocument === document) + ':' + (newDocument === destination));
			}
		}
		customElements.define('x-shadow-fragment', ShadowElement);
		const host = document.createElement('div');
		const shadow = host.attachShadow({mode: 'closed'});
		const child = document.createElement('x-shadow-fragment');
		shadow.appendChild(child);
		document.body.appendChild(host);
		reactions.length = 0;
		if (destination.documentElement.appendChild(shadow) !== shadow) throw new Error('appendChild must return shadow root');
		if (shadow.firstChild !== null) throw new Error('shadow root should be emptied');
		if (reactions.join('|') !== 'disconnected|adopted:true:true|connected') {
			throw new Error('shadow fragment reactions: ' + reactions.join('|'));
		}
	`, "custom-element-shadow-root-fragment-adoption.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementConnectionCallbackReparentDoesNotDuplicateDescendantReaction(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		let childConnections = 0;
		class ParentElement extends HTMLElement {
			connectedCallback() {
				const child = this.firstChild;
				this.removeChild(child);
				this.appendChild(child);
			}
		}
		class ChildElement extends HTMLElement {
			connectedCallback() { childConnections++; }
		}
		customElements.define('x-reparent-parent', ParentElement);
		customElements.define('x-reparent-child', ChildElement);
		const parent = document.createElement('x-reparent-parent');
		parent.appendChild(document.createElement('x-reparent-child'));
		document.body.appendChild(parent);
		if (childConnections !== 1) throw new Error('child connected reaction count: ' + childConnections);
	`, "custom-element-connection-reparent-once.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementNestedReactionQueueOrdering(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	err = r.Run(`
		const removalLog = [];
		customElements.define('x-reaction-parent-remove', class extends HTMLElement {
			connectedCallback() { removalLog.push('begin'); this.firstChild.remove(); removalLog.push('end'); }
		});
		customElements.define('x-reaction-child-remove', class extends HTMLElement {
			connectedCallback() { removalLog.push('connected'); }
			disconnectedCallback() { removalLog.push('disconnected'); }
		});
		const removalParent = document.createElement('x-reaction-parent-remove');
		removalParent.appendChild(document.createElement('x-reaction-child-remove'));
		document.body.appendChild(removalParent);
		if (removalLog.join(',') !== 'begin,connected,disconnected,end') throw new Error('nested removal reactions: ' + removalLog.join(','));

		const deferredLog = [];
		customElements.define('x-reaction-parent-defer', class extends HTMLElement {
			connectedCallback() { deferredLog.push('begin'); this.firstChild.remove(); deferredLog.push('end'); }
		});
		customElements.define('x-reaction-child-defer', class extends HTMLElement {
			connectedCallback() { deferredLog.push('connected'); }
		});
		const deferredParent = document.createElement('x-reaction-parent-defer');
		deferredParent.appendChild(document.createElement('x-reaction-child-defer'));
		document.body.appendChild(deferredParent);
		if (deferredLog.join(',') !== 'begin,end,connected') throw new Error('null-disconnect callback ordering: ' + deferredLog.join(','));

		const adoptionLog = [];
		const targetDocument = document.implementation.createHTMLDocument('target');
		customElements.define('x-reaction-parent-adopt', class extends HTMLElement {
			connectedCallback() { adoptionLog.push('begin'); targetDocument.adoptNode(this.firstChild); adoptionLog.push('end'); }
		});
		customElements.define('x-reaction-child-adopt', class extends HTMLElement {
			connectedCallback() { adoptionLog.push('connected'); }
			adoptedCallback() { adoptionLog.push('adopted'); }
		});
		const adoptionParent = document.createElement('x-reaction-parent-adopt');
		adoptionParent.appendChild(document.createElement('x-reaction-child-adopt'));
		document.body.appendChild(adoptionParent);
		if (adoptionLog.join(',') !== 'begin,connected,adopted,end') throw new Error('nested adoption reactions: ' + adoptionLog.join(','));
	`, "custom-element-nested-reaction-queue.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementConstructorAttributeChangesDoNotQueueAttributeCallbacks(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body><x-constructor-mutation></x-constructor-mutation></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const log = [];
		class ConstructorMutation extends HTMLElement {
			constructor() {
				super();
				this.id = 'assigned';
				this.setAttribute('id', 'assigned');
				this.removeAttribute('id');
				this.style.fontSize = '10px';
				log.push('constructor');
			}
			connectedCallback() { log.push('connected'); }
			attributeChangedCallback() { log.push('attribute'); }
			static get observedAttributes() { return ['id', 'style']; }
		}
		customElements.define('x-constructor-mutation', ConstructorMutation);
		if (log.join('|') !== 'constructor|connected') throw new Error('constructor reactions: ' + log.join('|'));
	`, "custom-element-constructor-attribute-reactions.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementAdoptedReactionDrainsMutatedElementQueue(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const log = [];
		let first, second, third;
		class QueueElement extends HTMLElement {
			adoptedCallback() {
				log.push('adopted:' + (this === first ? 'first' : this === second ? 'second' : 'third'));
				if (this === first) third.setAttribute('id', 'foo');
			}
			connectedCallback() { log.push('connected:' + (this === first ? 'first' : this === second ? 'second' : 'third')); }
			attributeChangedCallback() { log.push('attribute:third'); }
			static get observedAttributes() { return ['id']; }
		}
		customElements.define('x-reaction-queue', QueueElement);
		first = document.createElement('x-reaction-queue');
		second = document.createElement('x-reaction-queue');
		third = document.createElement('x-reaction-queue');
		const container = document.createElement('div');
		container.appendChild(first); container.appendChild(second); container.appendChild(third);
		log.length = 0;
		const destination = document.implementation.createHTMLDocument('destination');
		destination.documentElement.appendChild(container);
		const expected = 'adopted:first|adopted:third|connected:third|attribute:third|connected:first|adopted:second|connected:second';
		if (log.join('|') !== expected) throw new Error('reaction queue order: ' + log.join('|'));
	`, "custom-element-adopted-reaction-queue.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestElementInternalsSeesConstructorAttachedClosedShadowRoot(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		let constructed = false;
		let failure = '';
		customElements.define('x-internals-closed-shadow', class extends HTMLElement {
			constructor() {
				super();
				try {
					if (this.shadowRoot !== null) throw new Error('closed shadow root leaked before attachment');
					const internals = this.attachInternals();
					if (internals.shadowRoot !== null) throw new Error('pre-attach shadowRoot should be null');
					const shadow = this.attachShadow({mode: 'closed'});
					if (internals.shadowRoot !== shadow) throw new Error('internals did not return the closed shadow root');
					if (this.shadowRoot !== null) throw new Error('closed shadow root was exposed on the element');
					constructed = true;
				} catch (error) { failure = error.message; throw error; }
			}
		});
		document.createElement('x-internals-closed-shadow');
		if (!constructed) throw new Error('custom element constructor did not complete: ' + failure);
	`, "element-internals-closed-shadow.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestElementInternalsDoesNotRevealPreAttachedClosedShadowRoot(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const element = document.createElement('x-pre-attached-shadow');
		const root = element.attachShadow({mode: 'closed'});
		let constructed = false;
		customElements.define('x-pre-attached-shadow', class extends HTMLElement {
			constructor() {
				super();
				const internals = this.attachInternals();
				if (internals.shadowRoot !== null) throw new Error('ElementInternals exposed a pre-attached closed shadow root');
				constructed = true;
			}
		});
		customElements.upgrade(element);
		if (!constructed) throw new Error('custom element was not upgraded');
		if (element.shadowRoot !== null) throw new Error('closed shadow root leaked through Element.shadowRoot');
		if (!root) throw new Error('pre-attached root was lost');
	`, "element-internals-pre-attached-shadow-root.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomStatePseudoClassMatchesPlainIdentifierState(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		class StateElement extends HTMLElement {
			constructor() { super(); this.internals = this.attachInternals(); }
		}
		customElements.define('x-state-ident', StateElement);
		const element = new StateElement();
		if (element.matches(':state(foo)')) throw new Error('unset state matched');
		element.internals.states.add('foo');
		if (!element.matches(':state(foo)')) throw new Error('plain identifier state did not match');
		if (!element.matches(':is(:state(foo))')) throw new Error(':is state did not match');
		element.classList.add('c1', 'c2');
		if (!element.matches('.c1:state(foo)')) throw new Error('class before state did not match');
		if (!element.matches(':state(foo).c1')) throw new Error('state before class did not match');
		if (!element.matches('.c2:state(foo).c1')) throw new Error('compound selector did not match');
	`, "custom-state-pseudo-identifier.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDefinedPseudoClassTracksCustomElementState(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body><x-pending></x-pending><font-face></font-face><svg><x-pending></x-pending></svg></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const parsed = document.querySelector('x-pending');
		if (!parsed.matches(':not(:defined)')) throw new Error('unresolved parser-created custom element matched :defined');
		if (!document.querySelector('font-face').matches(':defined')) throw new Error('reserved custom-element name was treated as unresolved');
		if (!document.querySelector('svg x-pending').matches(':defined')) throw new Error('foreign-namespace element matched :not(:defined)');

		const created = document.createElement('x-created');
		if (!created.matches(':not(:defined)')) throw new Error('unresolved createElement candidate matched :defined');
		const createdNS = document.createElementNS('http://www.w3.org/1999/xhtml', 'x-created-ns');
		if (!createdNS.matches(':not(:defined)')) throw new Error('unresolved createElementNS candidate matched :defined');
		document.body.appendChild(createdNS);

		let constructorSawUndefined = false;
		customElements.define('x-pending', class extends HTMLElement {
			constructor() {
				constructorSawUndefined = parsed.matches(':not(:defined)');
				super();
				if (!this.matches(':not(:defined)')) throw new Error('element became defined before its constructor returned');
			}
		});
		if (!constructorSawUndefined) throw new Error('constructor did not see the unresolved state');
		if (!parsed.matches(':defined')) throw new Error('upgraded element did not match :defined');
		if (parsed.matches(':not(:defined)')) throw new Error('upgraded element still matched :not(:defined)');
		customElements.define('x-created-ns', class extends HTMLElement {});
		if (!createdNS.matches(':defined')) throw new Error('createElementNS candidate did not become defined');

		customElements.define('x-failed', class extends HTMLElement {
			constructor() { throw new Error('intentional failure'); }
		});
		const failed = document.createElement('x-failed');
		if (!failed.matches(':defined')) throw new Error('failed custom element did not match :defined');
	`, "custom-element-defined-pseudo.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomElementCrossRealmExceptionsReportToDefinitionRealm(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const first = document.createElement('iframe');
		const second = document.createElement('iframe');
		first.srcdoc = '';
		second.srcdoc = '';
		document.body.append(first, second);
		const reports = [];
		first.contentWindow.onerror = () => reports.push('first');
		second.contentWindow.onerror = () => reports.push('second');
		window.onerror = () => reports.push('top');
		let sawSecondErrorEvent = false;
		let secondErrorEventTargetMatches = false;
		second.contentWindow.addEventListener('error', event => { sawSecondErrorEvent = true; secondErrorEventTargetMatches = event.target === second.contentWindow; });

		const throwsFromSecondRealm = new second.contentWindow.Function('throw new parent.frames[1].Error("reported")');
		first.contentWindow.customElements.define('x-cross-realm-throw', throwsFromSecondRealm);
		const failedElement = first.contentDocument.createElement('x-cross-realm-throw');
		if (reports.join(',') !== 'second') throw new Error('constructor exception reported to ' + reports.join(','));
		if (!sawSecondErrorEvent) throw new Error('source realm error event was not delivered to its Window listener');
		if (!secondErrorEventTargetMatches) throw new Error('source realm error event had the wrong target');

		reports.length = 0;
		class CallbackElement extends first.contentWindow.HTMLElement {}
		CallbackElement.prototype.connectedCallback = new second.contentWindow.Function('throw new parent.frames[1].Error("reported")');
		first.contentWindow.customElements.define('x-cross-realm-callback', CallbackElement);
		first.contentDocument.body.append(first.contentDocument.createElement('x-cross-realm-callback'));
		if (reports.join(',') !== 'second') throw new Error('callback exception reported to ' + reports.join(','));
	`, "cross-realm-custom-element-exceptions.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIframeImportedErrorConstructorPreservesPrototypeIdentity(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const errorFrame = document.createElement('iframe');
		errorFrame.srcdoc = '';
		document.body.append(errorFrame);
		const ErrorConstructor = errorFrame.contentWindow.Error;
		const error = new ErrorConstructor('cross realm');
		if (Object.getPrototypeOf(error) !== ErrorConstructor.prototype) throw new Error('cross-realm Error prototype identity changed');
		if (!(error instanceof ErrorConstructor)) throw new Error('cross-realm Error instance failed instanceof');
	`, "cross-realm-error-constructor-prototype.js"); err != nil {
		t.Fatalf("Run cross-realm Error constructor case: %v", err)
	}
}

func TestCustomElementCrossRealmFailureUsesDestinationInterfacePrototype(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const definitionFrame = document.createElement('iframe');
		const constructionFrame = document.createElement('iframe');
		definitionFrame.srcdoc = '';
		constructionFrame.srcdoc = '';
		document.body.append(definitionFrame, constructionFrame);
		const definitionWindow = definitionFrame.contentWindow;
		const constructionWindow = constructionFrame.contentWindow;
		const ForeignElement = definitionWindow.Function('return class extends HTMLElement { constructor() { throw new Error("cross-realm failure"); } }')();
		constructionWindow.customElements.define('x-cross-realm-fallback', ForeignElement);
		if (!constructionWindow.customElements.get('x-cross-realm-fallback')) throw new Error('destination registry lost its definition');
		globalThis.__crossRealmFallbackEvent = 'pending';
		const nextError = new Promise(resolve => definitionWindow.addEventListener('error', event => {
			resolve(event);
			event.preventDefault();
		}, {once: true}));
		const element = constructionWindow.document.createElement('x-cross-realm-fallback');
		const unknownPrototype = constructionWindow.HTMLUnknownElement.prototype;
		if (Object.getPrototypeOf(element) !== unknownPrototype) throw new Error('failed element did not adopt the destination HTMLUnknownElement prototype');
		if (!(element instanceof constructionWindow.HTMLUnknownElement)) throw new Error('failed element was not an instance of the destination HTMLUnknownElement');
		nextError.then(event => {
			globalThis.__crossRealmFallbackEvent = event.target === definitionWindow && Object.getPrototypeOf(element) === unknownPrototype && window.event === event ? 'ok' : 'mismatch';
		});
	`, "cross-realm-custom-element-fallback-prototype.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := r.Run(`if (globalThis.__crossRealmFallbackEvent !== 'ok') throw new Error('error event continuation observed the wrong target or fallback prototype: ' + globalThis.__crossRealmFallbackEvent);`, "cross-realm-custom-element-fallback-prototype-check.js"); err != nil {
		t.Fatalf("Run async continuation check: %v", err)
	}
}

func TestCustomElementCrossRealmErrorEventSurvivesPromiseContinuation(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const definitionFrame = document.createElement('iframe');
		const constructionFrame = document.createElement('iframe');
		document.body.append(definitionFrame, constructionFrame);
		const definitionWindow = definitionFrame.contentWindow;
		const constructionWindow = constructionFrame.contentWindow;
		const ForeignElement = definitionWindow.Function('return class extends HTMLElement { constructor() { throw new Error("cross-realm failure"); } }')();
		constructionWindow.customElements.define('x-cross-realm-error-event', ForeignElement);
		globalThis.__crossRealmErrorEventCheck = 'pending';
		const nextError = new Promise(resolve => definitionWindow.addEventListener('error', resolve, { once: true }));
		constructionWindow.document.createElement('x-cross-realm-error-event');
		nextError.then(errorEvent => {
			globalThis.__crossRealmErrorEventCheck = errorEvent.target === definitionWindow && window.event === errorEvent ? 'ok' : 'mismatch';
		});
	`, "cross-realm-custom-element-error-event.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := r.Run(`
		if (globalThis.__crossRealmErrorEventCheck !== 'ok') throw new Error('window.event did not retain the imported error event and target through the promise continuation: ' + globalThis.__crossRealmErrorEventCheck);
	`, "cross-realm-custom-element-error-event-check.js"); err != nil {
		t.Fatalf("Run async continuation check: %v", err)
	}
}

func TestCustomElementCrossRealmInvalidReturnReportsToConstructorRealm(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const definitionFrame = document.createElement('iframe');
		const constructionFrame = document.createElement('iframe');
		document.body.append(definitionFrame, constructionFrame);
		const definitionWindow = definitionFrame.contentWindow;
		const constructionWindow = constructionFrame.contentWindow;
		const ForeignDiv = new definitionWindow.Function('return class extends HTMLDivElement { constructor() { return document.createElement("div"); } }')();
		constructionWindow.customElements.define('x-cross-realm-invalid-return', ForeignDiv, {extends: 'div'});
		let targetWasDefinitionWindow = false;
		let currentTargetWasDefinitionWindow = false;
		definitionWindow.addEventListener('error', event => {
			targetWasDefinitionWindow = event.target === definitionWindow;
			currentTargetWasDefinitionWindow = event.currentTarget === definitionWindow;
			event.preventDefault();
		}, {once: true});
		const element = constructionWindow.document.createElement('div', {is: 'x-cross-realm-invalid-return'});
		if (Object.getPrototypeOf(element) !== constructionWindow.HTMLDivElement.prototype) throw new Error('failed customized built-in did not retain its native interface prototype');
		if (!targetWasDefinitionWindow || !currentTargetWasDefinitionWindow) throw new Error('invalid constructor return was not reported to the constructor realm');
	`, "cross-realm-customized-built-in-invalid-return.js"); err != nil {
		t.Fatalf("Run cross-realm invalid-return case: %v", err)
	}
}

func TestIframeImportedConstructorPreservesNewTarget(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const frame = document.createElement('iframe');
		const childWindow = frame.contentWindow;
		const ChildConstructor = childWindow.Function('return new.target');
		function ParentNewTarget() {}
		const result = Reflect.construct(ChildConstructor, [], ParentNewTarget);
		if (result !== ParentNewTarget) throw new Error('iframe constructor lost the caller new.target');
	`, "iframe-constructor-new-target.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIframeImportedObjectPreservesPrototypeIdentity(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const frame = document.createElement('iframe');
		const childWindow = frame.contentWindow;
		const createWithPrototype = childWindow.Function('const proto = { realm: "child" }; return [Object.create(proto), proto]');
		const [value, proto] = createWithPrototype();
		if (Object.getPrototypeOf(value) !== proto) throw new Error('iframe object prototype identity was lost during import');
		if (proto.realm !== 'child') throw new Error('iframe prototype properties were lost during import');
		const childElement = childWindow.document.createElement('div');
		const childUnknownPrototype = childWindow.HTMLUnknownElement.prototype;
		const changeChildPrototype = childWindow.Function('node', 'prototype', 'Object.setPrototypeOf(node, prototype)');
		changeChildPrototype(childElement, childUnknownPrototype);
		if (Object.getPrototypeOf(childElement) !== childUnknownPrototype) throw new Error('iframe Node proxy did not expose the source wrapper\'s updated prototype');
	`, "iframe-object-prototype-identity.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestHTMLConstructorUsesInterfacePrototypeWhenNewTargetPrototypeIsNotObject(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		let returnPrimitive = false;
		function ParagraphElement() {
			return Reflect.construct(HTMLParagraphElement, [], new.target);
		}
		const DynamicConstructor = new Proxy(ParagraphElement, {
			get(target, property) {
				if (property === 'prototype') return returnPrimitive ? 5 : {};
				return target[property];
			}
		});
		customElements.define('x-dynamic-p', DynamicConstructor, { extends: 'p' });
		returnPrimitive = true;
		const element = new DynamicConstructor();
		if (Object.getPrototypeOf(element) !== HTMLParagraphElement.prototype) {
			throw new Error('HTML constructor did not use the interface fallback prototype');
		}
		let classReturnsPrimitive = false;
		function MatchingParagraphElement() {
			return Reflect.construct(HTMLParagraphElement, [], new.target);
		}
		MatchingParagraphElement.prototype = Object.create(HTMLParagraphElement.prototype);
		const DynamicClass = new Proxy(MatchingParagraphElement, {
			get(target, property) {
				if (property === 'prototype') return classReturnsPrimitive ? 5 : target.prototype;
				return target[property];
			}
		});
		customElements.define('x-dynamic-class-p', DynamicClass, { extends: 'p' });
		classReturnsPrimitive = true;
		const classElement = new DynamicClass();
		if (Object.getPrototypeOf(classElement) !== HTMLParagraphElement.prototype) {
			throw new Error('matching HTML constructor did not use the interface fallback prototype');
		}
	`, "html-constructor-new-target-fallback.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDefinedPseudoClassUpgradesCreateElementNSInIframeDocument(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const w = frame.contentWindow;
		const d = frame.contentDocument;
		const element = d.createElementNS('http://www.w3.org/1999/xhtml', 'x-frame-defined');
		d.body.appendChild(element);
		if (!element.matches(':not(:defined)')) throw new Error('iframe createElementNS candidate was defined before registration');
		const created = d.createElement('x-frame-created');
		d.body.appendChild(created);
		class FrameElement extends w.HTMLElement {}
		w.customElements.define('x-frame-defined', FrameElement);
		if (!element.matches(':defined')) throw new Error('iframe createElementNS candidate remained undefined after registration');
		if (!(element instanceof FrameElement)) throw new Error('iframe createElementNS candidate was not upgraded');
		class CreatedFrameElement extends w.HTMLElement {}
		w.customElements.define('x-frame-created', CreatedFrameElement);
		if (!created.matches(':defined')) throw new Error('iframe createElement candidate remained undefined after registration');
		if (!(created instanceof CreatedFrameElement)) throw new Error('iframe createElement candidate was not upgraded');
	`, "iframe-custom-element-defined-pseudo.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDefinedPseudoClassUpgradesIframeSrcdocCreateElement(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body><iframe id="iframe"></iframe></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var frameDone = false;
		var frameError = '';
		const frame = document.getElementById('iframe');
		frame.onload = () => {
			try {
				const w = frame.contentWindow;
				const d = frame.contentDocument;
			const detached = d.implementation.createHTMLDocument();
			const element = d.createElement('a-a');
			d.body.appendChild(element);
			const htmlElement = d.createElementNS('http://www.w3.org/1999/xhtml', 'a-a');
			d.body.appendChild(htmlElement);
			const svgElement = d.createElementNS('http://www.w3.org/2000/svg', 'a-a');
			d.body.appendChild(svgElement);
			detached.createElement('a-a');
			if (!element.matches(':not(:defined)')) throw new Error('candidate defined before registration');
				class A extends w.HTMLElement {}
				w.customElements.define('a-a', A);
				if (!element.matches(':defined')) throw new Error('candidate remained undefined after registration');
			if (!htmlElement.matches(':defined')) throw new Error('createElementNS candidate remained undefined after registration');
			if (!svgElement.matches(':defined')) throw new Error('SVG element not defined');
			} catch (error) { frameError = String(error); }
			frameDone = true;
		};
		frame.srcdoc = '<!doctype html><html><head><style>* { color: red; } :defined { color: orange; }</style></head><body><div></div></body></html>';
		document.body.appendChild(frame);
	`, "iframe-srcdoc-defined-pseudo.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for i := 0; i < 5; i++ {
		r.Tick()
	}
	state, err := r.Evaluate(`String(frameDone) + '|' + frameError`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if state != "true|" {
		t.Fatalf("srcdoc createElement upgrade state = %q, want true|", state)
	}
}

func TestDeclarativeShadowRootDefaultsToDocumentCustomElementRegistry(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><div id="host"><template shadowrootmode="open" shadowrootclonable="true"><a-b></a-b><ef></ef></template></div></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		const host = document.getElementById('host');
		if (!host.shadowRoot) throw new Error('declarative shadow root was not created');
		if (host.shadowRoot.customElementRegistry !== customElements) throw new Error('declarative shadow root did not inherit the document registry');
		if (host.shadowRoot.querySelector('a-b').customElementRegistry !== customElements) throw new Error('declarative shadow descendant did not inherit the document registry');
		const clone = host.cloneNode(true);
		if (clone.shadowRoot.customElementRegistry !== customElements) throw new Error('cloned declarative shadow root did not inherit the document registry');
		if (clone.shadowRoot.querySelector('a-b').customElementRegistry !== customElements) throw new Error('cloned declarative shadow descendant did not inherit the document registry');
		customElements.initialize(clone.shadowRoot);
		if (clone.shadowRoot.customElementRegistry !== customElements) throw new Error('initialize changed the root registry');
		if (clone.shadowRoot.querySelector('a-b').customElementRegistry !== customElements) throw new Error('initialize changed the descendant registry');
	`, "declarative-shadow-registry-default.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestNullRegistryDeclarativeShadowDescendantAdoption(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><div id="host"><template shadowrootmode="open" shadowrootclonable="true" shadowrootcustomelementregistry><a-b></a-b></template></div></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const clone = document.getElementById('host').cloneNode(true);
		const child = clone.shadowRoot.querySelector('a-b');
		document.body.appendChild(child);
		const sameDocumentNull = child.customElementRegistry === null;
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const frameWindow = frame.contentWindow;
		const frameDocument = frame.contentDocument;
		frameDocument.body.appendChild(child);
		globalThis.adoptionState = [sameDocumentNull, child.customElementRegistry === frameWindow.customElements, child.ownerDocument === frameDocument].join('|');
	`, "null-registry-shadow-adoption.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	state, err := r.Evaluate(`adoptionState`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if state != "true|true|true" {
		t.Fatalf("declarative shadow descendant adoption state = %q, want true|true|true", state)
	}
}

func TestGlobalRegistryInitializedShadowRootMovesToDestinationDocument(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><div id="host"><template shadowrootmode="open" shadowrootclonable="true" shadowrootcustomelementregistry></template></div></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const host = document.getElementById('host').cloneNode(true);
		const root = host.shadowRoot;
		customElements.initialize(root);
		const child = document.createElement('x-child', {customElementRegistry: null});
		root.appendChild(child);
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const targetWindow = frame.contentWindow;
		frame.contentDocument.body.appendChild(host);
		globalThis.registryMoveState = [
			root.customElementRegistry === targetWindow.customElements,
			child.customElementRegistry === targetWindow.customElements,
			root.customElementRegistry === customElements,
			child.customElementRegistry === customElements
		].join('|');
	`, "global-registry-shadow-adoption.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	state, err := r.Evaluate(`registryMoveState`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if state != "true|true|false|false" {
		t.Fatalf("global registry move state = %q, want destination registry for root and descendants", state)
	}
}

func TestLateSetDOMBindsGlobalRegistryDocumentForAdoption(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	r.SetDOM(domtest.Parse(`<!doctype html><html><body></body></html>`))
	if err := r.Run(`
		const host = document.createElement('div');
		const root = host.attachShadow({mode: 'open', customElementRegistry: customElements});
		const child = document.createElement('a-b', {customElementRegistry: customElements});
		root.appendChild(child);
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const targetWindow = frame.contentWindow;
		frame.contentDocument.body.appendChild(host);
		globalThis.lateDocumentRegistryState = [
			root.customElementRegistry === targetWindow.customElements,
			child.customElementRegistry === targetWindow.customElements,
			root.customElementRegistry === customElements,
			child.customElementRegistry === customElements
		].join('|');
	`, "late-setdom-custom-element-registry-adoption.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	state, err := r.Evaluate(`lateDocumentRegistryState`)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if state != "true|true|false|false" {
		t.Fatalf("late SetDOM registry move state = %q, want destination registry for root and descendant", state)
	}
}

func TestDocumentStyleSheetRuleListsSupportIndexedAccess(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head><style>div { color: red; } p { color: blue; }</style></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		if (!document.styleSheets || document.styleSheets.length !== 1) throw new Error('stylesheet list missing');
		const sheet = document.styleSheets[0];
		if (!sheet) throw new Error('indexed stylesheet missing');
		if (sheet.cssRules.length !== 2) throw new Error('cssRules length: ' + sheet.cssRules.length);
		if (sheet.cssRules[1].selectorText !== 'p') throw new Error('indexed cssRule missing');
	`, "stylesheet-indexed-lists.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestDocumentStyleSheetsInstallAfterLateDOMBinding(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	r.SetDOM(domtest.Parse(`<!doctype html><html><head><style>:state( \(escaped\ state  ) { color: red; }</style></head><body></body></html>`))
	if err := r.Run(`
		if (document.styleSheets.length !== 1) throw new Error('late document.styleSheets missing');
		if (document.styleSheets[0].cssRules.length !== 1) throw new Error('late CSSRuleList missing');
		if (document.styleSheets[0].cssRules[0].selectorText !== ':state(\\(escaped\\ state)') throw new Error('selectorText: ' + document.styleSheets[0].cssRules[0].selectorText);
	`, "late-dom-cssom.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCustomizedBuiltinsCreateAndUpgradeWithTheirNativeInterface(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		const reflectedChanges = [];
		class CustomButton extends HTMLButtonElement {
			static observedAttributes = ['disabled'];
			attributeChangedCallback(name, oldValue, newValue) { reflectedChanges.push(name + ':' + oldValue + '>' + newValue); }
		}
		customElements.define('x-custom-button', CustomButton, {extends: 'button'});
		const button = document.createElement('button', {is: 'x-custom-button'});
		button.disabled = true;
		if (reflectedChanges.join('|') !== 'disabled:null>') throw new Error('reflected property must enqueue the custom-element reaction: ' + reflectedChanges.join('|'));
		const videoChanges = [];
		class CustomVideo extends HTMLVideoElement {
			static observedAttributes = ['src', 'controls'];
			attributeChangedCallback(name) { videoChanges.push(name); }
		}
		customElements.define('x-custom-video', CustomVideo, {extends: 'video'});
		const video = document.createElement('video', {is: 'x-custom-video'});
		video.src = '/media/test.webm';
		video.controls = true;
		if (videoChanges.join('|') !== 'src|controls') throw new Error('video reflected properties must enqueue reactions: ' + videoChanges.join('|'));
		const meterChanges = [];
		class CustomMeter extends HTMLMeterElement {
			static observedAttributes = ['value'];
			attributeChangedCallback(name) { meterChanges.push(name); }
		}
		customElements.define('x-custom-meter', CustomMeter, {extends: 'meter'});
		const meter = document.createElement('meter', {is: 'x-custom-meter'});
		meter.value = 0.5;
		if (meterChanges.join('|') !== 'value') throw new Error('meter numeric reflection must enqueue reactions: ' + meterChanges.join('|'));

		class CustomParagraph extends HTMLParagraphElement {
			constructor() { super(); this.wasConstructed = true; }
		}
		customElements.define('x-custom-paragraph', CustomParagraph, {extends: 'p'});
		const plain = document.createElement('p', 'x-custom-paragraph');
		const customized = document.createElement('p', {is: 'x-custom-paragraph'});
		const beforeDefinition = document.createElement('div', {is: 'x-late-div'});
		class DirectDiv extends HTMLDivElement {}
		customElements.define('x-late-div', DirectDiv, {extends: 'div'});
		document.body.appendChild(beforeDefinition);
		const direct = new DirectDiv();
		if (!(plain instanceof HTMLParagraphElement) || plain instanceof CustomParagraph) throw new Error('string options must be ignored');
		if (!(customized instanceof HTMLParagraphElement) || !(customized instanceof CustomParagraph) || !customized.wasConstructed) throw new Error('customized paragraph construction');
		if (beforeDefinition.hasAttribute('is') || !(beforeDefinition instanceof DirectDiv)) throw new Error('undefined customized built-in upgrade');
		if (direct.localName !== 'div' || !direct.hasAttribute('is') || !(direct instanceof HTMLDivElement)) throw new Error('direct customized built-in construction');
		class CustomAddress extends HTMLElement {}
		customElements.define('x-address', CustomAddress, {extends: 'address'});
		const namespaced = document.createElementNS('http://www.w3.org/1999/xhtml', 'p:address', {is: 'x-address'});
		if (!(namespaced instanceof CustomAddress) || namespaced.prefix !== 'p' || namespaced.hasAttribute('is')) throw new Error('namespaced customized built-in creation');
	`, "customized-builtins.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestEmptyIsValueLeavesCustomizedBuiltInUndefined(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body><p is=""></p></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const parsed = document.querySelector('p[is=""]');
		const created = document.createElement('p', {is: ''});
		const namespaced = document.createElementNS('http://www.w3.org/1999/xhtml', 'p', {is: ''});
		if (parsed.matches(':defined')) throw new Error('parsed empty is value must remain undefined');
		if (created.matches(':defined')) throw new Error('createElement empty is value must remain undefined');
		if (namespaced.matches(':defined')) throw new Error('createElementNS empty is value must remain undefined');
	`, "empty-customized-built-in-is.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestParserCustomElementConstructorBlocksDynamicMarkupInsertion(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body><div id="host"></div></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const operations = {
			open: () => document.open(),
			write: () => document.write('<b>forbidden</b>'),
			writeln: () => document.writeln('<b>forbidden</b>'),
			close: () => document.close()
		};
		for (const [name, operation] of Object.entries(operations)) {
			let errorName = null;
			class ParserGuard extends HTMLElement {
				constructor() {
					super();
					if (this.ownerDocument !== document) throw new Error('constructor observed temporary parser document');
					try { operation(); } catch (error) { errorName = error.name; }
				}
			}
			customElements.define('x-parser-guard-' + name, ParserGuard);
			document.open();
			document.write('<!doctype html><html><body><x-parser-guard-' + name + '></x-parser-guard-' + name + '></body></html>');
			if (errorName !== 'InvalidStateError') throw new Error(name + ' did not throw InvalidStateError: ' + errorName);
			if (document.querySelector('b')) throw new Error(name + ' inserted forbidden markup');
			document.close();
		}
	`, "custom-element-parser-markup-counter.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIframeParserCustomElementConstructorUsesDocumentCounter(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var iframeParserFrames = [];
		for (var i = 0; i < 4; i++) {
			var frame = document.createElement('iframe');
			frame.srcdoc = '';
			document.body.appendChild(frame);
			iframeParserFrames.push(frame);
		}
	`, "iframe-custom-element-parser-frame.js"); err != nil {
		t.Fatalf("create iframe: %v", err)
	}
	if err := r.Run(`
		for (let i = 0; i < iframeParserFrames.length; i++) {
			const frame = iframeParserFrames[i];
			const childDocument = frame.contentDocument;
			const operations = [
				() => childDocument.open(),
				() => childDocument.write('<b>forbidden</b>'),
				() => childDocument.writeln('<b>forbidden</b>'),
				() => childDocument.close()
			];
			let constructed = false;
			let errorName = null;
			class IframeParserGuard extends HTMLElement {
				constructor() {
					super();
					constructed = true;
					if (this.ownerDocument !== childDocument) throw new Error('constructor saw wrong ownerDocument');
					try { operations[i](); } catch (error) { errorName = error.name; }
				}
			}
			frame.contentWindow.customElements.define('x-iframe-parser-guard-' + i, IframeParserGuard);
			childDocument.open();
			childDocument.write('<!doctype html><html><body><x-iframe-parser-guard-' + i + '></x-iframe-parser-guard-' + i + '></body></html>');
			if (!constructed) throw new Error('iframe parser did not construct element for operation ' + i);
			if (errorName !== 'InvalidStateError') throw new Error('iframe operation ' + i + ' did not throw: ' + errorName);
			if (childDocument.querySelector('b')) throw new Error('iframe constructor inserted forbidden markup');
			childDocument.close();
		}
	`, "iframe-custom-element-parser-markup-counter.js"); err != nil {
		t.Fatalf("iframe parser: %v", err)
	}
}

func TestIframeLoadDocumentWriteSynchronouslyUpgradesCustomElements(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		frame.onload = () => { window.iframeLoadForParserTest = true; };
		document.body.appendChild(frame);
		window.iframeForParserTest = frame;
	`, "iframe-parser-load-window.js"); err != nil {
		t.Fatalf("create iframe: %v", err)
	}
	r.FlushIFrameLoads()
	if err := r.Run(`
		if (!iframeLoadForParserTest) throw new Error('srcdoc iframe load did not fire');
		const childWindow = iframeForParserTest.contentWindow;
		const childDocument = childWindow.document;
		const operations = [
			['open', () => childDocument.open()],
			['close', () => childDocument.close()],
			['write', () => childDocument.write('<b>forbidden</b>')],
			['writeln', () => childDocument.writeln('<b>forbidden</b>')]
		];
		for (let i = 0; i < operations.length; i++) {
			childDocument.open();
			let constructed = false;
			let exceptionName = null;
			class WrittenCustomElement extends childWindow.HTMLElement {
				constructor() {
					super();
					constructed = true;
					try { operations[i][1](); } catch (error) { exceptionName = error.name; }
				}
			}
			childWindow.customElements.define('x-written-from-iframe-' + i, WrittenCustomElement);
			childDocument.write('<!doctype html><html><body><x-written-from-iframe-' + i + '></x-written-from-iframe-' + i + '></body></html>');
			childDocument.close();
			if (!constructed) throw new Error(operations[i][0] + ' did not synchronously construct the custom element');
			if (exceptionName !== 'InvalidStateError') throw new Error(operations[i][0] + ' threw ' + exceptionName);
			if (childDocument.querySelector('b')) throw new Error(operations[i][0] + ' inserted markup');
		}
	`, "iframe-parser-load-document-write.js"); err != nil {
		t.Fatalf("write iframe document: %v", err)
	}
}

func TestIframeParserCustomElementConstructorFromPromiseJob(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.appendChild(frame);
		window.promiseParserFrame = frame;
	`, "iframe-parser-promise-frame.js"); err != nil {
		t.Fatalf("create iframe: %v", err)
	}
	r.FlushIFrameLoads()
	if err := r.Run(`
		Promise.resolve().then(() => {
			const childWindow = promiseParserFrame.contentWindow;
			const childDocument = childWindow.document;
			childDocument.open();
			let constructed = false;
			class PromiseParserElement extends childWindow.HTMLElement {
				constructor() { super(); constructed = true; }
			}
			childWindow.customElements.define('x-promise-parser-element', PromiseParserElement);
			childDocument.write('<!doctype html><html><body><x-promise-parser-element></x-promise-parser-element></body></html>');
			childDocument.close();
			window.promiseParserConstructed = constructed;
		});
	`, "iframe-parser-promise-job.js"); err != nil {
		t.Fatalf("write iframe document from promise job: %v", err)
	}
	if err := r.Run(`
		if (window.promiseParserConstructed !== true) throw new Error('promise job did not observe constructor completion');
	`, "iframe-parser-promise-assert.js"); err != nil {
		t.Fatalf("assert iframe parser: %v", err)
	}
}

func TestDocumentWriteDefersParserCheckpointUntilJavaScriptReturns(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const events = [];
		Promise.resolve().then(() => events.push('promise'));
		class WrittenElement extends HTMLElement {
			constructor() { super(); events.push('constructor'); }
		}
		customElements.define('x-written-checkpoint', WrittenElement);
		document.open();
		document.write('<!doctype html><html><body><x-written-checkpoint></x-written-checkpoint></body></html>');
		events.push('after');
		Promise.resolve().then(() => {
			if (events.join('|') !== 'constructor|after|promise') throw new Error('document.write checkpoint order: ' + events.join('|'));
		});
	`, "document-write-custom-element-microtask-order.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestFormAssociatedCustomElementInternals(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><label for="control" id="label">Control</label><form id="owner"><x-internals id="control" name="control"><span id="anchor"></span></x-internals></form></body></html>`)
	r, err := js.New(js.Options{DOM: doc, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		class InternalsControl extends HTMLElement {
			static formAssociated = true;
			constructor() { super(); this.internals = this.attachInternals(); }
		}
		customElements.define('x-internals', InternalsControl);
		var control = document.querySelector('x-internals');
		if (document.getElementById('label').control !== control || document.getElementById('label').form !== document.getElementById('owner')) throw new Error('label control/form association failed');
		if (!(control.internals instanceof ElementInternals)) throw new Error('attachInternals must return ElementInternals');
		if (!(control.internals.states instanceof CustomStateSet) || control.internals.states !== control.internals.states || control.internals.states.size !== 0 || control.internals.states.toString() !== '[object CustomStateSet]') throw new Error('initial CustomStateSet shape or identity is wrong');
		var supportsTypeError = false;
		try { control.internals.states.supports('foo'); } catch (error) { supportsTypeError = error instanceof TypeError; }
		if (!supportsTypeError) throw new Error('supports must reject invalid custom state names');
		control.internals.states.add('--ready').add('--busy').add('--ready');
		if (control.internals.states.size !== 2 || !control.matches(':state(--ready)') || control.matches(':state(--missing)')) throw new Error('custom state matching failed');
		if ([...control.internals.states].join(',') !== '--ready,--busy') throw new Error('CustomStateSet insertion order failed');
		control.internals.states.delete('--ready');
		if (control.matches(':state(--ready)')) throw new Error('deleting a custom state did not update selectors');
		control.internals.states.clear();
		if (control.internals.states.size !== 0) throw new Error('CustomStateSet.clear failed');
		if (control.internals.role !== null || control.internals.ariaLabel !== null) throw new Error('ElementInternals ARIA values must default to null');
		control.internals.role = { toString: function() { return 'switch'; } };
		if (control.internals.role !== 'switch') throw new Error('role must convert to a string');
		control.internals.ariaActiveDescendantElement = document.body;
		if (control.internals.ariaActiveDescendantElement !== document.body) throw new Error('ARIA element reflection lost identity');
		var ariaRefs = [document.body];
		control.internals.ariaLabelledByElements = ariaRefs;
		if (control.internals.ariaLabelledByElements === ariaRefs || !Object.isFrozen(control.internals.ariaLabelledByElements) || control.internals.ariaLabelledByElements[0] !== document.body) throw new Error('ARIA element sequence must be copied and frozen');
		if ('ariaErrorMessageElement' in control.internals) throw new Error('singular ariaErrorMessageElement must not be exposed');
		if (control.internals.form !== document.getElementById('owner')) throw new Error('form owner not exposed');
		if (!(control.internals.labels instanceof NodeList) || control.internals.labels.length !== 1 || !control.internals.labels.hasOwnProperty(0) || control.internals.labels[0].id !== 'label') throw new Error('associated labels not exposed');
		if (document.getElementById('owner').elements.length !== 1 || document.getElementById('owner').elements[0] !== control) throw new Error('form.elements must include form-associated custom elements');
		if (!control.internals.willValidate || !control.internals.validity.valid) throw new Error('initial validity state is wrong');
		control.internals.setFormValue('hello', 'restore');
		if (control.internals.__formValue !== 'hello' || control.internals.__formState !== 'restore') throw new Error('setFormValue state not retained');
		if (new FormData(document.getElementById('owner')).get('control') !== 'hello') throw new Error('setFormValue value must enter form data');
		control.internals.setValidity({ valueMissing: true }, 'required', document.getElementById('anchor'));
		if (control.internals.validity.valid || !control.internals.validity.valueMissing) throw new Error('setValidity flags not reflected');
		if (control.internals.validationMessage !== 'required' || control.internals.validationAnchor.id !== 'anchor') throw new Error('validation details not retained');
		var externalAnchor = '', foreignAnchor = '';
		try { control.internals.setValidity({ valueMissing: true }, 'required', document.body); } catch (error) { externalAnchor = error.name; }
		try { control.internals.setValidity({ valueMissing: true }, 'required', document.createElementNS('urn:foreign', 'x-anchor')); } catch (error) { foreignAnchor = error.name; }
		if (externalAnchor !== 'NotFoundError' || foreignAnchor !== 'TypeError') throw new Error('validation anchor errors are incorrect');
		var invalidCount = 0;
		control.addEventListener('invalid', function(event) { if (event.target !== control || !event.cancelable) throw new Error('invalid event fields'); invalidCount++; });
		if (control.internals.checkValidity() || invalidCount !== 1) throw new Error('checkValidity must dispatch invalid and return false');
		if (document.getElementById('owner').checkValidity() || invalidCount !== 2) throw new Error('form.checkValidity must include custom controls');
		if (!control.matches(':invalid') || document.getElementById('owner').matches(':valid')) throw new Error('validity pseudo-classes must reflect custom validity');
		control.internals.setValidity({}, 'ignored when valid');
		if (control.internals.validationMessage !== '' || !control.matches(':valid') || !document.getElementById('owner').matches(':valid')) throw new Error('cleared validity state must be valid');
		var duplicate = false;
		try { control.attachInternals(); } catch (error) { duplicate = error.name === 'NotSupportedError'; }
		if (!duplicate) throw new Error('second attachInternals must throw NotSupportedError');
		class NonAssociated extends HTMLElement {}
		customElements.define('x-not-associated', NonAssociated);
		var nonAssociated = document.createElement('x-not-associated'), rejected = false;
		var nonAssociatedInternals = nonAssociated.attachInternals();
		try { nonAssociatedInternals.form; } catch (error) { rejected = error.name === 'NotSupportedError'; }
		if (!rejected) throw new Error('ElementInternals.form must reject non-form-associated custom elements');
		class DisabledInternals extends HTMLElement { static disabledFeatures = ['internals']; }
		customElements.define('x-disabled-internals', DisabledInternals);
		var disabledRejected = false;
		try { document.createElement('x-disabled-internals').attachInternals(); } catch (error) { disabledRejected = error.name === 'NotSupportedError'; }
		if (!disabledRejected) throw new Error('disabledFeatures internals must reject attachInternals');
		document.body.insertAdjacentHTML('afterbegin', '<x-internals id="dynamic" name="dynamic"></x-internals>');
		if (!(document.body.firstChild.internals instanceof ElementInternals)) throw new Error('insertAdjacentHTML must upgrade custom elements before returning');
		var typeError = false;
		try { document.body.firstChild.internals.setValidity({ valueMissing: true }); } catch (error) { typeError = error instanceof TypeError; }
		if (!typeError) throw new Error('setValidity requires a message when setting an invalid flag');
	`, "element-internals.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestElementInternalsReportValidityFocusesValidationAnchor(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><form><x-validation id="host"></x-validation></form></body></html>`)
	var focused *dom.Node
	r, err := js.New(js.Options{
		DOM:     doc,
		Timeout: 5 * time.Second,
		OnFocusChange: func(node *dom.Node, focus bool) {
			if focus {
				focused = node
			}
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		class ValidationControl extends HTMLElement {
			static formAssociated = true;
			constructor() {
				super();
				const root = this.attachShadow({mode: 'open'});
				root.innerHTML = '<input id="anchor">';
				this.internals = this.attachInternals();
				this.internals.setValidity({valueMissing: true}, 'required', root.querySelector('input'));
			}
		}
		customElements.define('x-validation', ValidationControl);
		var validationControl = document.querySelector('x-validation');
		if (validationControl.internals.checkValidity()) throw new Error('invalid control passed checkValidity');
	`, "element-internals-report-validity-focus.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if focused != nil {
		t.Fatalf("checkValidity focused %p; it must not move focus", focused)
	}
	if err := r.Run(`
		validationControl.addEventListener('invalid', event => event.preventDefault(), {once: true});
		if (validationControl.internals.reportValidity()) throw new Error('invalid control passed reportValidity');
	`, "element-internals-report-validity-cancel.js"); err != nil {
		t.Fatalf("Run canceled invalid event: %v", err)
	}
	if focused != nil {
		t.Fatalf("canceled reportValidity focused %p; it must not report", focused)
	}
	if err := r.Run(`
		if (validationControl.internals.reportValidity()) throw new Error('invalid control passed reportValidity');
	`, "element-internals-report-validity-focus.js"); err != nil {
		t.Fatalf("Run reportValidity: %v", err)
	}
	host := doc.ElementByID("host")
	if host == nil || host.ShadowRoot == nil {
		t.Fatal("validation control shadow root is missing")
	}
	want := host.ShadowRoot.FirstChild
	if focused != want {
		t.Fatalf("reportValidity focused %p, want validation anchor %p", focused, want)
	}
}

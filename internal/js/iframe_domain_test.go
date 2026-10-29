package js

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestIframeSameOriginDomainRequiresBothDocumentsToRelax(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	parentDoc, err := dom.ParseBounded(`<!doctype html><html><body><iframe id="child"></iframe></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := New(Options{URL: "https://shop.example.test:8443/", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	childDoc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	child, err := New(Options{URL: "https://frame.example.test:9443/", DOM: childDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	frame := parentDoc.ElementByID("child")
	if frame == nil {
		t.Fatal("parsed iframe not found")
	}
	parent.iframeLocations = make(map[*dom.Node]string)
	parent.iframeChildRuntimes = make(map[*dom.Node]*Runtime)
	parent.iframeLocations[frame] = child.opts.URL
	parent.iframeChildRuntimes[frame] = child
	child.iframeParent = parent
	if parent.iframeIsSameOrigin(frame) {
		t.Fatal("different tuple origins were treated as same-origin before relaxation")
	}
	if err := parent.Run(`document.domain = "example.test"`, "parent-domain.js"); err != nil {
		t.Fatal(err)
	}
	if parent.iframeIsSameOrigin(frame) {
		t.Fatal("one relaxed document was treated as same-origin-domain")
	}
	delete(parent.iframeChildRuntimes, frame)
	parent.iframeLocations[frame] = parent.opts.URL
	if parent.iframeIsSameOrigin(frame) {
		t.Fatal("a child with an unrelaxed origin was exposed before its Runtime was created")
	}
	parent.iframeLocations[frame] = child.opts.URL
	parent.iframeChildRuntimes[frame] = child
	if err := child.Run(`document.domain = "example.test"`, "child-domain.js"); err != nil {
		t.Fatal(err)
	}
	if !parent.iframeIsSameOrigin(frame) {
		t.Fatal("both documents relaxing to the same domain were not same-origin-domain")
	}

	child.setLocation("http://frame.example.test:9443/")
	if parent.iframeIsSameOrigin(frame) {
		t.Fatal("different schemes were treated as same-origin-domain")
	}
}

func TestCrossOriginWindowProxyExposesOnlySafelistedProperties(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	parentDoc, err := dom.ParseBounded(`<!doctype html><html><body><iframe id="child"></iframe></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := New(Options{URL: "https://parent.example.test/", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	childDoc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	child, err := New(Options{URL: "https://child.other.test/", DOM: childDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	frame := parentDoc.ElementByID("child")
	parent.iframeLocations = map[*dom.Node]string{frame: child.opts.URL}
	parent.iframeChildRuntimes = map[*dom.Node]*Runtime{frame: child}
	baseline := make(map[string]goja.Value)
	for _, key := range child.vm.GlobalObject().Keys() {
		baseline[key] = child.vm.GlobalObject().Get(key)
	}
	parent.iframeGlobalBaseline = map[*dom.Node]map[string]goja.Value{frame: baseline}
	_ = parent.vm.Set("remote", parent.iframeWindow(frame))
	for expression, want := range map[string]string{
		`(function(){try{remote.document;return 'accessible'}catch(e){return e.name}})()`:                                                                                       "SecurityError",
		`(function(){try{remote.secret=1;return 'writable'}catch(e){return e.name}})()`:                                                                                         "SecurityError",
		`(function(){try{remote.location.pathname;return 'accessible'}catch(e){return e.name}})()`:                                                                              "SecurityError",
		`(function(){try{remote.location.href;return 'readable'}catch(e){return e.name}})()`:                                                                                    "SecurityError",
		`(function(){try{return 'document' in remote?'visible':'hidden'}catch(e){return e.name}})()`:                                                                            "SecurityError",
		`(function(){try{return remote['Symbol.toStringTag']}catch(e){return e.name}})()`:                                                                                       "SecurityError",
		`String(remote[Symbol.toStringTag]===undefined)`:                                                                                                                        "true",
		`(function(){try{return remote[Symbol.for('hidden')]}catch(e){return e.name}})()`:                                                                                       "SecurityError",
		`(function(){try{Object.getOwnPropertyDescriptor(remote,'document');return 'visible'}catch(e){return e.name}})()`:                                                       "SecurityError",
		`(function(){try{return Reflect.ownKeys(remote).filter(k=>typeof k==='string').sort().join(',')}catch(e){return e.name}})()`:                                            "blur,close,closed,focus,frames,length,location,opener,parent,postMessage,self,then,top,window",
		`(function(){try{return Object.getOwnPropertyDescriptor(remote,'window').get()===remote?'safe':'wrong'}catch(e){return e.name}})()`:                                     "safe",
		`(function(){try{return Object.getOwnPropertyNames(remote.location).sort().join(',')}catch(e){return e.name}})()`:                                                       "href,replace,then",
		`(function(){try{let d=Object.getOwnPropertyDescriptor(remote.location,'href');return String(d.get===undefined&&typeof d.set==='function')}catch(e){return e.name}})()`: "true",
		`(function(){try{return Reflect.ownKeys(remote.location).filter(k=>typeof k==='string').join(',')}catch(e){return e.name}})()`:                                          "href,replace,then",
		`String('then' in remote.location&&remote.location.then===undefined)`:                                                                                                   "true",
		`String(remote.location[Symbol.toStringTag]===undefined)`:                                                                                                               "true",
		`String(Object.getOwnPropertyDescriptor(remote.location,Symbol.toStringTag).value===undefined)`:                                                                         "true",
		`(function(){try{return remote.location[Symbol.for('hidden')]}catch(e){return e.name}})()`:                                                                              "SecurityError",
		`(function(){try{return 'pathname' in remote.location?'visible':'hidden'}catch(e){return e.name}})()`:                                                                   "SecurityError",
		`(function(){try{return String(Object.getPrototypeOf(remote.location)===null)}catch(e){return e.name}})()`:                                                              "true",
		`String(remote.window===remote&&remote.self===remote&&remote.frames===remote&&typeof remote.postMessage==='function')`:                                                  "true",
		`String(remote.opener===null)`:                                    "true",
		`String(Object.getOwnPropertyNames(remote).includes('document'))`: "false",
	} {
		got, evalErr := parent.Evaluate(expression)
		if evalErr != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", expression, got, evalErr, want)
		}
	}
}

func TestCrossOriginWindowProxyIndexedChildAccess(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	parentDoc, err := dom.ParseBounded(`<!doctype html><html><body><iframe id="child"></iframe></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := New(Options{URL: "https://parent.example.test/", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	childDoc, err := dom.ParseBounded(`<!doctype html><html><body><iframe id="nested"></iframe></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	child, err := New(Options{URL: "https://child.other.test/", DOM: childDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	frame := parentDoc.ElementByID("child")
	parent.iframeLocations = map[*dom.Node]string{frame: child.opts.URL}
	parent.iframeChildRuntimes = map[*dom.Node]*Runtime{frame: child}
	baseline := make(map[string]goja.Value)
	for _, key := range child.vm.GlobalObject().Keys() {
		baseline[key] = child.vm.GlobalObject().Get(key)
	}
	parent.iframeGlobalBaseline = map[*dom.Node]map[string]goja.Value{frame: baseline}
	_ = parent.vm.Set("remote", parent.iframeWindow(frame))
	for expression, want := range map[string]string{
		`String(remote.length)`:                                          "1",
		`String(remote[0]===remote.frames[0])`:                           "true",
		`String(Object.getOwnPropertyDescriptor(remote,'0').enumerable)`: "true",
		`String(Object.keys(remote).join(','))`:                          "0",
	} {
		got, evalErr := parent.Evaluate(expression)
		if evalErr != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", expression, got, evalErr, want)
		}
	}
}

func TestImportChildValuePreservesEnumerableAccessorsWithoutInvokingThem(t *testing.T) {
	parent, err := New(Options{URL: "https://parent.example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	child, err := New(Options{URL: "https://child.other.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if err := child.Run(`globalThis.__getterCalls = 0; globalThis.__accessorObject = {}; Object.defineProperty(__accessorObject, 'value', { enumerable: true, get() { __getterCalls++; return 42; } });`, "accessor-bridge.js"); err != nil {
		t.Fatal(err)
	}
	imported := parent.importChildValue(child, child.vm.Get("__accessorObject"), make(map[*goja.Object]*goja.Object), 0).ToObject(parent.vm)
	if got := child.vm.Get("__getterCalls").ToInteger(); got != 0 {
		t.Fatalf("copy invoked source getter %d times, want 0", got)
	}
	if got := imported.Get("value").ToInteger(); got != 42 {
		t.Fatalf("imported getter returned %d, want 42", got)
	}
	if got := child.vm.Get("__getterCalls").ToInteger(); got != 1 {
		t.Fatalf("source getter called %d times after read, want exactly once", got)
	}
}

func TestBroadcastChannelDispatchAcceptsParentRealmEventInSameOriginChild(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	parentDoc, err := dom.ParseBounded(`<!doctype html><html><body><iframe id="child"></iframe></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := New(Options{URL: "https://same.example.test/parent", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	childDoc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	child, err := New(Options{URL: "https://same.example.test/child", DOM: childDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	frame := parentDoc.ElementByID("child")
	if frame == nil {
		t.Fatal("iframe not found")
	}
	parent.iframeLocations = map[*dom.Node]string{frame: child.opts.URL}
	parent.iframeChildRuntimes = map[*dom.Node]*Runtime{frame: child}
	child.iframeParent = parent
	child.iframeParentFrame = frame
	child.iframeParentObjects = make(map[*goja.Object]goja.Value)
	baseline := make(map[string]goja.Value)
	for _, key := range child.vm.GlobalObject().Keys() {
		baseline[key] = child.vm.GlobalObject().Get(key)
	}
	parent.iframeGlobalBaseline = map[*dom.Node]map[string]goja.Value{frame: baseline}
	if err := child.Run(`
		globalThis.channel = new BroadcastChannel('cross-realm-event');
		globalThis.receivedParentEvent = '';
		channel.addEventListener('cross-realm', function(event) {
			receivedParentEvent = event.type + ':' + event.data;
		});
	`, "child-channel.js"); err != nil {
		t.Fatal(err)
	}
	if err := parent.vm.Set("remote", parent.iframeWindow(frame)); err != nil {
		t.Fatal(err)
	}
	if err := parent.Run(`
		globalThis.foreignEvent = new MessageEvent('cross-realm', {data: 'parent'});
		remote.channel.dispatchEvent(foreignEvent);
	`, "parent-dispatch.js"); err != nil {
		t.Fatal(err)
	}
	if got := child.vm.Get("receivedParentEvent").String(); got != "cross-realm:parent" {
		t.Fatalf("child received parent-realm Event as %q, want %q", got, "cross-realm:parent")
	}
}

func TestCustomElementImportNodeUsesTargetRealmRegistry(t *testing.T) {
	doc, err := dom.ParseBounded(`<!doctype html><html><head></head><body></body></html>`, dom.ParseLimits{
		Nodes: 128, Depth: 32, Attributes: 32, AttributeBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Options{URL: "https://elements.example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		const frame = document.createElement('iframe');
		document.body.appendChild(frame);
		const targetWindow = frame.contentWindow;
		const targetDocument = frame.contentDocument;
		class SourceDiv extends HTMLDivElement {}
		class TargetDiv extends targetWindow.HTMLDivElement {}
		customElements.define('x-import-cross-realm', SourceDiv, {extends: 'div'});
		targetWindow.customElements.define('x-import-cross-realm', TargetDiv, {extends: 'div'});
		const source = document.createElement('div', {is: 'x-import-cross-realm'});
		const imported = targetDocument.importNode(source);
		globalThis.importNodeRealmResult = [
			imported instanceof TargetDiv,
			imported !== source,
			imported.getAttribute('is') === 'x-import-cross-realm',
			targetDocument === frame.contentDocument,
			targetDocument === targetWindow.document,
			imported.ownerDocument === targetDocument
		].join('|');
	`, "import-node-cross-realm.js"); err != nil {
		t.Fatal(err)
	}
	if got := r.vm.Get("importNodeRealmResult").String(); got != "true|true|true|true|true|true" {
		t.Fatalf("cross-realm importNode result = %q, want target-registry upgraded clone with stable Document identity", got)
	}
}

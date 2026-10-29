package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestClipboardExists(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		if (typeof navigator.clipboard !== 'object') {
			throw new Error('navigator.clipboard should be an object');
		}
		if (typeof navigator.clipboard.writeText !== 'function') {
			throw new Error('navigator.clipboard.writeText should be a function');
		}
		if (typeof navigator.clipboard.readText !== 'function') {
			throw new Error('navigator.clipboard.readText should be a function');
		}
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestClipboardWriteTextReturnsPromise(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var p = navigator.clipboard.writeText('hello');
		if (typeof p !== 'object' || typeof p.then !== 'function') {
			throw new Error('writeText should return a Promise');
		}
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestClipboardReadTextReturnsPromise(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var p = navigator.clipboard.readText();
		if (typeof p !== 'object' || typeof p.then !== 'function') {
			throw new Error('readText should return a Promise');
		}
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestClipboardWriteThenRead(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var readResult = null;
		navigator.clipboard.writeText('hello world').then(function() {
			return navigator.clipboard.readText();
		}).then(function(text) {
			readResult = text;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("readResult")
	if !ok {
		t.Fatal("readResult not found in global scope")
	}
	if val.Str != "hello world" {
		t.Errorf("readResult = %q, want %q", val.Str, "hello world")
	}
}

func TestClipboardOverwrite(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var readResult = null;
		navigator.clipboard.writeText('first').then(function() {
			return navigator.clipboard.writeText('second');
		}).then(function() {
			return navigator.clipboard.readText();
		}).then(function(text) {
			readResult = text;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("readResult")
	if !ok {
		t.Fatal("readResult not found in global scope")
	}
	if val.Str != "second" {
		t.Errorf("readResult = %q, want %q", val.Str, "second")
	}
}

func TestClipboardReadEmpty(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var readResult = null;
		navigator.clipboard.readText().then(function(text) {
			readResult = text;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("readResult")
	if !ok {
		t.Fatal("readResult not found in global scope")
	}
	if val.Str != "" {
		t.Errorf("readResult = %q, want empty string", val.Str)
	}
}

func TestClipboardWriteEmptyString(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var readResult = null;
		navigator.clipboard.writeText('something').then(function() {
			return navigator.clipboard.writeText('');
		}).then(function() {
			return navigator.clipboard.readText();
		}).then(function(text) {
			readResult = text;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("readResult")
	if !ok {
		t.Fatal("readResult not found in global scope")
	}
	if val.Str != "" {
		t.Errorf("readResult = %q, want empty string", val.Str)
	}
}

type fakeClipboard struct{ text string }

func (c *fakeClipboard) Read() string      { return c.text }
func (c *fakeClipboard) Write(text string) { c.text = text }

func TestPlatformClipboardRequiresSecureContextAndTransientActivation(t *testing.T) {
	provider := &fakeClipboard{text: "initial"}
	r, err := js.New(js.Options{
		Timeout:           5 * time.Second,
		URL:               "https://example.test/",
		ClipboardProvider: provider,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var rejectedName = '';
		var readRejectedName = '';
		navigator.clipboard.writeText('blocked').catch(function(error) { rejectedName = error.name; });
		navigator.clipboard.readText().catch(function(error) { readRejectedName = error.name; });
	`, "test.js"); err != nil {
		t.Fatalf("Run without activation: %v", err)
	}
	if got, _ := r.Global("rejectedName"); got.Str != "NotAllowedError" {
		t.Fatalf("rejected error name = %q, want NotAllowedError", got.Str)
	}
	if got, _ := r.Global("readRejectedName"); got.Str != "NotAllowedError" {
		t.Fatalf("read rejection name = %q, want NotAllowedError", got.Str)
	}
	if provider.text != "initial" {
		t.Fatalf("clipboard changed without activation: %q", provider.text)
	}

	r.SetTransientUserActivation(true)
	if err := r.Run(`
		var writeError = '';
		var readValue = '';
		navigator.clipboard.writeText('approved').catch(function(error) { writeError = error.name; });
		navigator.clipboard.readText().then(function(text) { readValue = text; });
	`, "test.js"); err != nil {
		t.Fatalf("Run with activation: %v", err)
	}
	if got, _ := r.Global("writeError"); got.Str != "" {
		t.Fatalf("activated write error = %q", got.Str)
	}
	if got, _ := r.Global("readValue"); got.Str != "approved" {
		t.Fatalf("activated read value = %q, want approved", got.Str)
	}
	if provider.text != "approved" {
		t.Fatalf("clipboard text = %q, want approved", provider.text)
	}

	insecure := &fakeClipboard{text: "unchanged"}
	insecureRT, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "http://example.test/", ClipboardProvider: insecure})
	if err != nil {
		t.Fatalf("New insecure runtime: %v", err)
	}
	defer insecureRT.Close()
	insecureRT.SetTransientUserActivation(true)
	if err := insecureRT.Run(`
		var insecureError = '';
		var insecureReadError = '';
		navigator.clipboard.writeText('blocked').catch(function(error) { insecureError = error.name; });
		navigator.clipboard.readText().catch(function(error) { insecureReadError = error.name; });
	`, "test.js"); err != nil {
		t.Fatalf("Run in insecure context: %v", err)
	}
	if got, _ := insecureRT.Global("insecureError"); got.Str != "NotAllowedError" {
		t.Fatalf("insecure error name = %q, want NotAllowedError", got.Str)
	}
	if got, _ := insecureRT.Global("insecureReadError"); got.Str != "NotAllowedError" {
		t.Fatalf("insecure read error name = %q, want NotAllowedError", got.Str)
	}
	if insecure.text != "unchanged" {
		t.Fatalf("insecure clipboard changed: %q", insecure.text)
	}
}

func TestClipboardPermissionsQueryAndLiveState(t *testing.T) {
	states := map[string]string{"clipboard-read": "prompt", "clipboard-write": "prompt"}
	r, err := js.New(js.Options{
		Timeout:           5 * time.Second,
		URL:               "https://example.test/",
		ClipboardProvider: &fakeClipboard{},
		ClipboardPermissionState: func(_, name string) string {
			return states[name]
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var readStatus = null;
		var writeStatus = null;
		var unsupportedPermission = '';
		navigator.permissions.query({name: 'clipboard-read'}).then(function(status) { readStatus = status; });
		navigator.permissions.query({name: 'clipboard-write'}).then(function(status) { writeStatus = status; });
		navigator.permissions.query({name: 'geolocation'}).catch(function(error) { unsupportedPermission = error.name; });
	`, "clipboard-permissions.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, err := r.Evaluate(`readStatus.name + '|' + readStatus.state + '|' + writeStatus.name + '|' + writeStatus.state`); err != nil || got != "clipboard-read|prompt|clipboard-write|prompt" {
		t.Fatalf("permission query = %q, err %v", got, err)
	}
	if got, _ := r.Global("unsupportedPermission"); got.Str != "NotSupportedError" {
		t.Fatalf("unsupported permission rejection = %q, want NotSupportedError", got.Str)
	}
	states["clipboard-read"], states["clipboard-write"] = "denied", "granted"
	if got, err := r.Evaluate(`readStatus.state + '|' + writeStatus.state`); err != nil || got != "denied|granted" {
		t.Fatalf("live PermissionStatus states = %q, err %v", got, err)
	}
	r.SetTransientUserActivation(true)
	if err := r.Run(`
		var blockedClipboardRead = '';
		navigator.clipboard.readText().catch(function(error) { blockedClipboardRead = error.name; });
	`, "clipboard-permission-denial.js"); err != nil {
		t.Fatalf("Run denied clipboard read: %v", err)
	}
	if got, _ := r.Global("blockedClipboardRead"); got.Str != "NotAllowedError" {
		t.Fatalf("blocked clipboard read error = %q, want NotAllowedError", got.Str)
	}

	fallback, err := js.New(js.Options{
		Timeout:                  5 * time.Second,
		ClipboardPermissionState: func(string, string) string { return "denied" },
	})
	if err != nil {
		t.Fatalf("New fallback runtime: %v", err)
	}
	defer fallback.Close()
	if err := fallback.Run(`
		var fallbackDenied = '';
		navigator.clipboard.writeText('blocked').catch(function(error) { fallbackDenied = error.name; });
	`, "clipboard-fallback-permission.js"); err != nil {
		t.Fatalf("Run blocked memory clipboard: %v", err)
	}
	if got, _ := fallback.Global("fallbackDenied"); got.Str != "NotAllowedError" {
		t.Fatalf("blocked memory clipboard error = %q, want NotAllowedError", got.Str)
	}
}

func TestClipboardPermissionStateUsesEachDocumentOrigin(t *testing.T) {
	states := map[string]string{
		"https://top.example.test/page":   "granted",
		"https://frame.example.test/page": "denied",
	}
	newRuntime := func(documentURL string) *js.Runtime {
		r, err := js.New(js.Options{
			Timeout:           5 * time.Second,
			URL:               documentURL,
			ClipboardProvider: &fakeClipboard{},
			ClipboardPermissionState: func(url, name string) string {
				if name != "clipboard-read" {
					return "prompt"
				}
				return states[url]
			},
		})
		if err != nil {
			t.Fatalf("New(%s): %v", documentURL, err)
		}
		return r
	}
	top := newRuntime("https://top.example.test/page")
	defer top.Close()
	child := newRuntime("https://frame.example.test/page")
	defer child.Close()
	for _, test := range []struct {
		runtime *js.Runtime
		url     string
		want    string
	}{
		{runtime: top, url: "https://top.example.test/page", want: "granted"},
		{runtime: child, url: "https://frame.example.test/page", want: "denied"},
	} {
		if err := test.runtime.Run(`
			var permissionState = '';
			navigator.permissions.query({name: 'clipboard-read'}).then(function(status) { permissionState = status.state; });
		`, "clipboard-origin-permission.js"); err != nil {
			t.Fatalf("query clipboard permission: %v", err)
		}
		if got, _ := test.runtime.Global("permissionState"); got.Str != test.want {
			t.Fatalf("clipboard permission for %s = %q, want %s", test.url, got.Str, test.want)
		}
	}
}

func TestClipboardItemReadWriteTextAndHTML(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var clipboardResult = null;
		var clipboardCallbackError = '';
		var clipboardStage = 'start';
		var source = new ClipboardItem({
			'text/plain': new Blob(['hello'], {type: 'text/plain'}),
			'text/html': new Blob(['<b>hello</b>'], {type: 'text/html'})
		});
		var syncState = JSON.stringify({isItem: source instanceof ClipboardItem, types: source.types,
			style: source.presentationStyle, supportsHTML: ClipboardItem.supports('text/html'),
			supportsPNG: ClipboardItem.supports('image/png')});
		navigator.clipboard.write([source]).then(function() {
			clipboardStage = 'written';
			return navigator.clipboard.read();
		}).then(function(items) {
			clipboardStage = 'read';
			return items[0].getType('text/plain').then(function(plainBlob) {
				clipboardStage = 'plain-blob';
				return plainBlob.text().then(function(plain) {
					clipboardStage = 'plain-text';
					return items[0].getType('text/html').then(function(htmlBlob) {
						clipboardStage = 'html-blob';
						return htmlBlob.text().then(function(html) {
							clipboardStage = 'html-text';
							clipboardResult = plain + '|' + html;
						});
					});
				});
			});
		}).catch(function(error) { clipboardCallbackError = clipboardStage + ':' + error.name + ':' + error.message; });
	`, "clipboard-item.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, ok := r.Global("clipboardResult")
	if !ok || got.Str != `hello|<b>hello</b>` {
		sync, _ := r.Global("syncState")
		callbackErr, _ := r.Global("clipboardCallbackError")
		t.Fatalf("clipboard item result = %q (present %v), sync=%q callbackError=%q", got.Str, ok, sync.Str, callbackErr.Str)
	}
	sync, _ := r.Global("syncState")
	if want := `{"isItem":true,"types":["text/plain","text/html"],"style":"unspecified","supportsHTML":true,"supportsPNG":false}`; sync.Str != want {
		t.Fatalf("ClipboardItem surface = %q, want %q", sync.Str, want)
	}
}

func TestClipboardItemRejectsUnsupportedRepresentationAndType(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var writeError = '';
		var missingTypeError = '';
		var item = new ClipboardItem({'text/plain': new Blob(['ok'], {type: 'text/plain'})});
		navigator.clipboard.write([new ClipboardItem({'application/json': new Blob(['{}'], {type: 'application/json'})})])
			.catch(function(error) { writeError = error.name; });
		item.getType('text/html').catch(function(error) { missingTypeError = error.name; });
	`, "clipboard-item-errors.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, _ := r.Global("writeError"); got.Str != "NotSupportedError" {
		t.Fatalf("unsupported write error = %q, want NotSupportedError", got.Str)
	}
	if got, _ := r.Global("missingTypeError"); got.Str != "NotFoundError" {
		t.Fatalf("missing type error = %q, want NotFoundError", got.Str)
	}
}

func TestClipboardItemPromiseRepresentationsResolveForReadAndWrite(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var resolvedResult = '';
		var invalidError = '';
		var promised = new ClipboardItem({
			'text/plain': Promise.resolve(new Blob(['from promise'], {type: 'text/plain'})),
			'text/html': Promise.resolve(new Blob(['<i>from promise</i>'], {type: 'text/html'}))
		});
		navigator.clipboard.write([promised]).then(function() {
			return navigator.clipboard.read();
		}).then(function(items) {
			return items[0].getType('text/html').then(function(blob) { return blob.text(); });
		}).then(function(html) {
			return navigator.clipboard.readText().then(function(text) {
				resolvedResult = text + '|' + html;
			});
		});
		var invalid = new ClipboardItem({'text/plain': Promise.resolve('not a Blob')});
		navigator.clipboard.write([invalid]).catch(function(error) { invalidError = error.name; });
	`, "clipboard-item-promise.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, _ := r.Global("resolvedResult"); got.Str != "from promise|<i>from promise</i>" {
		t.Fatalf("promised representation round trip = %q", got.Str)
	}
	if got, _ := r.Global("invalidError"); got.Str != "TypeError" {
		t.Fatalf("invalid promised representation error = %q, want TypeError", got.Str)
	}
}

func TestClipboardItemAcceptsFileAsBlobRepresentation(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var fileResult = '';
		var fileItem = new ClipboardItem({'text/plain': new File(['file text'], 'note.txt', {type: 'text/plain'})});
		navigator.clipboard.write([fileItem]).then(function() {
			return navigator.clipboard.readText();
		}).then(function(text) { fileResult = text; });
	`, "clipboard-item-file.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, _ := r.Global("fileResult"); got.Str != "file text" {
		t.Fatalf("File Blob representation result = %q, want file text", got.Str)
	}
}

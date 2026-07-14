//go:build darwin && cgo

package backend

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework WebKit -framework Cocoa -framework Foundation

#include <stdlib.h>
#include <stdbool.h>

typedef void* WebViewRef;

// --- Lifecycle ---
WebViewRef wkwebview_create(int width, int height);
void wkwebview_destroy(WebViewRef wv);

// --- Navigation ---
void wkwebview_navigate(WebViewRef wv, const char* url);
void wkwebview_stop(WebViewRef wv);
void wkwebview_reload(WebViewRef wv);
void wkwebview_go_back(WebViewRef wv);
void wkwebview_go_forward(WebViewRef wv);
bool wkwebview_can_go_back(WebViewRef wv);
bool wkwebview_can_go_forward(WebViewRef wv);

// --- Content ---
void wkwebview_load_html(WebViewRef wv, const char* html, const char* base_url);
void wkwebview_evaluate_js(WebViewRef wv, const char* script, int ctx_id);

// --- UI ---
void wkwebview_set_private(WebViewRef wv, bool private);
void wkwebview_show_devtools(WebViewRef wv);

// --- Thread pump (blocks — call on locked thread) ---
void wkwebview_pump_events(WebViewRef wv);
*/
import "C"
import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// jsEvalID tracks pending EvaluateJS calls. Using an integer ID instead of
// passing Go pointers to C avoids cgo pointer-passing rules violations.
var (
	jsEvalMu      sync.Mutex
	jsEvalResults = make(map[int]chan string)
	jsEvalNextID  int
)

// ---------------------------------------------------------------------------
// WKWebView Backend
// ---------------------------------------------------------------------------

type wkBackend struct {
	wv      C.WebViewRef
	private bool
	cb      Callbacks
	mu      sync.Mutex
}

var _ Backend = (*wkBackend)(nil)

// New creates a macOS WKWebView-backed Backend.
//
// Spawns a dedicated OS-thread-locked goroutine for the Cocoa event loop
// (required by WebKit). Must not be called from an init function.
func New() Backend {
	type result struct {
		wv  C.WebViewRef
		err error
	}
	resCh := make(chan result, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		wv := C.wkwebview_create(1024, 768)
		if wv == nil {
			resCh <- result{err: fmt.Errorf("wkwebview_create returned nil")}
			return
		}
		resCh <- result{wv: wv}
		C.wkwebview_pump_events(wv)
	}()

	res := <-resCh
	if res.err != nil {
		return NewDefaultBackend()
	}
	return &wkBackend{wv: res.wv}
}

// --- cgo exports — called from ObjC via _cgo_export.h --------------------

//export _go_nav_callback
func _go_nav_callback(event C.int, url *C.char) {
	cb := loadCallbacks()
	if cb.OnNavigation != nil {
		cb.OnNavigation(NavEvent(event), C.GoString(url))
	}
}

//export _go_title_callback
func _go_title_callback(title *C.char) {
	cb := loadCallbacks()
	if cb.OnTitleChanged != nil {
		cb.OnTitleChanged(C.GoString(title))
	}
}

//export _go_url_callback
func _go_url_callback(url *C.char) {
	cb := loadCallbacks()
	if cb.OnURLChanged != nil {
		cb.OnURLChanged(C.GoString(url))
	}
}

//export _go_loading_callback
func _go_loading_callback(loading C.bool) {
	cb := loadCallbacks()
	if cb.OnLoadingChanged != nil {
		cb.OnLoadingChanged(bool(loading))
	}
}

//export _go_js_callback
func _go_js_callback(result *C.char, ctx_id C.int) {
	jsEvalMu.Lock()
	ch, ok := jsEvalResults[int(ctx_id)]
	if ok {
		delete(jsEvalResults, int(ctx_id))
	}
	jsEvalMu.Unlock()
	if ok {
		ch <- C.GoString(result)
	}
}

// ---- Global callbacks (single-backend prototype) --------------------------

var (
	globalCB   Callbacks
	globalCBMu sync.RWMutex
)

func storeCallbacks(cb Callbacks) {
	globalCBMu.Lock()
	globalCB = cb
	globalCBMu.Unlock()
}

func loadCallbacks() Callbacks {
	globalCBMu.RLock()
	defer globalCBMu.RUnlock()
	return globalCB
}

// --- Backend interface -----------------------------------------------------

func (b *wkBackend) Navigate(url string) error {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	C.wkwebview_navigate(b.wv, cURL)
	return nil
}

func (b *wkBackend) Stop() error {
	C.wkwebview_stop(b.wv)
	return nil
}

func (b *wkBackend) Reload() error {
	C.wkwebview_reload(b.wv)
	return nil
}

func (b *wkBackend) GoBack() error {
	C.wkwebview_go_back(b.wv)
	return nil
}

func (b *wkBackend) GoForward() error {
	C.wkwebview_go_forward(b.wv)
	return nil
}

func (b *wkBackend) CanGoBack() bool {
	return bool(C.wkwebview_can_go_back(b.wv))
}

func (b *wkBackend) CanGoForward() bool {
	return bool(C.wkwebview_can_go_forward(b.wv))
}

func (b *wkBackend) LoadHTML(html string, baseURL string) error {
	cHTML := C.CString(html)
	cBase := C.CString(baseURL)
	defer C.free(unsafe.Pointer(cHTML))
	defer C.free(unsafe.Pointer(cBase))
	C.wkwebview_load_html(b.wv, cHTML, cBase)
	return nil
}

func (b *wkBackend) EvaluateJS(script string) (string, error) {
	resultCh := make(chan string, 1)

	jsEvalMu.Lock()
	id := jsEvalNextID
	jsEvalNextID++
	jsEvalResults[id] = resultCh
	jsEvalMu.Unlock()

	cScript := C.CString(script)
	defer C.free(unsafe.Pointer(cScript))
	C.wkwebview_evaluate_js(b.wv, cScript, C.int(id))

	result := <-resultCh
	return result, nil
}

func (b *wkBackend) SetPrivateMode(v bool) {
	b.mu.Lock()
	b.private = v
	b.mu.Unlock()
	C.wkwebview_set_private(b.wv, C.bool(v))
}

func (b *wkBackend) IsPrivateMode() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.private
}

func (b *wkBackend) ShowDevTools() error {
	C.wkwebview_show_devtools(b.wv)
	return nil
}

func (b *wkBackend) DevToolsURL() string {
	return ""
}

func (b *wkBackend) Close() error {
	C.wkwebview_destroy(b.wv)
	b.wv = nil
	return nil
}

func (b *wkBackend) SetCallbacks(cb Callbacks) {
	b.mu.Lock()
	b.cb = cb
	b.mu.Unlock()
	storeCallbacks(cb)
}

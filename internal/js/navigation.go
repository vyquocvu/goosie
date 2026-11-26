package js

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type NavigationKind uint8

const (
	NavigationAssign NavigationKind = iota
	NavigationReplace
	NavigationReload
	NavigationHistoryPush
	NavigationHistoryReplace
	NavigationHistoryTraverse
	NavigationOpenWindow
	NavigationCloseWindow
	NavigationFocusWindow
)

type NavigationRequest struct {
	Kind         NavigationKind
	URL          string
	Delta        int
	State        []byte
	Target       string
	Features     string
	NoOpener     bool
	TargetHandle WindowHandle
}

func (r *Runtime) SetNavigationSink(sink func(NavigationRequest)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.navigationSink = sink
	r.mu.Unlock()
}

func (r *Runtime) locationAssign(call goja.FunctionCall) goja.Value {
	r.navigateLocation(NavigationAssign, r.navigationArgument(call, "assign"))
	return goja.Undefined()
}

func (r *Runtime) locationReplace(call goja.FunctionCall) goja.Value {
	r.navigateLocation(NavigationReplace, r.navigationArgument(call, "replace"))
	return goja.Undefined()
}

func (r *Runtime) locationReload(call goja.FunctionCall) goja.Value {
	r.navigateLocation(NavigationReload, "")
	return goja.Undefined()
}

func (r *Runtime) navigationArgument(call goja.FunctionCall, method string) string {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute '" + method + "' on 'Location': 1 argument required, but only 0 present."))
	}
	value := call.Argument(0)
	if _, ok := value.(*goja.Symbol); ok {
		panic(r.vm.NewTypeError("Cannot convert a Symbol value to a string"))
	}
	return value.String()
}

func (r *Runtime) navigateLocation(kind NavigationKind, reference string) {
	raw, base := r.locationState()
	target := raw
	if kind != NavigationReload {
		var ok bool
		target, ok = resolveLocationURL(base, reference)
		if !ok {
			panic(r.navigationError("SyntaxError", "Failed to execute '"+navigationMethod(kind)+"' on 'Location': '"+reference+"' is not a valid URL."))
		}
	}
	if r.iframeParent != nil && r.iframeParentFrame != nil {
		r.setLocation(target)
		r.iframeParent.navigateIFrameLocationKind(r.iframeParentFrame, kind, target)
		return
	}
	if kind != NavigationReload {
		r.activateLinkFragment(target)
	}
	r.setLocation(target)
	r.enqueueNavigation(NavigationRequest{Kind: kind, URL: target})
}

func navigationMethod(kind NavigationKind) string {
	if kind == NavigationReplace {
		return "replace"
	}
	return "assign"
}

func resolveLocationURL(base *url.URL, reference string) (string, bool) {
	ref, err := url.Parse(strings.TrimSpace(reference))
	if err != nil {
		return "", false
	}
	if base != nil && base.IsAbs() {
		ref = base.ResolveReference(ref)
	}
	if !validNavigationURL(ref) {
		return "", false
	}
	return ref.String(), true
}

func validNavigationURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
		if u.Hostname() == "" {
			return false
		}
		if port := u.Port(); port != "" {
			value, err := strconv.Atoi(port)
			if err != nil || value < 0 || value > 65535 {
				return false
			}
		}
	}
	return true
}

func (r *Runtime) navigationError(name, message string) *goja.Object {
	if constructor, ok := goja.AssertConstructor(r.vm.Get(name)); ok {
		if exception, err := constructor(nil, r.vm.ToValue(message)); err == nil {
			return exception
		}
	}
	object := r.vm.NewObject()
	_ = object.Set("name", name)
	_ = object.Set("message", message)
	return object
}

func (r *Runtime) iframeLocationNavigate(frame *dom.Node, kind NavigationKind, call goja.FunctionCall) goja.Value {
	target := r.iframeLocations[frame]
	if target == "" {
		target = "about:blank"
	}
	if kind != NavigationReload {
		reference := r.navigationArgument(call, navigationMethod(kind))
		base := r.documentURL()
		if current := r.iframeLocations[frame]; current != "" && current != "about:blank" && current != "about:srcdoc" {
			if parsed, err := url.Parse(current); err == nil {
				base = parsed
			}
		}
		if base == nil {
			if parsed, err := url.Parse(target); err == nil {
				base = parsed
			}
		}
		var ok bool
		target, ok = resolveLocationURL(base, reference)
		if !ok {
			panic(r.navigationError("SyntaxError", "Failed to execute '"+navigationMethod(kind)+"' on 'Location': '"+reference+"' is not a valid URL."))
		}
	}
	r.navigateIFrameLocationKind(frame, kind, target)
	return goja.Undefined()
}

func (r *Runtime) enqueueNavigation(request NavigationRequest) {
	r.mu.Lock()
	sink := r.navigationSink
	closed := r.closed
	r.mu.Unlock()
	if closed || sink == nil {
		return
	}
	sink(request)
}

func (r *Runtime) navigationSinkAvailable() bool {
	r.mu.Lock()
	sink := r.navigationSink
	closed := r.closed
	r.mu.Unlock()
	return !closed && sink != nil
}

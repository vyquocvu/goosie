package js

import (
	"net/url"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) locationForDocument(doc *dom.Document) goja.Value {
	if doc == nil {
		return goja.Null()
	}
	if doc == r.domDoc && r.locObj != nil {
		return r.locObj
	}
	if frame := r.iframeFrameForDocument(&doc.Node); frame != nil {
		if window, ok := r.iframeWindow(frame).(*goja.Object); ok {
			return window.Get("location")
		}
	}
	return goja.Null()
}

func (r *Runtime) setupLocationParts(win *goja.Object) {
	r.setLocation(r.opts.URL)
	loc := r.vm.NewObject()
	get := func(name string, pick func(*url.URL, string) string) {
		_ = loc.DefineAccessorProperty(name, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			raw, u := r.locationState()
			return r.vm.ToValue(pick(u, raw))
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	_ = loc.DefineAccessorProperty("href", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		raw, u := r.locationState()
		if u == nil {
			return r.vm.ToValue(raw)
		}
		return r.vm.ToValue(u.String())
	}), r.vm.ToValue(r.locationAssign), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = loc.Set("assign", r.locationAssign)
	_ = loc.Set("replace", r.locationReplace)
	_ = loc.Set("reload", r.locationReload)
	get("protocol", func(u *url.URL, _ string) string {
		if u == nil || u.Scheme == "" {
			return ""
		}
		return u.Scheme + ":"
	})
	get("host", func(u *url.URL, _ string) string {
		if u == nil {
			return ""
		}
		return u.Host
	})
	get("hostname", func(u *url.URL, _ string) string {
		if u == nil {
			return ""
		}
		return u.Hostname()
	})
	get("port", func(u *url.URL, _ string) string {
		if u == nil {
			return ""
		}
		return u.Port()
	})
	get("pathname", func(u *url.URL, _ string) string {
		if u == nil {
			return ""
		}
		if u.Path == "" && u.Host != "" {
			return "/"
		}
		return u.Path
	})
	get("search", func(u *url.URL, _ string) string {
		if u == nil || u.RawQuery == "" {
			return ""
		}
		return "?" + u.RawQuery
	})
	_ = loc.DefineAccessorProperty("hash", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		_, u := r.locationState()
		if u == nil || u.Fragment == "" {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue("#" + u.Fragment)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		fragment := ""
		if value := call.Argument(0); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			fragment = strings.TrimPrefix(value.String(), "#")
		}
		r.activateLinkFragment("#" + fragment)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	get("origin", func(u *url.URL, _ string) string {
		if u == nil || u.Scheme == "" || u.Host == "" {
			return "null"
		}
		return u.Scheme + "://" + u.Host
	})
	_ = loc.Set("toString", func(call goja.FunctionCall) goja.Value {
		raw, u := r.locationState()
		if u == nil {
			return r.vm.ToValue(raw)
		}
		return r.vm.ToValue(u.String())
	})
	r.locObj = loc

	getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value { return r.locObj })
	_ = win.DefineAccessorProperty("location", getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	docLocation := r.docObj()
	_ = docLocation.DefineAccessorProperty("location", getter, r.vm.ToValue(r.locationAssign), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) locationState() (string, *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locRaw, r.locURL
}

func (r *Runtime) setLocation(raw string) {
	parsed, err := url.Parse(raw)
	if err != nil {
		parsed = nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if parsed != nil && !parsed.IsAbs() && r.locURL != nil && r.locURL.IsAbs() {
		if resolved, rerr := r.locURL.Parse(raw); rerr == nil {
			parsed = resolved
			raw = resolved.String()
		}
	}
	r.locRaw = raw
	r.locURL = parsed
}

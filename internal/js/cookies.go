package js

import (
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

type CookieStore interface {
	CookiesForDocument(url string) string
	SetCookieFromDocument(url, assignment string)
}

type memCookieJar struct {
	mu    sync.Mutex
	order []string
	vals  map[string]string
}

func newMemCookieJar() *memCookieJar {
	return &memCookieJar{vals: make(map[string]string)}
}

func (m *memCookieJar) CookiesForDocument(url string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	parts := make([]string, 0, len(m.order))
	for _, k := range m.order {
		if v, ok := m.vals[k]; ok {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

func (m *memCookieJar) SetCookieFromDocument(url, assignment string) {
	name, value, attrs, ok := splitCookiePair(assignment)
	if !ok {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range attrs {
		an, av := splitAttr(a)
		switch strings.ToLower(an) {
		case "expires":
			if t, err := time.Parse(time.RFC1123, strings.Trim(av, "\"")); err == nil && !t.After(time.Now()) {
				delete(m.vals, name)
				return
			}
		case "max-age":
			if n, ok := atoi(av); ok && n <= 0 {
				delete(m.vals, name)
				return
			}
		}
	}
	if _, dup := m.vals[name]; !dup {
		m.order = append(m.order, name)
	}
	m.vals[name] = value
}

func splitCookiePair(assignment string) (name, value string, attrs []string, ok bool) {
	segs := strings.Split(assignment, ";")
	if len(segs) == 0 {
		return "", "", nil, false
	}
	i := strings.IndexByte(segs[0], '=')
	if i <= 0 {
		return "", "", nil, false
	}
	name = strings.TrimSpace(segs[0][:i])
	value = strings.TrimSpace(segs[0][i+1:])
	if name == "" {
		return "", "", nil, false
	}
	return name, value, segs[1:], true
}

func splitAttr(a string) (string, string) {
	if i := strings.IndexByte(a, '='); i >= 0 {
		return strings.TrimSpace(a[:i]), strings.TrimSpace(a[i+1:])
	}
	return strings.TrimSpace(a), ""
}

func atoi(s string) (int, bool) {
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}

func (r *Runtime) setupCookies() {
	docVal := r.vm.Get("document")
	doc, ok := docVal.(*goja.Object)
	if !ok {
		return
	}
	var store CookieStore = newMemCookieJar()
	if r.opts.Cookies != nil {
		store = r.opts.Cookies
	}
	docURL := r.opts.URL
	bridge := r.vm.NewObject()
	_ = bridge.Set("get", func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(store.CookiesForDocument(docURL))
	})
	_ = bridge.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 && call.Arguments[0] != nil {
			store.SetCookieFromDocument(docURL, call.Arguments[0].String())
		}
		return goja.Undefined()
	})
	_ = doc.Set("__cookieStore__", bridge)
	_, _ = r.vm.RunString(`
		Object.defineProperty(document, 'cookie', {
			get: function() {
				return document.__cookieStore__.get();
			},
			set: function(v) {
				document.__cookieStore__.set(String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
}

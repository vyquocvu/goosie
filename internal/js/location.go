package js

import (
	"net/url"

	"github.com/dop251/goja"
)

// window.location, as the parts pages actually read.
//
// Before this file the binding set exactly one property:
//
//	_ = loc.Set("href", r.opts.URL)   // internal/js/js.go:381 at fe4928c
//
// and nothing else, on a page where 33 measured reads ask for more. Counting
// `location.<part>(?![A-Za-z])` in the saved vnexpress home page (201,234-byte
// copy, sha1 1cd00f06…, see docs/tasks/TASK-047-DOM-JQUERY-APIS.md §1.2) the
// inline scripts read
//
//	location.href 7, location.search 6, location.hash 5, location.pathname 4,
//	location.hostname 4, location.origin 3, location.protocol 2, location.host 2
//
// and one of those reads is the error that survived every other fix in this
// task: `Cannot read property 'indexOf' of undefined or null` inside
// `validateCachePage` (offset 66,563 of that copy), whose whole first statement
// is `if (window.location.hostname.indexOf('demo.') !== -1) { return; }` - a
// missing property, not a missing method, so it throws as a TypeError the page
// cannot work around and takes the rest of its boot script with it.
//
// Each part is an accessor over one parsed *url.URL held by the runtime, not a
// snapshot string set at boot, for the same reason the DOM accessors are live:
// history.pushState/replaceState move the document's URL, and a page that
// reads location.pathname afterwards must see the new one. Writing location
// itself is navigation and is not implemented - see assign/replace/reload below.

// setupLocationParts replaces the href-only location stub with the full set of
// URL parts. An unparseable Options.URL still yields an object whose href reads
// back exactly what the host gave, which is the behaviour a `file://` or a
// scheme-less host string needs; the derived parts then read as empty strings
// rather than as a partial parse of a URL Go's parser rejected.
func (r *Runtime) setupLocationParts(win *goja.Object) {
	r.setLocation(r.opts.URL)
	loc := r.vm.NewObject()
	get := func(name string, pick func(*url.URL, string) string) {
		_ = loc.DefineAccessorProperty(name, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			raw, u := r.locationState()
			return r.vm.ToValue(pick(u, raw))
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	get("href", func(u *url.URL, raw string) string {
		if u == nil {
			return raw
		}
		return u.String()
	})
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
		// An absolute URL with no path is normalised to "/" exactly as the URL
		// serializer does: pages concatenate location.pathname with a route
		// without checking, and "" would build "example.testlogin" out of
		// "https://example.test" + "login".
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
	get("hash", func(u *url.URL, _ string) string {
		if u == nil || u.Fragment == "" {
			return ""
		}
		return "#" + u.Fragment
	})
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

	// Two names, one object. Pages reach the location through document as
	// commonly as through window - the saved vietnamnet home page's analytics
	// snippet reads `document.location.protocol`, and with no such property
	// that was the page's first surviving error ("Cannot read property
	// 'protocol' of undefined"). Both accessors return the same object, so
	// `document.location === window.location` holds as it does in a browser.
	//
	// Read-only on purpose: assigning to window.location means navigation, and
	// a plain writable property would let a script silently replace the object
	// - after which every later reader, including the history walk, would see a
	// stale location while the document stayed put.
	getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value { return r.locObj })
	_ = win.DefineAccessorProperty("location", getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.docObj().DefineAccessorProperty("location", getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// locationState returns the document's current URL, raw and parsed. A nil parse
// means the host gave something Go could not parse; callers render the raw
// string for href and empty strings for the derived parts.
func (r *Runtime) locationState() (string, *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locRaw, r.locURL
}

// setLocation moves the document's URL. Navigation is not implemented, so this
// only ever runs for the two things that legitimately change a document's URL
// without loading a new one: the initial binding and history's session-history
// walk.
//
// A relative argument is resolved against the URL the document already has,
// because that is what the standard does with the entry settings object:
// `history.pushState(s, "", "/route?x=1")` leaves the page at
// "https://host/route?x=1", and every part (protocol, host, pathname) has to
// read from the resolved URL for the next route's own checks to work. An
// unparseable value, or a relative one with no absolute base to resolve it
// against, still becomes the href verbatim - matching the previous href-only
// binding rather than silently keeping the old URL.
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

// Deliberately absent from this object, each because a page that calls it would
// be asking Goosie to do something the engine does not do yet:
//
//   - location.assign(), location.replace(), location.reload(): navigation. A
//     call throws a TypeError naming the missing member, which is the loud
//     failure this package's contract asks for; a silent no-op would leave a
//     reload loop-looking script with nothing to show for itself.
//   - location.searchParams: Location implements the URL mixin, so browsers do
//     expose it, but it is a live view of href and neither measured page reads
//     it (0 occurrences). `new URL(href).searchParams` is available already.
//   - location.ancestorOrigins: not read by any measured page.
//   - href/protocol/... setters: browsers make every part writable and a write
//     navigates. Here the accessors are read-only, so a write is ignored rather
//     than navigating: Goosie has no navigation, and pretending to accept the
//     write would leave a page believing it moved when nothing did.

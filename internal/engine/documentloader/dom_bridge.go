package documentloader

import "github.com/vyquocvu/goosie/internal/dom"

// FromDomResource converts a streaming-parser discovery (internal/dom)
// into a coordinator input (this package). It is the single bridge
// between the pure parser and the orchestrator: the parser reports
// facts, the coordinator decides whether and how to fetch.
//
// The function is intentionally a plain mapping; it does not perform
// CSP checks, URL resolution, or any other policy work. Those happen
// inside Coordinator.HandleResource.
//
// The dependency direction is one-way: documentloader imports dom, but
// dom does not import documentloader. This keeps the parser free of
// orchestration concerns and prevents cycles if the parser is later
// reused by other consumers (e.g. the headless renderer or a server
// HTML inspector).
func FromDomResource(r dom.Resource) Resource {
	return Resource{
		Kind:        fromDomKind(r.Kind),
		URL:         r.URL,
		Position:    r.Position,
		ScriptMode:  fromDomScriptMode(r.ScriptMode),
		Inline:      r.Inline,
		Integrity:   r.Integrity,
		CrossOrigin: r.CrossOrigin,
	}
}

// FromDomResources converts a batch in one call. Convenience for
// callers that collected discoveries into a slice before handing them
// to the coordinator.
func FromDomResources(rs []dom.Resource) []Resource {
	if len(rs) == 0 {
		return nil
	}
	out := make([]Resource, len(rs))
	for i, r := range rs {
		out[i] = FromDomResource(r)
	}
	return out
}

// fromDomKind maps internal/dom's ResourceKind to this package's
// ResourceKind. The two enums currently have identical numeric values
// and labels but are kept separate to avoid coupling. If they ever
// drift, this is the only place to update.
func fromDomKind(k dom.ResourceKind) ResourceKind {
	switch k {
	case dom.ResourceCSS:
		return KindCSS
	case dom.ResourceScript:
		return KindScript
	case dom.ResourceImage:
		return KindImage
	default:
		// Unknown parser-side kinds map to KindCSS (the coordinator's
		// zero-discoverable value is reserved for "unset"). This is a
		// defensive default; new parser-side kinds should be added here
		// when introduced.
		return KindCSS
	}
}

// fromDomScriptMode maps internal/dom's ScriptMode to this package's
// ScriptMode. Both use the same numeric values today (Classic=0,
// Async=1, Defer=2, Module=3) but are kept distinct so the two
// packages can evolve independently.
func fromDomScriptMode(m dom.ScriptMode) ScriptMode {
	switch m {
	case dom.ScriptModeClassic:
		return ScriptModeClassic
	case dom.ScriptModeAsync:
		return ScriptModeAsync
	case dom.ScriptModeDefer:
		return ScriptModeDefer
	case dom.ScriptModeModule:
		return ScriptModeModule
	default:
		return ScriptModeClassic
	}
}

// FromDomOnResource returns a closure suitable for passing as
// dom.ParseConfig.OnResource. Each discovery is converted via
// FromDomResource and forwarded to HandleResource on the coordinator.
//
// The returned closure is safe to invoke from the streaming parser
// goroutine; HandleResource is non-blocking.
func (c *Coordinator) FromDomOnResource() func(dom.Resource) {
	return func(r dom.Resource) {
		c.handleResource(FromDomResource(r), true)
	}
}

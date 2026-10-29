package js

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) queueDynamicScript(node *dom.Node) {
	if node == nil || !node.Element() || node.Data != "script" || r.opts.ScriptingDisabled || r.dynamicScriptFetcher == nil {
		return
	}
	src := strings.TrimSpace(node.GetAttribute("src"))
	if src == "" || r.dynamicScriptStarted[node] {
		return
	}
	typ := strings.ToLower(strings.TrimSpace(node.GetAttribute("type")))
	if typ != "" && typ != "text/javascript" && typ != "application/javascript" && typ != "application/ecmascript" && typ != "text/ecmascript" {
		return
	}
	if r.dynamicScriptQueued == nil {
		r.dynamicScriptQueued = make(map[*dom.Node]bool)
	}
	if r.dynamicScriptQueued[node] {
		return
	}
	r.dynamicScriptQueued[node] = true
	r.pendingDynamicScripts = append(r.pendingDynamicScripts, node)
}

// SetDynamicScriptLimit bounds external classic scripts fetched for this
// document. The engine supplies its per-document admission limit.
func (r *Runtime) SetDynamicScriptLimit(limit int) {
	if r == nil {
		return
	}
	if limit < 0 {
		limit = 0
	}
	r.dynamicScriptLimit = limit
}

// tickDynamicScripts performs one dynamically inserted classic script task.
// A fresh queue is used so scripts inserted by the executing script run in a
// later turn, as they do for async classic scripts in browsers.
func (r *Runtime) tickDynamicScripts() bool {
	if len(r.pendingDynamicScripts) == 0 {
		return false
	}
	node := r.pendingDynamicScripts[0]
	r.pendingDynamicScripts = r.pendingDynamicScripts[1:]
	delete(r.dynamicScriptQueued, node)
	if node == nil || !node.IsConnected() || r.dynamicScriptFetcher == nil {
		return true
	}
	src := strings.TrimSpace(node.GetAttribute("src"))
	if src == "" {
		return true
	}
	if r.dynamicScriptStarted[node] {
		return true
	}
	if r.dynamicScriptStarted == nil {
		r.dynamicScriptStarted = make(map[*dom.Node]bool)
	}
	r.dynamicScriptStarted[node] = true
	if r.dynamicScriptLimit > 0 && r.dynamicScriptCount >= r.dynamicScriptLimit {
		r.dispatchWithWindowStep(node, dom.NewEvent("error", false, false))
		if len(r.dynamicScriptErrors) < 64 {
			r.dynamicScriptErrors = append(r.dynamicScriptErrors, src+": external script limit exceeded")
		}
		return true
	}
	r.dynamicScriptCount++
	base := ""
	if u := r.documentURLForNode(node); u != nil {
		base = u.String()
	}
	data, err := r.dynamicScriptFetcher(base, src)
	if err == nil && len(data) <= MaxScriptBytes {
		err = r.Run(string(data), src)
	} else if err == nil {
		err = ErrTooLarge
	}
	typ := "load"
	if err != nil {
		if len(r.dynamicScriptErrors) < 64 {
			r.dynamicScriptErrors = append(r.dynamicScriptErrors, src+": "+err.Error())
		}
		typ = "error"
	}
	r.dispatchWithWindowStep(node, dom.NewEvent(typ, false, false))
	return true
}

// DynamicScriptErrors returns bounded execution/fetch errors from this
// Runtime and its iframe browsing contexts. Hosts use it for diagnostics.
func (r *Runtime) DynamicScriptErrors() []string {
	var errors []string
	seen := make(map[*Runtime]bool)
	var visit func(*Runtime)
	visit = func(current *Runtime) {
		if current == nil || seen[current] {
			return
		}
		seen[current] = true
		errors = append(errors, current.dynamicScriptErrors...)
		for _, child := range current.iframeChildRuntimes {
			visit(child)
		}
	}
	visit(r)
	return errors
}

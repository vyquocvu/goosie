package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// setupWindowOpen exposes window.open through the same browsing-context path
// used for iframe documents. The returned proxy stays stable while navigation
// replaces the child document, as a WindowProxy does.
func (r *Runtime) setupWindowOpen(win *goja.Object) {
	if r.iframeParent == nil {
		_ = win.Set("frameElement", goja.Null())
	}
	_ = win.Set("open", func(call goja.FunctionCall) goja.Value {
		return r.openWindowCall(call)
	})
}

func (r *Runtime) openWindowCall(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || r.opts.ScriptingDisabled {
		return goja.Null()
	}
	url := "about:blank"
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		if raw := strings.TrimSpace(call.Argument(0).String()); raw != "" {
			url = r.resolveURLString(raw)
		}
	}
	name := ""
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		name = call.Argument(1).String()
	}
	features := ""
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
		features = call.Argument(2).String()
	}
	switch strings.ToLower(name) {
	case "_self", "_parent", "_top":
		return r.openExistingBrowsingContext(strings.ToLower(name), url)
	}
	noOpener := windowFeatureEnabled(features, "noopener") || windowFeatureEnabled(features, "noreferrer")
	if noOpener {
		// The noopener feature forces a new unnamed top-level context rather
		// than reusing the named target, and window.open returns null.
		name = ""
		opened := r.openBrowsingContext(url, name, goja.Null())
		if goja.IsNull(opened) || goja.IsUndefined(opened) {
			return opened
		}
		return goja.Null()
	}
	return r.openBrowsingContext(url, name, r.vm.GlobalObject())
}

func (r *Runtime) openExistingBrowsingContext(target, url string) goja.Value {
	owner, frame := runtimeBrowsingContext(r)
	switch target {
	case "_parent":
		// A window opened as a top-level context has itself as parent. In
		// Goosie's single-process model that is represented by an iframe marked
		// as an opened browsing context.
		if owner == nil || frame == nil || !owner.iframeOpenedContexts[frame] {
			owner, frame = runtimeBrowsingContext(owner)
		}
	case "_top":
		for owner != nil && frame != nil && !owner.iframeOpenedContexts[frame] {
			nextOwner, nextFrame := runtimeBrowsingContext(owner)
			if nextOwner == owner && nextFrame == nil {
				owner, frame = nextOwner, nil
				break
			}
			owner, frame = nextOwner, nextFrame
		}
	}
	if owner == nil {
		return goja.Null()
	}
	if url != "about:blank" && frame != nil {
		owner.navigateIFrameLocation(frame, url)
	}
	return r.windowProxyForContext(owner, frame)
}

func runtimeBrowsingContext(runtime *Runtime) (*Runtime, *dom.Node) {
	if runtime == nil {
		return nil, nil
	}
	if runtime.iframeParent != nil && runtime.iframeParentFrame != nil {
		return runtime.iframeParent, runtime.iframeParentFrame
	}
	return runtime, nil
}

func (r *Runtime) windowProxyForContext(owner *Runtime, frame *dom.Node) goja.Value {
	if r == nil || owner == nil {
		return goja.Null()
	}
	currentOwner, currentFrame := runtimeBrowsingContext(r)
	if owner == currentOwner && frame == currentFrame {
		return r.vm.GlobalObject()
	}
	if r.iframeParent != nil {
		parentOwner, parentFrame := runtimeBrowsingContext(r.iframeParent)
		if owner == parentOwner && frame == parentFrame {
			return r.iframeParentWindow
		}
		if owner == r.iframeParent && frame != nil {
			return r.importParentValue(owner, owner.iframeWindow(frame))
		}
	}
	if owner == r {
		if frame == nil {
			return r.vm.GlobalObject()
		}
		return r.iframeWindow(frame)
	}
	return goja.Null()
}

func windowFeatureEnabled(features, name string) bool {
	for _, token := range strings.Split(features, ",") {
		parts := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if !strings.EqualFold(strings.TrimSpace(parts[0]), name) {
			continue
		}
		if len(parts) == 1 {
			return true
		}
		switch strings.ToLower(strings.TrimSpace(parts[1])) {
		case "0", "no", "false", "off":
			return false
		default:
			return true
		}
	}
	return false
}

func (r *Runtime) openBrowsingContext(url, name string, opener goja.Value) goja.Value {
	if r.domDoc == nil || r.domDoc.Body == nil || r.opts.ScriptingDisabled {
		return goja.Null()
	}
	if strings.EqualFold(name, "_blank") {
		name = ""
	}
	frameValue := r.jsCreateElementInDocument(goja.FunctionCall{
		Arguments: []goja.Value{r.vm.ToValue("iframe")},
	}, r.domDoc, r.customElementRegistryForDocument(r.domDoc))
	frame := r.getNode(frameValue)
	if frame == nil {
		return goja.Null()
	}
	frameWrapper := frameValue
	r.jsSetAttribute(goja.FunctionCall{This: frameWrapper, Arguments: []goja.Value{r.vm.ToValue("style"), r.vm.ToValue("display:none")}})
	if name != "" && !strings.EqualFold(name, "_blank") {
		r.jsSetAttribute(goja.FunctionCall{This: frameWrapper, Arguments: []goja.Value{r.vm.ToValue("name"), r.vm.ToValue(name)}})
	}
	if url != "about:blank" {
		r.jsSetAttribute(goja.FunctionCall{This: frameWrapper, Arguments: []goja.Value{r.vm.ToValue("src"), r.vm.ToValue(url)}})
	}
	r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(r.domDoc.Body), Arguments: []goja.Value{frameValue}})
	if r.iframeLocations == nil {
		r.iframeLocations = make(map[*dom.Node]string)
	}
	r.iframeLocations[frame] = url
	if r.iframeOpenedContexts == nil {
		r.iframeOpenedContexts = make(map[*dom.Node]bool)
	}
	r.iframeOpenedContexts[frame] = true
	if r.iframeContextOpeners == nil {
		r.iframeContextOpeners = make(map[*dom.Node]goja.Value)
	}
	r.iframeContextOpeners[frame] = opener
	if r.iframeContextNames == nil {
		r.iframeContextNames = make(map[*dom.Node]string)
	}
	r.iframeContextNames[frame] = name
	if r.iframeHistoryEntries == nil {
		r.iframeHistoryEntries = make(map[*dom.Node][]string)
		r.iframeHistoryIndex = make(map[*dom.Node]int)
	}
	r.iframeHistoryEntries[frame] = []string{url}
	r.iframeHistoryIndex[frame] = 0

	proxy := r.iframeWindow(frame)
	target := r.iframeWindowTarget(frame)
	_ = target.Set("parent", proxy)
	_ = target.Set("top", proxy)
	_ = target.Set("frameElement", goja.Null())
	if opener != nil && !goja.IsNull(opener) && !goja.IsUndefined(opener) {
		_ = target.Set("opener", opener)
	} else {
		_ = target.Set("opener", goja.Null())
	}
	r.installIFrameHistory(frame, target)
	r.QueueTask(func() { r.flushIFrameLoads() })
	return proxy
}

func (r *Runtime) installIFrameHistory(frame *dom.Node, win *goja.Object) {
	history := r.vm.NewObject()
	_ = history.DefineAccessorProperty("length", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.iframeHistoryEntries[frame]))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = history.Set("state", goja.Null())
	_ = history.Set("back", func(goja.FunctionCall) goja.Value {
		r.traverseIFrameHistory(frame, -1)
		return goja.Undefined()
	})
	_ = history.Set("forward", func(goja.FunctionCall) goja.Value {
		r.traverseIFrameHistory(frame, 1)
		return goja.Undefined()
	})
	_ = history.Set("go", func(call goja.FunctionCall) goja.Value {
		delta := 0
		if len(call.Arguments) > 0 {
			delta = int(call.Argument(0).ToInteger())
		}
		r.traverseIFrameHistory(frame, delta)
		return goja.Undefined()
	})
	_ = win.Set("history", history)
	_ = win.DefineAccessorProperty("closed", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.iframeClosedContexts[frame])
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = win.Set("close", func(goja.FunctionCall) goja.Value {
		r.closeIFrameContext(frame)
		return goja.Undefined()
	})
}

func (r *Runtime) closeIFrameContext(frame *dom.Node) {
	if child := r.iframeChildRuntimes[frame]; child != nil {
		_ = child.Close()
		delete(r.iframeChildRuntimes, frame)
	}
	if frame != nil && frame.Parent != nil {
		frame.Parent.RemoveChild(frame)
	}
	if r.iframeClosedContexts == nil {
		r.iframeClosedContexts = make(map[*dom.Node]bool)
	}
	r.iframeClosedContexts[frame] = true
	r.notifyMutation()
}

func (r *Runtime) recordIFrameHistory(frame *dom.Node, target string) {
	if r.iframeHistoryEntries == nil {
		r.iframeHistoryEntries = make(map[*dom.Node][]string)
		r.iframeHistoryIndex = make(map[*dom.Node]int)
	}
	entries := r.iframeHistoryEntries[frame]
	if len(entries) == 0 {
		r.iframeHistoryEntries[frame] = []string{target}
		r.iframeHistoryIndex[frame] = 0
		return
	}
	index := r.iframeHistoryIndex[frame]
	if index >= 0 && index < len(entries) && entries[index] == target {
		return
	}
	if index < 0 || index >= len(entries) {
		index = len(entries) - 1
	}
	entries = append(entries[:index+1], target)
	r.iframeHistoryEntries[frame] = entries
	r.iframeHistoryIndex[frame] = len(entries) - 1
}

func (r *Runtime) cacheIFrameHistoryDocument(frame *dom.Node) {
	if frame == nil || r.iframeDocuments[frame] == nil || len(r.iframeHistoryEntries[frame]) == 0 {
		return
	}
	index := r.iframeHistoryIndex[frame]
	if index < 0 {
		return
	}
	if r.iframeHistoryDocs == nil {
		r.iframeHistoryDocs = make(map[*dom.Node]map[int]*dom.Document)
	}
	if r.iframeHistoryDocs[frame] == nil {
		r.iframeHistoryDocs[frame] = make(map[int]*dom.Document)
	}
	r.iframeHistoryDocs[frame][index] = r.iframeDocuments[frame]
}

func (r *Runtime) traverseIFrameHistory(frame *dom.Node, delta int) {
	entries := r.iframeHistoryEntries[frame]
	index := r.iframeHistoryIndex[frame] + delta
	if index < 0 || index >= len(entries) || entries[index] == r.iframeLocations[frame] {
		return
	}
	r.cacheIFrameHistoryDocument(frame)
	r.iframeHistoryIndex[frame] = index
	r.scheduleIFrameNavigation(frame, entries[index])
}

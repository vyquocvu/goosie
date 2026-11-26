package main

import (
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

const (
	maxWindowContexts       = 256
	maxPendingWindowMessage = 64
)

type windowSeed struct {
	Handle       js.WindowHandle
	Name         string
	OpenerHandle js.WindowHandle
	OpenerName   string
	Origin       string
	OpaqueID     string
	Blank        bool
}

type browsingContext struct {
	handle    js.WindowHandle
	handles   []js.WindowHandle
	tabID     uint64
	name      string
	origin    string
	opaqueKey string
	session   *engine.Session
	pending   []js.WindowMessage
	closed    bool
}

type browsingContextRegistry struct {
	mu       sync.Mutex
	next     uint64
	byHandle map[js.WindowHandle]*browsingContext
	byTab    map[uint64]*browsingContext
	byName   map[string]*browsingContext

	closedHandles map[js.WindowHandle]bool
	closedOrder   []js.WindowHandle
	nameSink      func(js.WindowHandle, string)
}

func newBrowsingContextRegistry() *browsingContextRegistry {
	return &browsingContextRegistry{
		byHandle:      make(map[js.WindowHandle]*browsingContext),
		byTab:         make(map[uint64]*browsingContext),
		byName:        make(map[string]*browsingContext),
		closedHandles: make(map[js.WindowHandle]bool),
	}
}

func (r *browsingContextRegistry) newOpaqueID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	return "opaque-" + strconv.FormatUint(r.next, 10)
}

func (r *browsingContextRegistry) allocateRoot() js.WindowHandle {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	return js.WindowHandle("root-" + strconv.FormatUint(r.next, 10))
}

func (r *browsingContextRegistry) register(handle js.WindowHandle, tabID uint64, name string) {
	if r == nil || handle == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.byTab[tabID]
	ctx := previous
	if ctx == nil {
		ctx = r.byHandle[handle]
	}
	if ctx == nil {
		ctx = &browsingContext{}
	}
	if alias := r.byHandle[handle]; alias != nil && alias != ctx {
		for _, oldHandle := range alias.handles {
			r.byHandle[oldHandle] = ctx
		}
		ctx.handles = append(ctx.handles, alias.handles...)
		ctx.pending = append(ctx.pending, alias.pending...)
		if ctx.session == nil {
			ctx.session = alias.session
		}
		if ctx.origin == "" {
			ctx.origin = alias.origin
		}
		if ctx.opaqueKey == "" {
			ctx.opaqueKey = alias.opaqueKey
		}
	}
	if ctx.handle != "" && ctx.handle != handle {
		ctx.session = nil
	}
	if previousName := ctx.name; previousName != "" && r.byName[previousName] == ctx {
		delete(r.byName, previousName)
	}
	if name != "" {
		ctx.name = name
	}
	ctx.handle = handle
	ctx.tabID = tabID
	if previous != nil && previous != ctx {
		for _, oldHandle := range previous.handles {
			r.byHandle[oldHandle] = ctx
		}
		ctx.handles = append(ctx.handles, previous.handles...)
		ctx.pending = append(ctx.pending, previous.pending...)
		if ctx.session == nil {
			ctx.session = previous.session
		}
		if ctx.origin == "" {
			ctx.origin = previous.origin
		}
	}
	seen := false
	for _, oldHandle := range ctx.handles {
		if oldHandle == handle {
			seen = true
		}
		r.byHandle[oldHandle] = ctx
	}
	if !seen {
		ctx.handles = append(ctx.handles, handle)
	}
	if len(ctx.handles) > maxWindowContexts {
		ctx.handles = ctx.handles[len(ctx.handles)-maxWindowContexts:]
	}
	r.byHandle[handle] = ctx
	r.byTab[tabID] = ctx
	if ctx.name != "" {
		r.byName[ctx.name] = ctx
	}
}

func (r *browsingContextRegistry) aliasHandle(alias, handle js.WindowHandle) {
	if r == nil || alias == "" || handle == "" || alias == handle {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byHandle[handle]
	if ctx == nil {
		return
	}
	if aliasContext := r.byHandle[alias]; aliasContext != nil && aliasContext != ctx {
		ctx.pending = append(ctx.pending, aliasContext.pending...)
		if ctx.session == nil {
			ctx.session = aliasContext.session
		}
		if ctx.origin == "" {
			ctx.origin = aliasContext.origin
		}
		if ctx.opaqueKey == "" {
			ctx.opaqueKey = aliasContext.opaqueKey
		}
	}
	r.byHandle[alias] = ctx
	seen := false
	for _, existing := range ctx.handles {
		if existing == alias {
			seen = true
		}
	}
	if !seen {
		ctx.handles = append(ctx.handles, alias)
	}
	if len(ctx.handles) > maxWindowContexts {
		ctx.handles = ctx.handles[len(ctx.handles)-maxWindowContexts:]
	}
}

func (r *browsingContextRegistry) handleForTab(tabID uint64) js.WindowHandle {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byTab[tabID]
	if ctx == nil {
		return ""
	}
	return ctx.handle
}

func allowedWindowMessages(ctx *browsingContext, messages []js.WindowMessage) []js.WindowMessage {
	if ctx == nil || ctx.origin == "" {
		return nil
	}
	allowed := make([]js.WindowMessage, 0, len(messages))
	for _, message := range messages {
		ok := js.TargetOriginAllows(message.TargetOrigin, ctx.origin, message.Origin)
		if ctx.origin == "null" && message.TargetOrigin == "/" {
			ok = message.SourceKey == ctx.opaqueKey
		}
		if ok {
			allowed = append(allowed, message)
		}
	}
	return allowed
}

func (r *browsingContextRegistry) bindSession(handle js.WindowHandle, session *engine.Session, origin string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byHandle[handle]
	if ctx == nil {
		ctx = &browsingContext{handle: handle, handles: []js.WindowHandle{handle}}
		r.byHandle[handle] = ctx
	}
	ctx.session = session
	ctx.origin = origin
	if origin == "null" {
		ctx.opaqueKey = session.WindowOpaqueID()
		if ctx.opaqueKey == "" {
			ctx.opaqueKey = string(handle)
		}
	} else {
		ctx.opaqueKey = ""
	}
}

func (r *browsingContextRegistry) deliver(message js.WindowMessage) {
	if r == nil || message.Target == "" {
		return
	}
	data := append([]byte(nil), message.Data...)
	message.Data = data
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byHandle[message.Target]
	if ctx == nil {
		if len(r.byHandle) >= maxWindowContexts {
			return
		}
		ctx = &browsingContext{handle: message.Target, handles: []js.WindowHandle{message.Target}}
		r.byHandle[message.Target] = ctx
	}
	if len(ctx.pending) >= maxPendingWindowMessage {
		ctx.pending = ctx.pending[1:]
	}
	ctx.pending = append(ctx.pending, message)
}

func redirectWindowMessages(ctx *browsingContext, messages []js.WindowMessage) {
	if ctx == nil {
		return
	}
	for i := range messages {
		messages[i].Target = ctx.handle
	}
}

func (r *browsingContextRegistry) takePending(tabID uint64) (*engine.Session, []js.WindowMessage) {
	if r == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byTab[tabID]
	if ctx == nil || ctx.session == nil || len(ctx.pending) == 0 {
		return nil, nil
	}
	pending := allowedWindowMessages(ctx, ctx.pending)
	ctx.pending = nil
	if len(pending) == 0 {
		return nil, nil
	}
	redirectWindowMessages(ctx, pending)
	return ctx.session, pending
}

type pendingWindowDelivery struct {
	session  *engine.Session
	messages []js.WindowMessage
}

func (r *browsingContextRegistry) flushAll() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	deliveries := make([]pendingWindowDelivery, 0, len(r.byTab))
	for _, ctx := range r.byTab {
		if ctx.session == nil || len(ctx.pending) == 0 {
			continue
		}
		messages := allowedWindowMessages(ctx, ctx.pending)
		ctx.pending = nil
		if len(messages) == 0 {
			continue
		}
		redirectWindowMessages(ctx, messages)
		deliveries = append(deliveries, pendingWindowDelivery{session: ctx.session, messages: messages})
	}
	r.mu.Unlock()
	for _, delivery := range deliveries {
		for _, message := range delivery.messages {
			delivery.session.DeliverWindowMessage(message)
		}
	}
	return len(deliveries) > 0
}

func (r *browsingContextRegistry) closed(handle js.WindowHandle) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closedHandles[handle] {
		return true
	}
	ctx := r.byHandle[handle]
	return ctx != nil && ctx.closed
}

func (r *browsingContextRegistry) rename(handle js.WindowHandle, name string) {
	if r == nil || handle == "" {
		return
	}
	r.mu.Lock()
	ctx := r.byHandle[handle]
	if ctx == nil {
		r.mu.Unlock()
		return
	}
	if ctx.name != "" && r.byName[ctx.name] == ctx {
		delete(r.byName, ctx.name)
	}
	ctx.name = name
	if name != "" {
		r.byName[name] = ctx
	}
	sink := r.nameSink
	r.mu.Unlock()
	if sink != nil {
		sink(handle, name)
	}
}

func (r *browsingContextRegistry) removeTab(tabID uint64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byTab[tabID]
	if ctx == nil {
		return
	}
	ctx.closed = true
	ctx.session = nil
	ctx.pending = nil
	for _, handle := range ctx.handles {
		if r.byHandle[handle] == ctx {
			delete(r.byHandle, handle)
		}
		if !r.closedHandles[handle] {
			r.closedHandles[handle] = true
			r.closedOrder = append(r.closedOrder, handle)
			if len(r.closedOrder) > maxWindowContexts {
				oldest := r.closedOrder[0]
				r.closedOrder = r.closedOrder[1:]
				delete(r.closedHandles, oldest)
			}
		}
	}
	if ctx.name != "" && r.byName[ctx.name] == ctx {
		delete(r.byName, ctx.name)
	}
	delete(r.byTab, tabID)
}

func (r *browsingContextRegistry) tabIDForHandle(handle js.WindowHandle) uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx := r.byHandle[handle]
	if ctx == nil {
		return 0
	}
	return ctx.tabID
}

func browsingContextOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" {
		return "null"
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "http" || scheme == "ws") && port == "80" {
		port = ""
	}
	if (scheme == "https" || scheme == "wss") && port == "443" {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	switch scheme {
	case "http", "https", "ws", "wss":
		return scheme + "://" + host
	default:
		return "null"
	}
}

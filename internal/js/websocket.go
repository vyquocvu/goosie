package js

import (
	"sync"

	"github.com/dop251/goja"
)

// WebSocket readyState constants.
const (
	WSConnecting = 0
	WSOpen       = 1
	WSClosing    = 2
	WSClosed     = 3
)

// WebSocketConn is the interface a WebSocket connection satisfies. The engine
// provides the concrete implementation; internal/js defines the interface so
// it does not import internal/net (archtest forbids it).
type WebSocketConn interface {
	Send(data string) error
	SendBytes(data []byte) error
	Close() error
	ReadMessage() (messageType int, data []byte, err error)
}

// WebSocketDialer opens a WebSocket connection. The engine wires the concrete
// dialer at pipeline assembly time.
type WebSocketDialer interface {
	Dial(url string, protocols []string) (WebSocketConn, string, error)
}

// wsEvent is a pending WebSocket event to be drained by Tick.
type wsEvent struct {
	typ      string
	data     string
	bdata    []byte
	code     int
	reason   string
	wasClean bool
}

// wsObj holds the internal state of one WebSocket object.
type wsObj struct {
	mu         sync.Mutex
	conn       WebSocketConn
	url        string
	protocol   string
	extensions string
	state      int
	buffered   int
	binaryType string // "blob" or "arraybuffer"
	events     []wsEvent
	obj        *goja.Object
	r          *Runtime
	closed     bool
}

const wsStateKey = "__wsState__"

type wsState struct {
	mu      sync.Mutex
	dialer  WebSocketDialer
	sockets []*wsObj
}

func setWSState(r *Runtime, s *wsState) {
	_ = r.vm.Set(wsStateKey, s)
}

func wsStateOf(r *Runtime) *wsState {
	v := r.vm.Get(wsStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	s, ok := v.Export().(*wsState)
	if !ok {
		return nil
	}
	return s
}

// setupWebSocket installs the WebSocket constructor on the global scope.
func (r *Runtime) setupWebSocket(dialer WebSocketDialer) {
	state := &wsState{
		dialer: dialer,
	}
	setWSState(r, state)

	// WebSocket constructor.
	constructor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("WebSocket requires a URL argument"))
		}

		urlStr := call.Arguments[0].String()

		var protocols []string
		if len(call.Arguments) >= 2 {
			arg1 := call.Arguments[1]
			if arg1.ExportType() != nil {
				switch v := arg1.Export().(type) {
				case string:
					protocols = []string{v}
				case []interface{}:
					for _, p := range v {
						if s, ok := p.(string); ok {
							protocols = append(protocols, s)
						}
					}
				}
			}
		}

		ws := &wsObj{
			url:        urlStr,
			state:      WSConnecting,
			binaryType: "blob",
			r:          r,
		}

		obj := call.This
		ws.obj = obj

		// readyState property.
		_ = obj.Set("readyState", WSConnecting)
		_ = obj.Set("url", urlStr)
		_ = obj.Set("protocol", "")
		_ = obj.Set("extensions", "")
		_ = obj.Set("bufferedAmount", 0)
		_ = obj.Set("binaryType", "blob")

		// Event handlers (initially null).
		_ = obj.Set("onopen", goja.Null())
		_ = obj.Set("onmessage", goja.Null())
		_ = obj.Set("onclose", goja.Null())
		_ = obj.Set("onerror", goja.Null())

		// send(data) method.
		_ = obj.Set("send", func(call goja.FunctionCall) goja.Value {
			ws.mu.Lock()
			if ws.state != WSOpen {
				ws.mu.Unlock()
				panic(r.vm.NewTypeError("WebSocket is not open"))
			}
			conn := ws.conn
			ws.mu.Unlock()

			if conn == nil {
				return goja.Undefined()
			}

			arg := call.Arguments[0]
			var err error
			switch v := arg.Export().(type) {
			case string:
				err = conn.Send(v)
			default:
				// For binary data, convert to string representation.
				err = conn.Send(arg.String())
			}

			if err != nil {
				ws.mu.Lock()
				ws.events = append(ws.events, wsEvent{typ: "error"})
				ws.mu.Unlock()
			}
			return goja.Undefined()
		})

		// close(code, reason) method.
		_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
			ws.mu.Lock()
			if ws.state == WSClosing || ws.state == WSClosed {
				ws.mu.Unlock()
				return goja.Undefined()
			}
			ws.state = WSClosing
			_ = obj.Set("readyState", WSClosing)
			conn := ws.conn
			ws.mu.Unlock()

			var code int = 1000
			var reason string
			if len(call.Arguments) >= 1 {
				code = int(call.Arguments[0].ToInteger())
			}
			if len(call.Arguments) >= 2 {
				reason = call.Arguments[1].String()
			}

			if conn != nil {
				_ = conn.Close()
			}

			ws.mu.Lock()
			ws.state = WSClosed
			ws.closed = true
			_ = obj.Set("readyState", WSClosed)
			ws.events = append(ws.events, wsEvent{
				typ:      "close",
				code:     code,
				reason:   reason,
				wasClean: true,
			})
			ws.mu.Unlock()

			return goja.Undefined()
		})

		// Connect asynchronously.
		state.mu.Lock()
		state.sockets = append(state.sockets, ws)
		state.mu.Unlock()

		go func() {
			if state.dialer == nil {
				ws.mu.Lock()
				ws.events = append(ws.events, wsEvent{typ: "error"})
				ws.state = WSClosed
				ws.closed = true
				ws.events = append(ws.events, wsEvent{
					typ:      "close",
					code:     1006,
					reason:   "no dialer",
					wasClean: false,
				})
				ws.mu.Unlock()
				return
			}

			conn, protocol, err := state.dialer.Dial(urlStr, protocols)
			ws.mu.Lock()
			if err != nil {
				ws.events = append(ws.events, wsEvent{typ: "error"})
				ws.state = WSClosed
				ws.closed = true
				ws.events = append(ws.events, wsEvent{
					typ:      "close",
					code:     1006,
					reason:   err.Error(),
					wasClean: false,
				})
				ws.mu.Unlock()
				return
			}
			if ws.closed {
				ws.mu.Unlock()
				_ = conn.Close()
				return
			}

			ws.conn = conn
			ws.protocol = protocol
			ws.state = WSOpen
			// NOTE: no goja access here. DrainWSEvents syncs readyState/protocol
			// onto the JS object on the VM thread before firing the open event.
			ws.events = append(ws.events, wsEvent{typ: "open"})
			ws.mu.Unlock()

			// Read messages in a loop.
			for {
				msgType, data, err := conn.ReadMessage()
				ws.mu.Lock()
				if err != nil {
					if ws.state == WSOpen {
						ws.state = WSClosed
						ws.closed = true
						ws.events = append(ws.events, wsEvent{
							typ:      "close",
							code:     1006,
							reason:   "",
							wasClean: false,
						})
					}
					ws.mu.Unlock()
					return
				}

				if msgType == 1 { // text
					ws.events = append(ws.events, wsEvent{
						typ:  "message",
						data: string(data),
					})
				} else { // binary
					ws.events = append(ws.events, wsEvent{
						typ:   "message",
						bdata: data,
					})
				}
				ws.mu.Unlock()
			}
		}()

		return nil
	}

	proto := r.vm.NewObject()
	proto.Set("CONNECTING", WSConnecting)
	proto.Set("OPEN", WSOpen)
	proto.Set("CLOSING", WSClosing)
	proto.Set("CLOSED", WSClosed)

	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("WebSocket", ctor)

	// Also install on window.
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("WebSocket", ctor)
		}
	}

	// Static constants on the constructor.
	if wsCtor := r.vm.Get("WebSocket"); wsCtor != nil {
		if wsObj, ok := wsCtor.(*goja.Object); ok {
			_ = wsObj.Set("CONNECTING", WSConnecting)
			_ = wsObj.Set("OPEN", WSOpen)
			_ = wsObj.Set("CLOSING", WSClosing)
			_ = wsObj.Set("CLOSED", WSClosed)
		}
	}

	_ = proto
}

// DrainWSEvents drains pending WebSocket events and fires them on the main
// goroutine. Call this from Tick. It also syncs readyState/protocol from the
// plain-Go fields onto the JS objects, so background goroutines never touch
// goja values.
func (r *Runtime) DrainWSEvents() {
	state := wsStateOf(r)
	if state == nil {
		return
	}

	state.mu.Lock()
	sockets := make([]*wsObj, len(state.sockets))
	copy(sockets, state.sockets)
	state.mu.Unlock()

	for _, ws := range sockets {
		ws.mu.Lock()
		events := make([]wsEvent, len(ws.events))
		copy(events, ws.events)
		ws.events = ws.events[:0]
		readyState := ws.state
		protocol := ws.protocol
		obj := ws.obj
		ws.mu.Unlock()

		if obj != nil {
			_ = obj.Set("readyState", readyState)
			_ = obj.Set("protocol", protocol)
		}
		for _, ev := range events {
			fireWSEvent(r, obj, ev)
		}
	}
}

func fireWSEvent(r *Runtime, obj *goja.Object, ev wsEvent) {
	handlerName := "on" + ev.typ
	handler := obj.Get(handlerName)
	if handler == nil || goja.IsNull(handler) || goja.IsUndefined(handler) {
		return
	}

	fn, ok := goja.AssertFunction(handler)
	if !ok {
		return
	}

	eventObj := r.vm.NewObject()
	_ = eventObj.Set("type", ev.typ)

	switch ev.typ {
	case "message":
		if ev.bdata != nil {
			_ = eventObj.Set("data", string(ev.bdata))
		} else {
			_ = eventObj.Set("data", ev.data)
		}
	case "close":
		_ = eventObj.Set("code", ev.code)
		_ = eventObj.Set("reason", ev.reason)
		_ = eventObj.Set("wasClean", ev.wasClean)
	}

	_, _ = fn(obj, eventObj)
}

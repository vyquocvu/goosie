package dom

import (
	"reflect"
	"strings"
	"sync"
	"time"
)

func funcEqual(a, b func(*Event)) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

type EventPhase int

const (
	NonePhase EventPhase = iota
	CapturingPhase
	AtTarget
	BubblingPhase
)

type Event struct {
	Type                       string
	target                     *Node
	currentTarget              *Node
	phase                      EventPhase
	bubbles                    bool
	cancelable                 bool
	isTrusted                  bool
	interfaceName              string
	persisted                  bool
	defaultPrevented           bool
	inPassiveListener          bool
	scrollCancelableByListener bool
	stopped                    bool
	immediateStopped           bool
	timeStamp                  time.Time

	payload any
}

func NewEvent(typ string, bubbles, cancelable bool) *Event {
	return &Event{
		Type:          typ,
		bubbles:       bubbles,
		cancelable:    cancelable,
		interfaceName: "Event",
		timeStamp:     time.Now(),
	}
}

func NewTrustedEvent(typ string, bubbles, cancelable bool) *Event {
	e := NewEvent(typ, bubbles, cancelable)
	e.isTrusted = true
	return e
}

func NewPageTransitionEvent(typ string, bubbles, cancelable, persisted bool) *Event {
	e := NewTrustedEvent(typ, bubbles, cancelable)
	e.interfaceName = "PageTransitionEvent"
	e.persisted = persisted
	return e
}

func (e *Event) IsTrusted() bool { return e.isTrusted }

func (e *Event) InterfaceName() string {
	if e.interfaceName == "" {
		return "Event"
	}
	return e.interfaceName
}

func (e *Event) Persisted() bool { return e.persisted }

func (e *Event) StopPropagation() {
	e.stopped = true
}

func (e *Event) StopImmediatePropagation() {
	e.stopped = true
	e.immediateStopped = true
}

func (e *Event) PreventDefault() {
	if e.cancelable && !e.inPassiveListener {
		e.defaultPrevented = true
	}
}

func (e *Event) SetInPassiveListener(passive bool) { e.inPassiveListener = passive }

func (e *Event) InPassiveListener() bool { return e.inPassiveListener }

func (e *Event) SetTarget(target *Node) { e.target = target }

func (e *Event) Target() *Node { return e.target }

func (e *Event) CurrentTarget() *Node { return e.currentTarget }

func (e *Event) Phase() EventPhase { return e.phase }

func (e *Event) Bubbles() bool { return e.bubbles }

func (e *Event) Cancelable() bool {
	if !e.cancelable {
		return false
	}
	if e.isTrusted && IsScrollBlockingEventType(e.Type) {
		return e.scrollCancelableByListener
	}
	return true
}

func (e *Event) SetScrollCancelableByListener(present bool) {
	e.scrollCancelableByListener = present
}

func IsScrollBlockingEventType(typ string) bool {
	switch typ {
	case "wheel", "mousewheel", "touchstart", "touchmove":
		return true
	}
	return false
}

func (e *Event) DefaultPrevented() bool { return e.defaultPrevented }

func (e *Event) PropagationStopped() bool { return e.stopped }

func (e *Event) ClearPropagationFlags() {
	e.stopped = false
	e.immediateStopped = false
}

func (e *Event) TimeStamp() time.Time { return e.timeStamp }

type MouseEvent struct {
	Event
	ClientX       int32
	ClientY       int32
	ScreenX       int32
	ScreenY       int32
	RelatedTarget *Node
	Button        int
	Buttons       int
	Detail        int
	AltKey        bool
	CtrlKey       bool
	ShiftKey      bool
	MetaKey       bool
}

type PointerEvent struct {
	MouseEvent
	PointerID   int32
	Width       float32
	Height      float32
	Pressure    float32
	TiltX       int32
	TiltY       int32
	Twist       int32
	PointerType string
	IsPrimary   bool
}

type WheelEvent struct {
	MouseEvent
	DeltaX float64
	DeltaY float64
	DeltaZ float64
}

type mouseEventPayload interface {
	mouseEvent() *MouseEvent
}

func (e *MouseEvent) mouseEvent() *MouseEvent { return e }

func (e *PointerEvent) mouseEvent() *MouseEvent { return &e.MouseEvent }

func (e *WheelEvent) mouseEvent() *MouseEvent { return &e.MouseEvent }

func NewTrustedWheelEvent(typ string, deltaX, deltaY, clientX, clientY float64) *Event {
	we := &WheelEvent{
		MouseEvent: MouseEvent{
			Event:   *NewTrustedEvent(typ, true, true),
			ClientX: int32(clientX),
			ClientY: int32(clientY),
			ScreenX: int32(clientX),
			ScreenY: int32(clientY),
		},
		DeltaX: deltaX,
		DeltaY: deltaY,
	}
	e := &we.Event
	e.interfaceName = "WheelEvent"
	e.payload = we
	return e
}

type Touch struct {
	Identifier int
	ClientX    float64
	ClientY    float64
	Target     *Node
}

type TouchEvent struct {
	Event
	Touches        []*Touch
	TargetTouches  []*Touch
	ChangedTouches []*Touch
}

func NewTrustedTouchEvent(typ string, target *Node, touches, changed []*Touch) *Event {
	te := &TouchEvent{
		Event:          *NewTrustedEvent(typ, true, true),
		Touches:        touches,
		ChangedTouches: changed,
	}
	for _, touch := range touches {
		if touch != nil && touch.Target == target && target != nil {
			te.TargetTouches = append(te.TargetTouches, touch)
		}
	}
	e := &te.Event
	e.interfaceName = "TouchEvent"
	e.payload = te
	return e
}

type ErrorEvent struct {
	Event
	Message  string
	Filename string
	Lineno   uint32
	Colno    uint32
	Error    any
}

type HashChangeEvent struct {
	Event
	OldURL string
	NewURL string
}

type SubmitEvent struct {
	Event
	Submitter *Node
}

type ToggleEvent struct {
	Event
	OldState string
	NewState string
	Source   *Node
}

type FormDataEvent struct {
	Event
	FormData any
}

type ClipboardEvent struct {
	Event
	ClipboardData *DataTransfer
}

type MessageEvent struct {
	Event
	Data        any
	Origin      string
	LastEventID string
	Source      any
	Ports       []any
}

func NewMessageEvent(typ string, bubbles, cancelable bool, data any, origin, lastEventID string, source any, ports []any) *MessageEvent {
	e := &MessageEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "MessageEvent",
			timeStamp:     time.Now(),
		},
		Data: data, Origin: origin, LastEventID: lastEventID, Source: source, Ports: ports,
	}
	e.Event.payload = e
	return e
}

func NewTrustedMessageEvent(typ string, bubbles, cancelable bool, data any, origin, lastEventID string, source any, ports []any) *MessageEvent {
	e := NewMessageEvent(typ, bubbles, cancelable, data, origin, lastEventID, source, ports)
	e.isTrusted = true
	return e
}

func NewErrorEvent(typ string, bubbles, cancelable bool, message, filename string, lineno, colno uint32, value any) *ErrorEvent {
	e := &ErrorEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "ErrorEvent",
			timeStamp:     time.Now(),
		},
		Message: message, Filename: filename, Lineno: lineno, Colno: colno, Error: value,
	}
	e.Event.payload = e
	return e
}

func NewHashChangeEvent(typ string, bubbles, cancelable bool, oldURL, newURL string) *HashChangeEvent {
	e := &HashChangeEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "HashChangeEvent",
			timeStamp:     time.Now(),
		},
		OldURL: oldURL,
		NewURL: newURL,
	}
	e.Event.payload = e
	return e
}

func NewSubmitEvent(typ string, bubbles, cancelable bool, submitter *Node) *SubmitEvent {
	e := &SubmitEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "SubmitEvent",
			timeStamp:     time.Now(),
		},
		Submitter: submitter,
	}
	e.Event.payload = e
	return e
}

func NewToggleEvent(typ string, bubbles, cancelable bool, oldState, newState string) *ToggleEvent {
	e := &ToggleEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "ToggleEvent",
			timeStamp:     time.Now(),
		},
		OldState: oldState,
		NewState: newState,
	}
	e.Event.payload = e
	return e
}

func NewTrustedToggleEvent(typ string, bubbles, cancelable bool, oldState, newState string) *ToggleEvent {
	e := NewToggleEvent(typ, bubbles, cancelable, oldState, newState)
	e.Event.isTrusted = true
	return e
}

func NewFormDataEvent(typ string, bubbles, cancelable bool, formData any) *FormDataEvent {
	e := &FormDataEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "FormDataEvent",
			timeStamp:     time.Now(),
		},
		FormData: formData,
	}
	e.Event.payload = e
	return e
}

func NewTrustedClipboardEvent(typ string, bubbles, cancelable bool, data *DataTransfer) *ClipboardEvent {
	e := &ClipboardEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			isTrusted:     true,
			interfaceName: "ClipboardEvent",
			timeStamp:     time.Now(),
		},
		ClipboardData: data,
	}
	e.Event.payload = e
	return e
}

func (e *Event) AsClipboard() *ClipboardEvent {
	clipboard, _ := e.payload.(*ClipboardEvent)
	return clipboard
}

func NewMouseEvent(typ string, bubbles, cancelable bool, clientX, clientY, screenX, screenY int32, button int) *MouseEvent {
	me := &MouseEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "MouseEvent",
			timeStamp:     time.Now(),
		},
		ClientX: clientX,
		ClientY: clientY,
		ScreenX: screenX,
		ScreenY: screenY,
		Button:  button,
	}
	me.Event.payload = me
	return me
}

func NewPointerEvent(typ string, bubbles, cancelable bool, clientX, clientY int32, button int) *PointerEvent {
	me := &PointerEvent{
		MouseEvent: MouseEvent{
			Event: Event{
				Type:          typ,
				bubbles:       bubbles,
				cancelable:    cancelable,
				interfaceName: "PointerEvent",
				timeStamp:     time.Now(),
			},
			ClientX: clientX,
			ClientY: clientY,
			ScreenX: clientX,
			ScreenY: clientY,
			Button:  button,
		},
		PointerID:   1,
		Width:       1,
		Height:      1,
		PointerType: "mouse",
		IsPrimary:   true,
	}
	me.Event.payload = me
	return me
}

func NewTrustedPointerEvent(typ string, clientX, clientY int32, button int) *Event {
	pe := NewPointerEvent(typ, true, true, clientX, clientY, button)
	pe.Event.isTrusted = true
	return &pe.Event
}

func NewTrustedPointerEventFlags(typ string, clientX, clientY int32, button int, bubbles, cancelable bool) *Event {
	pe := NewPointerEvent(typ, bubbles, cancelable, clientX, clientY, button)
	pe.Event.isTrusted = true
	return &pe.Event
}

func NewTrustedMouseEvent(typ string, clientX, clientY int32, button, buttons, detail int) *Event {
	me := NewMouseEvent(typ, true, true, clientX, clientY, clientX, clientY, button)
	me.Event.isTrusted = true
	me.Buttons = buttons
	me.Detail = detail
	return &me.Event
}

func NewTrustedMouseEventFlags(typ string, clientX, clientY int32, button, buttons, detail int, bubbles, cancelable bool) *Event {
	me := NewMouseEvent(typ, bubbles, cancelable, clientX, clientY, clientX, clientY, button)
	me.Event.isTrusted = true
	me.Buttons = buttons
	me.Detail = detail
	return &me.Event
}

type KeyboardEvent struct {
	Event
	Key      string
	Code     string
	AltKey   bool
	CtrlKey  bool
	ShiftKey bool
	MetaKey  bool
	Repeat   bool
}

type InputEvent struct {
	Event
	Data         *string
	InputType    string
	IsComposing  bool
	DataTransfer *DataTransfer
	TargetRanges []StaticRange
}

type StaticRange struct {
	StartContainer *Node
	StartOffset    int
	EndContainer   *Node
	EndOffset      int
}

type CompositionEvent struct {
	Event
	Data string
}

func NewKeyboardEvent(typ string, bubbles, cancelable bool, key, code string) *KeyboardEvent {
	ke := &KeyboardEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "KeyboardEvent",
			timeStamp:     time.Now(),
		},
		Key:  key,
		Code: code,
	}
	ke.Event.payload = ke
	return ke
}

func NewInputEvent(typ string, bubbles, cancelable bool, data *string, inputType string, isComposing bool) *InputEvent {
	e := &InputEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "InputEvent",
			timeStamp:     time.Now(),
		},
		Data: data, InputType: inputType, IsComposing: isComposing,
	}
	e.Event.payload = e
	return e
}

func NewTrustedInputEvent(typ string, bubbles, cancelable bool, data *string, inputType string, isComposing bool) *InputEvent {
	e := NewInputEvent(typ, bubbles, cancelable, data, inputType, isComposing)
	e.Event.isTrusted = true
	return e
}

func (e *Event) AsInput() *InputEvent {
	input, _ := e.payload.(*InputEvent)
	return input
}

func NewCompositionEvent(typ string, bubbles, cancelable bool, data string) *CompositionEvent {
	e := &CompositionEvent{
		Event: Event{
			Type:          typ,
			bubbles:       bubbles,
			cancelable:    cancelable,
			interfaceName: "CompositionEvent",
			timeStamp:     time.Now(),
		},
		Data: data,
	}
	e.Event.payload = e
	return e
}

func NewTrustedCompositionEvent(typ string, bubbles, cancelable bool, data string) *CompositionEvent {
	e := NewCompositionEvent(typ, bubbles, cancelable, data)
	e.Event.isTrusted = true
	return e
}

func (e *Event) AsComposition() *CompositionEvent {
	composition, _ := e.payload.(*CompositionEvent)
	return composition
}

func (e *Event) AsKeyboard() *KeyboardEvent {
	ke, _ := e.payload.(*KeyboardEvent)
	return ke
}

func (e *Event) AsMouse() *MouseEvent {
	me, _ := e.payload.(mouseEventPayload)
	if me == nil {
		return nil
	}
	return me.mouseEvent()
}

func (e *Event) AsPointer() *PointerEvent {
	pe, _ := e.payload.(*PointerEvent)
	return pe
}

func (e *Event) AsWheel() *WheelEvent {
	we, _ := e.payload.(*WheelEvent)
	return we
}

func (e *Event) AsTouch() *TouchEvent {
	te, _ := e.payload.(*TouchEvent)
	return te
}

func (e *Event) AsError() *ErrorEvent {
	ee, _ := e.payload.(*ErrorEvent)
	return ee
}

func (e *Event) AsHashChange() *HashChangeEvent {
	he, _ := e.payload.(*HashChangeEvent)
	return he
}

func (e *Event) AsSubmit() *SubmitEvent {
	se, _ := e.payload.(*SubmitEvent)
	return se
}

func (e *Event) AsToggle() *ToggleEvent {
	te, _ := e.payload.(*ToggleEvent)
	return te
}

func (e *Event) AsFormData() *FormDataEvent {
	fe, _ := e.payload.(*FormDataEvent)
	return fe
}

func (e *Event) AsMessage() *MessageEvent {
	me, _ := e.payload.(*MessageEvent)
	return me
}

type DataTransfer struct {
	store         *DataTransferStore
	associated    bool
	dropEffect    string
	effectAllowed string
}

type DataTransferStore struct {
	data             map[string]string
	order            []string
	itemIDs          map[string]uint64
	nextItemID       uint64
	mode             DataTransferMode
	allowedEffects   string
	currentOperation string
}

type DataTransferMode uint8

const (
	DataTransferReadWrite DataTransferMode = iota
	DataTransferReadOnly
	DataTransferProtected
	DataTransferDisabled
)

func NewDataTransfer() *DataTransfer {
	return &DataTransfer{
		store: &DataTransferStore{
			data:           make(map[string]string),
			itemIDs:        make(map[string]uint64),
			mode:           DataTransferReadWrite,
			allowedEffects: "none",
		},
		associated:    true,
		dropEffect:    "none",
		effectAllowed: "none",
	}
}

func NewDragDataTransfer() *DataTransfer {
	return &DataTransfer{
		store: &DataTransferStore{
			data:             make(map[string]string),
			itemIDs:          make(map[string]uint64),
			mode:             DataTransferProtected,
			allowedEffects:   "uninitialized",
			currentOperation: "none",
		},
		associated:    true,
		dropEffect:    "none",
		effectAllowed: "uninitialized",
	}
}

func (dt *DataTransfer) Mode() DataTransferMode {
	if dt == nil || !dt.associated || dt.store == nil {
		return DataTransferDisabled
	}
	return dt.store.mode
}

func (dt *DataTransfer) SetMode(mode DataTransferMode) {
	if dt != nil && dt.store != nil {
		dt.store.mode = mode
	}
}

func (dt *DataTransfer) StoreIdentity() *DataTransferStore {
	if dt == nil {
		return nil
	}
	return dt.store
}

func (dt *DataTransfer) NewEventView(eventType string, mode DataTransferMode) *DataTransfer {
	if dt == nil || dt.store == nil {
		return NewDataTransfer()
	}
	dt.store.mode = mode
	dropEffect := "none"
	switch eventType {
	case "drop", "dragend":
		dropEffect = dt.store.currentOperation
	case "dragenter", "dragover":
		dropEffect = defaultDropEffect(dt.store.allowedEffects)
	}
	return &DataTransfer{
		store:         dt.store,
		associated:    true,
		dropEffect:    dropEffect,
		effectAllowed: dt.store.allowedEffects,
	}
}

func (dt *DataTransfer) Disassociate() {
	if dt != nil {
		dt.associated = false
	}
}

func defaultDropEffect(effectAllowed string) string {
	switch effectAllowed {
	case "copy", "copyLink", "copyMove", "all", "uninitialized":
		return "copy"
	case "link", "linkMove":
		return "link"
	case "move":
		return "move"
	default:
		return "none"
	}
}

func (dt *DataTransfer) StoreEffectAllowed(effect string) {
	if dt != nil && dt.store != nil {
		dt.store.allowedEffects = effect
	}
}

func (dt *DataTransfer) SetCurrentOperation(effect string) {
	if dt != nil && dt.store != nil {
		dt.store.currentOperation = effect
	}
}

func DragOperation(effectAllowed, dropEffect string) string {
	switch dropEffect {
	case "copy":
		switch effectAllowed {
		case "uninitialized", "copy", "copyLink", "copyMove", "all":
			return "copy"
		}
	case "link":
		switch effectAllowed {
		case "uninitialized", "link", "copyLink", "linkMove", "all":
			return "link"
		}
	case "move":
		switch effectAllowed {
		case "uninitialized", "move", "copyMove", "linkMove", "all":
			return "move"
		}
	}
	return "none"
}

func (dt *DataTransfer) CurrentOperation() string {
	if dt == nil || dt.store == nil {
		return "none"
	}
	return dt.store.currentOperation
}

func normalizeFormat(format string) string {
	f := strings.ToLower(strings.Trim(format, "\t\n\f\r "))
	switch f {
	case "text":
		return "text/plain"
	case "url":
		return "text/uri-list"
	}
	return f
}

func (dt *DataTransfer) SetData(format, data string) {
	if dt.Mode() != DataTransferReadWrite {
		return
	}
	store := dt.store
	key := normalizeFormat(format)
	if key == "" {
		return
	}
	if store.data == nil {
		store.data = make(map[string]string)
	}
	if _, exists := store.data[key]; exists {
		dt.removeFormat(key)
	}
	store.data[key] = data
	store.order = append(store.order, key)
	if store.itemIDs == nil {
		store.itemIDs = make(map[string]uint64)
	}
	store.itemIDs[key] = store.newItemID()
}

func (dt *DataTransfer) GetData(format string) string {
	if dt.Mode() == DataTransferDisabled || dt.Mode() == DataTransferProtected {
		return ""
	}
	if dt.store.data == nil {
		return ""
	}
	key := normalizeFormat(format)
	data, exists := dt.store.data[key]
	if !exists && strings.HasPrefix(key, "text/uri-list;") {
		data = dt.store.data["text/uri-list"]
	}
	if strings.EqualFold(strings.Trim(format, "\t\n\f\r "), "url") {
		return firstURIListEntry(data)
	}
	return data
}

func firstURIListEntry(data string) string {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func (dt *DataTransfer) ClearData(format ...string) {
	if dt.Mode() != DataTransferReadWrite || dt.store.data == nil {
		return
	}
	if len(format) == 0 {
		clear(dt.store.data)
		clear(dt.store.itemIDs)
		dt.store.order = nil
		return
	}
	key := normalizeFormat(format[0])
	dt.removeFormat(key)
}

func (dt *DataTransfer) removeFormat(key string) {
	delete(dt.store.data, key)
	delete(dt.store.itemIDs, key)
	for i, t := range dt.store.order {
		if t == key {
			dt.store.order = append(dt.store.order[:i], dt.store.order[i+1:]...)
			break
		}
	}
}

func (dt *DataTransfer) HasFormat(format string) bool {
	if dt == nil || dt.store == nil {
		return false
	}
	_, ok := dt.store.data[normalizeFormat(format)]
	return ok
}

func (dt *DataTransfer) AddStringItem(format, data string) bool {
	if dt.Mode() != DataTransferReadWrite {
		return false
	}
	store := dt.store
	key := strings.ToLower(format)
	if key == "" || dt.HasTypeString(key) {
		return false
	}
	if store.data == nil {
		store.data = make(map[string]string)
	}
	if store.itemIDs == nil {
		store.itemIDs = make(map[string]uint64)
	}
	store.data[key] = data
	store.order = append(store.order, key)
	store.itemIDs[key] = store.newItemID()
	return true
}

func (dt *DataTransfer) HasTypeString(format string) bool {
	if dt == nil || dt.store == nil {
		return false
	}
	_, ok := dt.store.data[strings.ToLower(format)]
	return ok
}

func (dt *DataTransfer) ItemID(format string) (uint64, bool) {
	if dt == nil || dt.store == nil || !dt.HasTypeString(format) {
		return 0, false
	}
	id := dt.store.itemIDs[strings.ToLower(format)]
	return id, id != 0
}

func (dt *DataTransfer) HasItemID(format string, id uint64) bool {
	current, ok := dt.ItemID(format)
	return ok && current == id
}

func (store *DataTransferStore) newItemID() uint64 {
	store.nextItemID++
	if store.nextItemID == 0 {
		store.nextItemID++
	}
	return store.nextItemID
}

func (dt *DataTransfer) RemoveStringItem(format string) {
	if dt.Mode() == DataTransferReadWrite {
		dt.removeFormat(strings.ToLower(format))
	}
}

func (dt *DataTransfer) Types() []string {
	if dt.Mode() == DataTransferDisabled || len(dt.store.order) == 0 {
		return nil
	}
	types := make([]string, len(dt.store.order))
	copy(types, dt.store.order)
	return types
}

func (dt *DataTransfer) DropEffect() string { return dt.dropEffect }

func (dt *DataTransfer) SetDropEffect(effect string) {
	switch effect {
	case "none", "copy", "move", "link":
		dt.dropEffect = effect
	}
}

func (dt *DataTransfer) EffectAllowed() string { return dt.effectAllowed }

func (dt *DataTransfer) SetEffectAllowed(effect string) {
	if dt.Mode() != DataTransferReadWrite {
		return
	}
	switch effect {
	case "none", "copy", "copyLink", "copyMove", "link", "linkMove", "move", "all", "uninitialized":
		dt.effectAllowed = effect
	}
}

func (dt *DataTransfer) SetDragImage(_ *Node, _, _ int32) {}

func dragCancelable(typ string) bool {
	switch typ {
	case "dragleave", "dragend":
		return false
	}
	return true
}

type DragEvent struct {
	MouseEvent
	DataTransfer *DataTransfer
}

func NewDragEvent(eventType string, me MouseEvent, dt *DataTransfer) *Event {
	if dt == nil {
		dt = NewDataTransfer()
	}
	de := &DragEvent{
		MouseEvent:   me,
		DataTransfer: dt,
	}
	e := NewEvent(eventType, true, dragCancelable(eventType))
	e.payload = de
	return e
}

func NewTrustedDragEvent(eventType string, me MouseEvent, dt *DataTransfer) *Event {
	e := NewDragEvent(eventType, me, dt)
	e.isTrusted = true
	return e
}

func (e *Event) AsDrag() *DragEvent {
	de, _ := e.payload.(*DragEvent)
	return de
}

type EventListener struct {
	Type     string
	Callback func(*Event)
	Capture  bool
	Passive  bool
	removed  bool
	dedupKey interface{}
}

var listenerStore = &sync.Map{}

func getListeners(n *Node) []*EventListener {
	v, _ := listenerStore.Load(n)
	if v == nil {
		return nil
	}
	return v.([]*EventListener)
}

func setListeners(n *Node, ls []*EventListener) {
	if len(ls) == 0 {
		listenerStore.Delete(n)
	} else {
		listenerStore.Store(n, ls)
	}
}

func AddEventListener(n *Node, typ string, callback func(*Event), capture bool) {
	ls := getListeners(n)
	for _, l := range ls {
		if l.Type == typ && listenerMatches(l, callback, nil, capture) {
			return
		}
	}
	ls = append(ls, &EventListener{Type: typ, Callback: callback, Capture: capture})
	setListeners(n, ls)
}

func AddEventListenerWithKey(n *Node, typ string, callback func(*Event), capture bool, key interface{}) bool {
	return AddEventListenerWithOptionsAndKey(n, typ, callback, capture, false, key)
}

func AddEventListenerWithOptionsAndKey(n *Node, typ string, callback func(*Event), capture, passive bool, key interface{}) bool {
	ls := getListeners(n)
	for _, l := range ls {
		if l.Type == typ && listenerMatches(l, callback, key, capture) {
			return false
		}
	}
	ls = append(ls, &EventListener{Type: typ, Callback: callback, Capture: capture, Passive: passive, dedupKey: key})
	setListeners(n, ls)
	return true
}

func listenerMatches(l *EventListener, callback func(*Event), key interface{}, capture bool) bool {
	if l.Capture != capture {
		return false
	}
	if key != nil {
		return l.dedupKey == key
	}
	if l.dedupKey != nil {
		return false
	}
	return funcEqual(l.Callback, callback)
}

func RemoveEventListener(n *Node, typ string, callback func(*Event), capture bool) {
	ls := getListeners(n)
	for i, l := range ls {
		if l.Type == typ && funcEqual(l.Callback, callback) && l.Capture == capture {
			l.removed = true
			ls = append(ls[:i], ls[i+1:]...)
			setListeners(n, ls)
			return
		}
	}
}

func RemoveEventListenerByKey(n *Node, typ string, key interface{}, capture bool) {
	ls := getListeners(n)
	for i, l := range ls {
		if l.Type == typ && listenerMatches(l, nil, key, capture) {
			l.removed = true
			ls = append(ls[:i], ls[i+1:]...)
			setListeners(n, ls)
			return
		}
	}
}

func GetNodeListeners(n *Node) []*EventListener {
	orig := getListeners(n)
	if orig == nil {
		return nil
	}
	cp := make([]*EventListener, len(orig))
	copy(cp, orig)
	return cp
}

func DispatchEvent(target *Node, event *Event) bool {
	return DispatchEventWithPath(target, event, buildPath(target))
}

func EventPath(target *Node) []*Node {
	return buildPath(target)
}

func DispatchEventWithPath(target *Node, event *Event, path []*Node) bool {
	return DispatchEventWithPathWhile(target, event, path, nil)
}

func DispatchEventWithPathWhile(target *Node, event *Event, path []*Node, shouldContinue func() bool) bool {
	event.target = target
	if len(path) == 0 && target != nil {
		path = buildPath(target)
	}

	event.phase = CapturingPhase
	for i := len(path) - 1; i > 0; i-- {
		if event.stopped || !dispatchShouldContinue(shouldContinue) {
			break
		}
		node := path[i]
		event.currentTarget = node
		fireListenersWhile(node, event, true, shouldContinue)
	}

	if !event.stopped && dispatchShouldContinue(shouldContinue) {
		event.phase = AtTarget
		event.currentTarget = target
		for _, l := range GetNodeListeners(target) {
			if event.immediateStopped || !dispatchShouldContinue(shouldContinue) {
				break
			}
			if !l.removed && l.Type == event.Type && l.Capture {
				invokeEventListener(l, event)
			}
		}
		if !event.stopped {
			for _, l := range GetNodeListeners(target) {
				if event.immediateStopped || !dispatchShouldContinue(shouldContinue) {
					break
				}
				if !l.removed && l.Type == event.Type && !l.Capture {
					invokeEventListener(l, event)
				}
			}
		}
	}

	if event.bubbles && !event.stopped && dispatchShouldContinue(shouldContinue) {
		event.phase = BubblingPhase
		for i := 1; i < len(path); i++ {
			if event.stopped || !dispatchShouldContinue(shouldContinue) {
				break
			}
			node := path[i]
			event.currentTarget = node
			fireListenersWhile(node, event, false, shouldContinue)
		}
	}

	event.phase = NonePhase
	event.currentTarget = nil

	return !event.defaultPrevented
}

func buildPath(target *Node) []*Node {
	var path []*Node
	for n := target; n != nil; n = n.Parent {
		path = append(path, n)
	}
	return path
}

func fireListeners(node *Node, event *Event, capture bool) {
	fireListenersWhile(node, event, capture, nil)
}

func fireListenersWhile(node *Node, event *Event, capture bool, shouldContinue func() bool) {
	for _, l := range GetNodeListeners(node) {
		if event.immediateStopped || !dispatchShouldContinue(shouldContinue) {
			break
		}
		if !l.removed && l.Type == event.Type && l.Capture == capture {
			invokeEventListener(l, event)
		}
	}
}

func dispatchShouldContinue(shouldContinue func() bool) bool {
	return shouldContinue == nil || shouldContinue()
}

func invokeEventListener(listener *EventListener, event *Event) {
	event.SetInPassiveListener(listener.Passive)
	defer event.SetInPassiveListener(false)
	listener.Callback(event)
}

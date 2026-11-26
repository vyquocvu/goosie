package engine

import "github.com/vyquocvu/goosie/internal/js"

func (s *Session) WindowHandle() js.WindowHandle {
	if s == nil || s.jsRT == nil {
		return ""
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.jsRT.WindowHandle()
}

func (s *Session) WindowName() string {
	if s == nil || s.jsRT == nil {
		return ""
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.jsRT.WindowName()
}

func (s *Session) WindowOrigin() string {
	if s == nil || s.jsRT == nil {
		return ""
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.jsRT.WindowOrigin()
}

func (s *Session) WindowOpaqueID() string {
	if s == nil || s.jsRT == nil {
		return ""
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.jsRT.WindowOpaqueID()
}

func (s *Session) DeliverWindowMessage(message js.WindowMessage) {
	if s == nil || s.jsRT == nil {
		return
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	s.jsMu.Lock()
	defer s.jsMu.Unlock()
	s.jsRT.DeliverWindowMessage(message)
}

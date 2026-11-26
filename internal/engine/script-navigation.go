package engine

import "github.com/vyquocvu/goosie/internal/js"

const maxPendingNavigations = 64

const maxPendingNavigationStateBytes = js.MaxHistoryStateBytes

func (s *Session) queueScriptNavigation(request js.NavigationRequest) {
	s.navigationMu.Lock()
	defer s.navigationMu.Unlock()
	if s.navigationClosed || len(s.pendingNavigations) >= maxPendingNavigations {
		return
	}
	stateLen := len(request.State)
	if s.pendingNavigationBytes+stateLen > maxPendingNavigationStateBytes {
		return
	}
	request.State = append([]byte(nil), request.State...)
	s.pendingNavigations = append(s.pendingNavigations, request)
	s.pendingNavigationBytes += stateLen
}

func (s *Session) PopNavigation() (js.NavigationRequest, bool) {
	s.navigationMu.Lock()
	defer s.navigationMu.Unlock()
	if s.navigationClosed || len(s.pendingNavigations) == 0 {
		return js.NavigationRequest{}, false
	}
	request := s.pendingNavigations[0]
	s.pendingNavigations[0] = js.NavigationRequest{}
	s.pendingNavigations = s.pendingNavigations[1:]
	s.pendingNavigationBytes -= len(request.State)
	return request, true
}

func (s *Session) closeScriptNavigations() {
	s.navigationMu.Lock()
	s.navigationClosed = true
	clear(s.pendingNavigations)
	s.pendingNavigations = nil
	s.pendingNavigationBytes = 0
	s.navigationMu.Unlock()
}

func (s *Session) SetHistorySnapshot(urls []string, states [][]byte, index int) {
	if s == nil {
		return
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	s.jsMu.Lock()
	defer s.jsMu.Unlock()
	if s.jsRT == nil {
		return
	}
	s.jsRT.SetHistorySnapshot(urls, states, index)
}

func (s *Session) ApplyHistoryEntry(target string, state []byte, index, length, delta int) {
	if s == nil {
		return
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	s.jsMu.Lock()
	defer s.jsMu.Unlock()
	if s.jsRT == nil {
		return
	}
	s.jsRT.ApplyHistoryEntry(target, state, index, length, delta)
}

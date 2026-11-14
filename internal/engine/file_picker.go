package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func (s *Session) queueFilePicker(n *dom.Node) {
	if n == nil {
		return
	}
	s.opMu.Lock()
	if s.pendingFilePicker == nil {
		s.pendingFilePicker = n
	}
	s.opMu.Unlock()
}

func (s *Session) PopFilePickerRequest() (*dom.Node, bool) {
	s.opMu.Lock()
	node := s.pendingFilePicker
	s.pendingFilePicker = nil
	s.opMu.Unlock()
	if node != nil {
		return node, true
	}
	if s.jsRT != nil {
		return s.jsRT.TakeFilePickerRequest()
	}
	return nil, false
}

func (s *Session) SetInputFiles(node *dom.Node, files []js.FileSelection) bool {
	if s == nil || s.jsRT == nil {
		return false
	}
	if !s.jsRT.SetSelectedFiles(node, files) {
		return false
	}
	dom.DispatchEvent(node, dom.NewTrustedEvent("input", true, false))
	dom.DispatchEvent(node, dom.NewTrustedEvent("change", true, false))
	return true
}

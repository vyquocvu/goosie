package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// queueFilePicker remembers that activating n should open the host's file
// picker. A picker is modal, so the first pending request wins until the host
// drains it.
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

// PopFilePickerRequest returns and clears the file control whose picker the
// host should open. Activation from a pointer or key records the request in the
// session; input.showPicker() and HTMLElement.click() record it in the script
// runtime, so both are drained here.
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

// SetInputFiles replaces the selection of a file input with files chosen by the
// host's picker and fires the trusted input and change events a native picker
// produces. It reports whether node is a file control in a scripted document;
// without a runtime there is no FileList to populate.
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

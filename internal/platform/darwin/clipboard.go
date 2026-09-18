//go:build darwin && cgo

package darwin

/*
#include "shim.h"
#include <stdlib.h>
*/
import "C"

import "unsafe"

// Clipboard implements toolbar.Clipboard using NSPasteboard via the shim's
// C API. All calls are thread-safe; the shim dispatches to the main queue.
type Clipboard struct{}

func NewClipboard() *Clipboard { return &Clipboard{} }

func (c *Clipboard) Read() string {
	cs := C.GoosieClipboardRead()
	if cs == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cs))
	return C.GoString(cs)
}

func (c *Clipboard) Write(s string) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.GoosieClipboardWrite(cs)
}

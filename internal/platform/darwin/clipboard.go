//go:build darwin && cgo

package darwin

/*
#include "shim.h"
#include <stdlib.h>
*/
import "C"

import "unsafe"

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

func (c *Clipboard) ReadHTML() string {
	cs := C.GoosieClipboardReadHTML()
	if cs == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cs))
	return C.GoString(cs)
}

func (c *Clipboard) Write(s string) {
	c.WriteRich(s, "")
}

func (c *Clipboard) WriteRich(text, html string) {
	textValue := C.CString(text)
	defer C.free(unsafe.Pointer(textValue))
	htmlValue := C.CString(html)
	defer C.free(unsafe.Pointer(htmlValue))
	C.GoosieClipboardWriteRich(textValue, htmlValue)
}

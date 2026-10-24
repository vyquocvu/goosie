//go:build darwin && cgo

package darwin

/*
#include "shim.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ChooseSavePath presents the native save panel and returns the selected path.
// The panel is always marshaled onto AppKit's main thread by the shim.
func (w *Window) ChooseSavePath(directory, filename string) (string, bool, error) {
	if w == nil || w.gw == nil {
		return "", false, ErrClosed
	}
	cDirectory := C.CString(directory)
	defer C.free(unsafe.Pointer(cDirectory))
	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	var selected *C.char
	switch result := C.GoosieChooseSavePath(w.gw, cDirectory, cFilename, &selected); result {
	case 1:
		if selected == nil {
			return "", false, errors.New("platform/darwin: save panel returned no path")
		}
		defer C.free(unsafe.Pointer(selected))
		return C.GoString(selected), true, nil
	case 0:
		return "", false, nil
	default:
		return "", false, errors.New("platform/darwin: save panel unavailable")
	}
}

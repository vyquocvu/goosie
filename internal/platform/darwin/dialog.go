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

// ChooseFiles presents the native open panel and returns the selected paths.
// The panel is always marshaled onto AppKit's main thread by the shim.
func (w *Window) ChooseFiles(accept string, multiple bool) ([]string, bool, error) {
	if w == nil || w.gw == nil {
		return nil, false, ErrClosed
	}
	cAccept := C.CString(accept)
	defer C.free(unsafe.Pointer(cAccept))
	multipleArg := C.int(0)
	if multiple {
		multipleArg = 1
	}
	var paths *C.char
	var count C.int
	switch result := C.GoosieChooseFiles(w.gw, cAccept, multipleArg, &paths, &count); result {
	case 1:
		if paths == nil || count <= 0 {
			return nil, false, errors.New("platform/darwin: open panel returned no paths")
		}
		defer C.free(unsafe.Pointer(paths))
		selected := make([]string, 0, int(count))
		cursor := paths
		for i := 0; i < int(count); i++ {
			path := C.GoString(cursor)
			selected = append(selected, path)
			cursor = (*C.char)(unsafe.Add(unsafe.Pointer(cursor), len(path)+1))
		}
		return selected, true, nil
	case 0:
		return nil, false, nil
	default:
		return nil, false, errors.New("platform/darwin: open panel unavailable")
	}
}

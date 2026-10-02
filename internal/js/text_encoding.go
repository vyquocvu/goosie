package js

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// TextEncoder and TextDecoder API implementation.

// setupTextEncoding installs TextEncoder and TextDecoder constructors.
func (r *Runtime) setupTextEncoding() {
	// TextEncoder constructor.
	encoderCtor := func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		_ = obj.Set("encoding", "utf-8")

		_ = obj.Set("encode", func(call goja.FunctionCall) goja.Value {
			var input string
			if len(call.Arguments) >= 1 {
				input = call.Arguments[0].String()
			}
			bytes := []byte(input)
			arr := r.vm.NewArray()
			for i, b := range bytes {
				_ = arr.Set(fmt.Sprintf("%d", i), int(b))
			}
			return arr
		})

		_ = obj.Set("encodeInto", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			input := call.Arguments[0].String()
			dest := call.Arguments[1]
			if dest == nil || goja.IsUndefined(dest) {
				return goja.Undefined()
			}
			destObj, ok := dest.(*goja.Object)
			if !ok {
				return goja.Undefined()
			}

			bytes := []byte(input)
			read := len(bytes)
			written := len(bytes)

			for i, b := range bytes {
				_ = destObj.Set(fmt.Sprintf("%d", i), int(b))
			}

			result := r.vm.NewObject()
			_ = result.Set("read", read)
			_ = result.Set("written", written)
			return result
		})

		return nil
	}

	encVal := r.vm.ToValue(encoderCtor)
	_ = r.vm.Set("TextEncoder", encVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("TextEncoder", encVal)
		}
	}

	// TextDecoder constructor.
	decoderCtor := func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		encoding := "utf-8"
		if len(call.Arguments) >= 1 {
			encoding = strings.ToLower(call.Arguments[0].String())
		}
		_ = obj.Set("encoding", encoding)
		_ = obj.Set("fatal", false)
		_ = obj.Set("ignoreBOM", false)

		_ = obj.Set("decode", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return r.vm.ToValue("")
			}

			arg := call.Arguments[0]
			var bytes []byte

			// Handle Uint8Array or Array
			if arrObj, ok := arg.(*goja.Object); ok {
				lengthVal := arrObj.Get("length")
				if lengthVal == nil || goja.IsUndefined(lengthVal) {
					return r.vm.ToValue("")
				}
				length := int(lengthVal.ToInteger())
				bytes = make([]byte, length)
				for i := 0; i < length; i++ {
					val := arrObj.Get(fmt.Sprintf("%d", i))
					if val != nil && !goja.IsUndefined(val) {
						bytes[i] = byte(val.ToInteger())
					}
				}
			}

			switch encoding {
			case "utf-16le":
				if len(bytes) >= 2 {
					// Decode UTF-16LE
					u16s := make([]uint16, len(bytes)/2)
					for i := range u16s {
						u16s[i] = binary.LittleEndian.Uint16(bytes[i*2 : i*2+2])
					}
					runes := utf16.Decode(u16s)
					return r.vm.ToValue(string(runes))
				}
				return r.vm.ToValue("")
			case "utf-16be":
				if len(bytes) >= 2 {
					u16s := make([]uint16, len(bytes)/2)
					for i := range u16s {
						u16s[i] = binary.BigEndian.Uint16(bytes[i*2 : i*2+2])
					}
					runes := utf16.Decode(u16s)
					return r.vm.ToValue(string(runes))
				}
				return r.vm.ToValue("")
			default:
				// UTF-8 (default)
				if utf8.Valid(bytes) {
					return r.vm.ToValue(string(bytes))
				}
				// Replace invalid UTF-8 with replacement character
				var sb strings.Builder
				for len(bytes) > 0 {
					r, size := utf8.DecodeRune(bytes)
					if r == utf8.RuneError && size == 1 {
						sb.WriteRune('\uFFFD')
					} else {
						sb.WriteRune(r)
					}
					bytes = bytes[size:]
				}
				return r.vm.ToValue(sb.String())
			}
		})

		return nil
	}

	decVal := r.vm.ToValue(decoderCtor)
	_ = r.vm.Set("TextDecoder", decVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("TextDecoder", decVal)
		}
	}
}

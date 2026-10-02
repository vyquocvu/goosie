package js

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/dop251/goja"
)

// Web Crypto API (subset) and Performance API implementation.

// setupCrypto installs the crypto global with getRandomValues.
func (r *Runtime) setupCrypto() {
	cryptoObj := r.vm.NewObject()

	_ = cryptoObj.Set("getRandomValues", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("getRandomValues requires 1 argument"))
		}

		arg := call.Arguments[0]
		arrObj, ok := arg.(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("getRandomValues argument must be a typed array"))
		}

		lengthVal := arrObj.Get("length")
		if lengthVal == nil || goja.IsUndefined(lengthVal) {
			panic(r.vm.NewTypeError("getRandomValues argument must have a length"))
		}
		length := int(lengthVal.ToInteger())

		if length > 65536 {
			panic(r.vm.NewTypeError("getRandomValues: requested length exceeds 65536 bytes"))
		}

		buf := make([]byte, length)
		if _, err := rand.Read(buf); err != nil {
			panic(r.vm.ToValue("getRandomValues failed"))
		}

		for i := 0; i < length; i++ {
			_ = arrObj.Set(fmt.Sprintf("%d", i), int(buf[i]))
		}

		return arrObj
	})

	_ = r.vm.Set("crypto", cryptoObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("crypto", cryptoObj)
		}
	}
}

// setupPerformance installs the performance global with now().
func (r *Runtime) setupPerformance() {
	startTime := time.Now()

	perfObj := r.vm.NewObject()

	_ = perfObj.Set("now", func(call goja.FunctionCall) goja.Value {
		elapsed := time.Since(startTime)
		return r.vm.ToValue(elapsed.Seconds() * 1000) // milliseconds
	})

	_ = perfObj.Set("timeOrigin", startTime.UnixMilli())

	// mark/measure stubs.
	_ = perfObj.Set("mark", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = perfObj.Set("measure", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = perfObj.Set("getEntriesByName", func(call goja.FunctionCall) goja.Value {
		return r.vm.NewArray()
	})
	_ = perfObj.Set("getEntriesByType", func(call goja.FunctionCall) goja.Value {
		return r.vm.NewArray()
	})
	_ = perfObj.Set("clearMarks", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = perfObj.Set("clearMeasures", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})

	_ = r.vm.Set("performance", perfObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("performance", perfObj)
		}
	}
}

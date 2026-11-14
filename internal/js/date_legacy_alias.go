package js

import (
	"github.com/dop251/goja"
)

func (r *Runtime) setupDateLegacyAliases() {
	dateVal := r.vm.Get("Date")
	if dateVal == nil {
		return
	}
	date, ok := dateVal.(*goja.Object)
	if !ok {
		return
	}
	protoVal := date.Get("prototype")
	if protoVal == nil {
		return
	}
	proto, ok := protoVal.(*goja.Object)
	if !ok {
		return
	}
	toUTC, ok := goja.AssertFunction(proto.Get("toUTCString"))
	if !ok {
		return
	}
	_ = proto.Set("toGMTString", func(call goja.FunctionCall) goja.Value {
		value, err := toUTC(call.This)
		if err != nil {
			panic(err)
		}
		return value
	})
}

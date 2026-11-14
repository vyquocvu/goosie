package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) SetAccessibilityProvider(provider func(*dom.Node) (role, label string, ok bool)) error {
	if r == nil || r.vm == nil {
		return fmt.Errorf("missing JavaScript runtime")
	}
	query := func(call goja.FunctionCall) goja.Value {
		if provider == nil {
			return goja.Null()
		}
		node := r.getNode(call.Argument(0))
		if node == nil {
			return goja.Null()
		}
		role, label, ok := provider(node)
		if !ok {
			return goja.Null()
		}
		result := r.vm.NewObject()
		_ = result.Set("role", role)
		_ = result.Set("label", label)
		return result
	}
	return r.vm.Set("__goosieAccessibilityInfo", query)
}

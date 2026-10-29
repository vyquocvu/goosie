package js

import (
	"github.com/dop251/goja"
)

// installLegacyOptionConstructor implements the HTML legacy Option() factory.
// Unlike HTMLOptionElement's interface constructor, Option() creates and
// returns a detached option with the supplied text, value, and selectedness.
func (r *Runtime) installLegacyOptionConstructor() {
	proto := r.protoForTag("option")
	constructor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if r.domDoc == nil {
			panic(r.vm.NewTypeError("Option is unavailable without a document"))
		}
		option := r.domDoc.NewElement("option")
		text := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			text = call.Argument(0).String()
		}
		if text != "" {
			option.AppendChild(r.domDoc.NewText(text))
		}
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			option.SetAttribute("value", call.Argument(1).String())
		}
		defaultSelected := len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && call.Argument(2).ToBoolean()
		// The third argument sets the selected content attribute only. The
		// initial selectedness remains false unless the fourth argument is
		// provided, matching the legacy constructor's special rule.
		selected := false
		if len(call.Arguments) > 3 && !goja.IsUndefined(call.Argument(3)) {
			selected = call.Argument(3).ToBoolean()
		}
		if defaultSelected {
			option.SetAttribute("selected", "")
		}
		option.SetOptionSelected(selected, false)
		return r.wrapNode(option).(*goja.Object)
	}).(*goja.Object)
	if !ok {
		return
	}
	_ = constructor.Set("prototype", proto)
	_ = proto.Set("constructor", constructor)
	_ = r.vm.Set("Option", constructor)
}

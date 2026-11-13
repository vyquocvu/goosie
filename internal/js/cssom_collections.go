package js

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// The three CSSOM collections each answer for an interface of their own.
//
// Measured 2026-11-13 on one http fixture served to Chromium 153.0.8010.12, the
// build the Node Playwright 1.63.0 oracle drives (viewport 1280x400, DPR 1; the
// dumps are /tmp/b287/cr_col.json through cr_col12.txt plus cr_colA/B/C, and
// test/js/cssom_collections_test.go and test/engine/cssom_collection_test.go
// read this engine against the same numbers). Re-running those 14 probes on the
// pinned render oracle - Playwright 1.49.1, Chromium 131.0.6778.33 - agreed on
// 12; the two that did not are the MediaList a page holds across a media
// attribute change (recorded on the liveness law below) and the name at the top
// of the StyleSheetList prototype chain, which no oracle reads:
// `Object.getPrototypeOf(Object.getPrototypeOf(styleSheets)).name` is null on
// 131 and undefined on 153.
//
// Object.prototype.toString.call(x)  [object StyleSheetList] / [object CSSRuleList] / [object MediaList]
// Object.getOwnPropertyNames(list)   the indices only: "0+1+2" for three sheets, "0" for one rule,
//
//	"0+1" for a two-entry MediaList; Object.keys gives the same,
//	so length and item are not own
//
// own names of the prototype         length+item+constructor, and for MediaList
//
//	length+mediaText+item+toString+constructor (Object.keys drops
//	constructor); appendMedium/deleteMedium also live on Chromium's
//	MediaList prototype and are deferred with insertRule to
//	GOOSIE-CSSOMMUTATION-001
//
// length descriptor on the proto     getter only, enumerable, configurable, and no setter
// item descriptor on the proto       data function of arity 1, enumerable; MediaList's toString
//
//	has arity 0
//
// item identity                      rules.item === rules.item === CSSRuleList.prototype.item
// own symbols of each proto          Symbol.toStringTag + Symbol.iterator, the iterator
//
//	non-enumerable, writable, configurable; for-of over each list
//	yields its members
//
// prototype chain                    one link, ending at Object.prototype
// instanceof / isPrototypeOf         true against the globals StyleSheetList / CSSRuleList / MediaList
// the globals themselves             functions of length 0; calling one is
//
//	`TypeError: Illegal constructor`, and `new`ing one is
//	`TypeError: Failed to construct 'X': Illegal
//	 constructor` - this engine answers the bare message both ways,
//	house style in every interface, which is GOOSIE-CTORMESSAGE-001
//
// for-in over a list                 indices, then the enumerable proto members
// sheet.cssRules === sheet.cssRules  true, and a second sheet's list is a different object
//
// The receiver law is an identity law. Every prototype member answers
// `TypeError: Illegal invocation` for a receiver that is not a list the engine
// handed out - the length getter, item, MediaList's mediaText getter and
// toString, with or without arguments, and including `[]`, `{ length: 3 }`,
// `{ length: 3, 1: 'x' }`, one wearing the interface's own @@toStringTag, and
// any of the three lists handed to a *different* interface's member. Called on a
// real list, the same members answer live: the length getter on a detached copy
// gives 3 / 1 / 2 for the three collections and MediaList's mediaText getter
// gives its text.
//
// item() then needs its index: `StyleSheetList.item()` with nothing at all is
// `TypeError: Failed to execute 'item' on 'StyleSheetList': 1 argument
// required, but only 0 present.` (per interface), a Symbol argument the same
// sentence ending `Cannot convert a Symbol value to a number`, and the check for
// both comes after the receiver. undefined, null, NaN, `{}` and a non-numeric
// string convert to 0 and give the first member, 1.7 truncates to the second,
// and past the end or below zero is null.
//
// A MediaList is the media attribute, read now: `media="screen, print"` gives
// two entries whose descriptors are non-writable, enumerable and configurable,
// `String(media)` and `media.toString()` are the text, and one top-level comma
// query (`screen and (min-width: 100px)`) stays one entry. A sheet with no media
// attribute - link or style element alike - reports the empty text, length 0 and
// no own index at all, not "all". After `L1.setAttribute("media", …)` the
// MediaList already in hand reports the new text, and after appending a <style>
// the StyleSheetList already in hand has length 4 with the new sheet at index 3.
// That liveness law is the one the two reference builds disagree on, measured on
// the same fixture: held after the change, 153.0.8010.12 answers `screen and
// (min-width: 100px),1` and re-reads the very same object, while 131.0.6778.33
// keeps answering `screen, print,2` on the handle in hand and hands a *different*
// object on the next read. This engine follows 153, which is the reading the
// interface defines - the list is a live view of the attribute, not a snapshot of
// it - and the BR-285 oracle pins it (`TestCSSOMCollectionsStayLiveThroughOneObject`).
//
// Reads of a live collection go through its Proxy trap, because that is what
// keeps them live: a sheet appended, removed or disabled moves through the same
// list object without a rebuild. The prototype supplies the members, and each of
// their bodies first asks the identity registry below whether the receiver is a
// list this runtime handed out - Chromium tests exactly that, since even a
// hand-made object wearing the right @@toStringTag is refused. Once the test
// passes, the body reads through the receiver again, so `item(i)` and `list[i]`
// hand back the same wrapper.
func (r *Runtime) cssomCollectionProto(name string) *goja.Object {
	if r.cssomCollectionProtos == nil {
		r.cssomCollectionProtos = map[string]*goja.Object{}
	}
	if proto, ok := r.cssomCollectionProtos[name]; ok {
		return proto
	}
	proto := r.vm.NewObject()
	_ = proto.SetPrototype(r.vm.Get("Object").ToObject(r.vm).Get("prototype").ToObject(r.vm))
	_ = proto.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return cssomProperty(r.cssomCollectionReceiver(call.This, name), "length")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	// Own names come back in insertion order, and Chromium's MediaList lists
	// mediaText between length and item, so the order here is the measured one:
	// length+mediaText+appendMedium+deleteMedium+item+toString+constructor, of
	// which the two mutation methods are deferred to
	// GOOSIE-CSSOMMUTATION-001.
	if name == "MediaList" {
		_ = proto.DefineAccessorProperty("mediaText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			return cssomProperty(r.cssomCollectionReceiver(call.This, name), "mediaText")
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = proto.DefineDataProperty("item", r.cssomMethod("item", true, r.cssomItemFor(name)), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	if name == "MediaList" {
		_ = proto.DefineDataProperty("toString", r.cssomMethod("toString", false, func(call goja.FunctionCall) goja.Value {
			text := cssomProperty(r.cssomCollectionReceiver(call.This, name), "mediaText")
			if goja.IsUndefined(text) || goja.IsNull(text) {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			return r.vm.ToValue(text.String())
		}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	// defineConstructor installs the brand, the non-enumerable constructor and
	// the global a page can name the interface with - all three measured above.
	r.defineConstructor(name, proto)
	// The iterator goes on after the brand because both are own symbols and a
	// page reads them in insertion order: measured, a prototype's symbol list is
	// Symbol.toStringTag then Symbol.iterator, the second one non-enumerable.
	iterator, err := r.vm.RunString("Array.prototype[Symbol.iterator]")
	if err == nil {
		_ = proto.DefineDataPropertySymbol(goja.SymIterator, iterator, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	r.cssomCollectionProtos[name] = proto
	return proto
}

// cssomItemFor builds one interface's item(). Chromium checks the receiver
// before the arguments - `CSSRuleList.prototype.item.call({ length: 3 })`
// answers `Illegal invocation`, not the arity error - and then needs its index:
// `StyleSheetList.item()` answers `TypeError: Failed to execute 'item' on
// 'StyleSheetList': 1 argument required, but only 0 present.`, and a Symbol
// argument the same sentence ending `Cannot convert a Symbol value to a number`.
// Out of range and negative indexes give null; undefined, null, NaN, `{}` and a
// non-numeric string all convert to 0 and give the first member, and a real
// number is truncated, so `item(1.7)` answers the second member.
func (r *Runtime) cssomItemFor(name string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		obj := r.cssomCollectionReceiver(call.This, name)
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'item' on '" + name + "': 1 argument required, but only 0 present."))
		}
		if _, isSymbol := call.Argument(0).(*goja.Symbol); isSymbol {
			panic(r.vm.NewTypeError("Failed to execute 'item' on '" + name + "': Cannot convert a Symbol value to a number"))
		}
		var count int64
		switch length := cssomProperty(obj, "length").Export().(type) {
		case int64:
			count = length
		case int:
			count = int64(length)
		case float64:
			count = int64(length)
		default:
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		index := call.Argument(0).ToInteger()
		if index < 0 || index >= count {
			return goja.Null()
		}
		value := cssomProperty(obj, strconv.Itoa(int(index)))
		if goja.IsUndefined(value) || goja.IsNull(value) {
			return goja.Null()
		}
		return value
	}
}

// cssomCollectionReceiver is this engine's brand check: the receiver has to be a
// live list this runtime handed out, and one of the interface being called.
// Chromium's own test is the wrapper it gave the page, which a hand-made object
// cannot fake even with the right @@toStringTag, so the registry - not a
// property shape or a tag - is what answers here.
func (r *Runtime) cssomCollectionReceiver(this goja.Value, name string) *goja.Object {
	obj, ok := this.(*goja.Object)
	if !ok || obj == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	if owned, live := r.cssomCollections[obj]; !live || owned != name {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	return obj
}

// cssomCollection records the list a page can call members on, together with the
// interface name that test compares against. Every collection this file builds
// passes through it, and the entries live as long as the caches that already
// hold these lists.
func (r *Runtime) cssomCollection(value goja.Value, name string) goja.Value {
	if r.cssomCollections == nil {
		r.cssomCollections = map[*goja.Object]string{}
	}
	if obj, ok := value.(*goja.Object); ok {
		r.cssomCollections[obj] = name
	}
	return value
}

// cssomProperty reads one property off a receiver. Object.Get answers a Go nil
// for a property that is not there at all, so the nil has to become a Value
// before any page-visible comparison or the measured TypeError turns into a
// crash.
func cssomProperty(obj *goja.Object, name string) goja.Value {
	if value := obj.Get(name); value != nil {
		return value
	}
	return goja.Undefined()
}

// cssomMethod gives a prototype method the arity Chromium reports - item is a
// function of one argument on all three interfaces, MediaList's toString of
// none - which a Go closure cannot answer because goja reports 0 for every one
// of them. It forwards `arguments` rather than the named parameter, so the body
// sees how many values the page really passed: Chromium's `item()` with nothing
// at all throws, and a wrapper that always handed over its (undefined) index
// would answer the first member instead.
func (r *Runtime) cssomMethod(name string, takesIndex bool, impl func(goja.FunctionCall) goja.Value) *goja.Object {
	source := `(function (impl) {
		function NAME() { return impl.apply(this, arguments); }
		Object.defineProperty(NAME, 'name', { value: 'NAME', configurable: true });
		return NAME;
	})(__goosieCSSOMMethod__)`
	if takesIndex {
		source = `(function (impl) {
		function NAME(index) { return impl.apply(this, arguments); }
		Object.defineProperty(NAME, 'name', { value: 'NAME', configurable: true });
		return NAME;
	})(__goosieCSSOMMethod__)`
	}
	source = strings.ReplaceAll(source, "NAME", name)
	_ = r.vm.Set("__goosieCSSOMMethod__", r.vm.ToValue(impl))
	value, err := r.vm.RunString(source)
	_ = r.vm.Set("__goosieCSSOMMethod__", nil)
	if err != nil || value == nil {
		return r.vm.ToValue(impl).(*goja.Object)
	}
	return value.ToObject(r.vm)
}

// cssRuleList answers the one CSSRuleList of one sheet. Chromium hands back the
// same object on every read - `sheet.cssRules === sheet.cssRules` is true there,
// and a second sheet's list is a different object - so the list is cached
// against the owner together with the text its rules were built from.
//
// A sheet whose text changed gets a new list and the handle the page already
// holds keeps the rules it was built from. Measured: after replacing S1's
// textContent with two rules, `s[2].cssRules === before` is false and the fresh
// read reports 2 rules and their new selectors while `before.length` is still 1
// and `before[0] === after[0]` is false. The same law holds across the four edit
// shapes (/tmp/b287/index12.html): an appended rule makes the next read 3 while
// the handle in hand stays 2, and a replacement that drops one makes it 1 while
// the handle stays 2. That is the opposite of the StyleSheetList and the
// MediaList, both of which a page watches change through the one object it
// already has.
//
// What this does NOT close is the cascade: Chromium follows the edit with its
// computed styles in the same task - a removed rule leaves rgb(0, 0, 0) and an
// added one computes rgb(9, 9, 9) for a matching element - while this engine
// keeps the colours it resolved at load. That half is GOOSIE-STYLETEXTRULES-001.
func (r *Runtime) cssRuleList(owner *dom.Node) goja.Value {
	text := r.sheetCSSText(owner)
	if cached, ok := r.cssRuleListObjects[owner]; ok && cached.text == text {
		return cached.value
	}
	if r.cssRuleListObjects == nil {
		r.cssRuleListObjects = map[*dom.Node]cachedCSSRuleList{}
	}
	value := r.newCSSRuleList(r.sheetRuleObjects(owner))
	r.cssRuleListObjects[owner] = cachedCSSRuleList{text: text, value: value}
	return value
}

type cachedCSSRuleList struct {
	text  string
	value goja.Value
}

func (r *Runtime) newCSSRuleList(items []*goja.Object) goja.Value {
	target := r.vm.NewObject()
	_ = target.SetPrototype(r.cssomCollectionProto("CSSRuleList"))
	return r.cssomCollection(r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(proxied *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "length" {
				return r.vm.ToValue(len(items))
			}
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				if index >= 0 && index < len(items) {
					return items[index]
				}
				return goja.Undefined()
			}
			return proxied.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			if index >= 0 && index < len(items) {
				return items[index]
			}
			return goja.Undefined()
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(items) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			if index < 0 || index >= len(items) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: items[index], Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(proxied *goja.Object) *goja.Object {
			keys := make([]interface{}, 0, len(items)+len(proxied.Keys()))
			for index := range items {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range proxied.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})), "CSSRuleList")
}

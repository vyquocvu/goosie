package js

import (
	"bytes"
	"math"
	"strconv"
	"unicode/utf16"

	"github.com/dop251/goja"
)

const (
	maxIndexedDBKeyDepth       = 64
	maxIndexedDBKeyArrayLength = 1 << 16
)

// IndexedDB key kinds are ordered by the IndexedDB key comparison algorithm.
type indexedDBKeyKind uint8

const (
	idbNumber indexedDBKeyKind = iota + 1
	idbDate
	idbString
	idbBinary
	idbArray
)

type indexedDBKey struct {
	kind   indexedDBKeyKind
	number float64
	text   string
	binary []byte
	array  []indexedDBKey
}

type indexedDBKeyRange struct {
	lower, upper         indexedDBKey
	hasLower, hasUpper   bool
	lowerOpen, upperOpen bool
}

// setupIndexedDB installs the realm's stable IDBFactory object. The factory's
// cmp() operation is useful independently of database persistence and follows
// the ordering and invalid-key rules required by IndexedDB.
func (r *Runtime) setupIndexedDB() {
	factory := r.vm.NewObject()
	datePrototype := r.vm.Get("Date").ToObject(r.vm).Get("prototype").ToObject(r.vm)
	r.indexedDBDateGetTime, _ = goja.AssertFunction(datePrototype.Get("getTime"))
	_ = factory.Set("cmp", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if !call.This.StrictEquals(factory) {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if len(call.Arguments) < 2 {
			panic(r.vm.NewTypeError("Failed to execute 'cmp' on 'IDBFactory': 2 arguments required"))
		}
		left, ok := r.toIndexedDBKey(call.Argument(0), 0)
		if !ok {
			panic(r.domError("DataError", "The first argument is not a valid IndexedDB key"))
		}
		right, ok := r.toIndexedDBKey(call.Argument(1), 0)
		if !ok {
			panic(r.domError("DataError", "The second argument is not a valid IndexedDB key"))
		}
		return r.vm.ToValue(compareIndexedDBKeys(left, right))
	}))
	tag := r.vm.Get("Symbol").ToObject(r.vm).Get("toStringTag").(*goja.Symbol)
	_ = factory.DefineDataPropertySymbol(tag, r.vm.ToValue("IDBFactory"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	_ = r.vm.GlobalObject().DefineDataProperty("indexedDB", factory, goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.setupIDBKeyRange()
}

func (r *Runtime) setupIDBKeyRange() {
	prototype := r.vm.NewObject()
	ranges := make(map[*goja.Object]*indexedDBKeyRange)
	if r.ifaceCtors == nil {
		r.ifaceCtors = make(map[string]*goja.Object)
	}
	constructor := r.defineConstructor("IDBKeyRange", prototype)
	getRange := func(this goja.Value) *indexedDBKeyRange {
		object, ok := this.(*goja.Object)
		if !ok || ranges[object] == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		return ranges[object]
	}
	_ = prototype.DefineAccessorProperty("lower", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := getRange(call.This)
		if !value.hasLower {
			return goja.Undefined()
		}
		return r.indexedDBKeyValue(value.lower)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = prototype.DefineAccessorProperty("upper", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := getRange(call.This)
		if !value.hasUpper {
			return goja.Undefined()
		}
		return r.indexedDBKeyValue(value.upper)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = prototype.DefineAccessorProperty("lowerOpen", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(getRange(call.This).lowerOpen)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = prototype.DefineAccessorProperty("upperOpen", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(getRange(call.This).upperOpen)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = prototype.Set("includes", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := getRange(call.This)
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'includes' on 'IDBKeyRange': 1 argument required"))
		}
		key, ok := r.toIndexedDBKey(call.Argument(0), 0)
		if !ok {
			panic(r.domError("DataError", "The value is not a valid IndexedDB key"))
		}
		if value.hasLower {
			cmp := compareIndexedDBKeys(key, value.lower)
			if cmp < 0 || cmp == 0 && value.lowerOpen {
				return r.vm.ToValue(false)
			}
		}
		if value.hasUpper {
			cmp := compareIndexedDBKeys(key, value.upper)
			if cmp > 0 || cmp == 0 && value.upperOpen {
				return r.vm.ToValue(false)
			}
		}
		return r.vm.ToValue(true)
	}))
	newRange := func(value *indexedDBKeyRange) *goja.Object {
		object := r.vm.NewObject()
		_ = object.SetPrototype(prototype)
		ranges[object] = value
		return object
	}
	keyArgument := func(call goja.FunctionCall, index int, method string) indexedDBKey {
		if len(call.Arguments) <= index {
			panic(r.vm.NewTypeError("Failed to execute '" + method + "' on 'IDBKeyRange': required argument missing"))
		}
		key, ok := r.toIndexedDBKey(call.Argument(index), 0)
		if !ok {
			panic(r.domError("DataError", "The argument is not a valid IndexedDB key"))
		}
		return key
	}
	_ = constructor.Set("only", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		key := keyArgument(call, 0, "only")
		return newRange(&indexedDBKeyRange{lower: key, upper: key, hasLower: true, hasUpper: true})
	}))
	_ = constructor.Set("lowerBound", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		key := keyArgument(call, 0, "lowerBound")
		open := len(call.Arguments) > 1 && call.Argument(1).ToBoolean()
		return newRange(&indexedDBKeyRange{lower: key, hasLower: true, lowerOpen: open, upperOpen: true})
	}))
	_ = constructor.Set("upperBound", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		key := keyArgument(call, 0, "upperBound")
		open := len(call.Arguments) > 1 && call.Argument(1).ToBoolean()
		return newRange(&indexedDBKeyRange{upper: key, hasUpper: true, lowerOpen: true, upperOpen: open})
	}))
	_ = constructor.Set("bound", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		lower := keyArgument(call, 0, "bound")
		upper := keyArgument(call, 1, "bound")
		lowerOpen := len(call.Arguments) > 2 && call.Argument(2).ToBoolean()
		upperOpen := len(call.Arguments) > 3 && call.Argument(3).ToBoolean()
		cmp := compareIndexedDBKeys(lower, upper)
		if cmp > 0 {
			panic(r.domError("DataError", "The lower bound must not exceed the upper bound"))
		}
		return newRange(&indexedDBKeyRange{lower: lower, upper: upper, hasLower: true, hasUpper: true, lowerOpen: lowerOpen, upperOpen: upperOpen})
	}))
}

func (r *Runtime) indexedDBKeyValue(key indexedDBKey) goja.Value {
	switch key.kind {
	case idbNumber:
		return r.vm.ToValue(key.number)
	case idbDate:
		date, err := r.vm.New(r.vm.Get("Date"), r.vm.ToValue(key.number))
		if err != nil {
			return goja.Undefined()
		}
		return date
	case idbString:
		return r.vm.ToValue(key.text)
	case idbBinary:
		return r.vm.ToValue(r.vm.NewArrayBuffer(append([]byte(nil), key.binary...)))
	case idbArray:
		items := make([]interface{}, 0, len(key.array))
		for _, item := range key.array {
			items = append(items, r.indexedDBKeyValue(item))
		}
		return r.vm.NewArray(items...)
	default:
		return goja.Undefined()
	}
}

func (r *Runtime) toIndexedDBKey(value goja.Value, depth int) (indexedDBKey, bool) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) || depth > maxIndexedDBKeyDepth {
		return indexedDBKey{}, false
	}
	if _, symbol := value.(*goja.Symbol); symbol {
		return indexedDBKey{}, false
	}
	if object, ok := value.(*goja.Object); ok {
		switch object.ClassName() {
		case "Date":
			if r.indexedDBDateGetTime == nil {
				return indexedDBKey{}, false
			}
			date, err := r.indexedDBDateGetTime(object)
			if err != nil || math.IsNaN(date.ToFloat()) || math.IsInf(date.ToFloat(), 0) {
				return indexedDBKey{}, false
			}
			return indexedDBKey{kind: idbDate, number: date.ToFloat()}, true
		case "ArrayBuffer":
			buffer, ok := object.Export().(goja.ArrayBuffer)
			if !ok || buffer.Detached() {
				return indexedDBKey{}, false
			}
			return indexedDBKey{kind: idbBinary, binary: append([]byte(nil), buffer.Bytes()...)}, true
		case "Array":
			length := object.Get("length").ToInteger()
			if length < 0 || length > maxIndexedDBKeyArrayLength {
				return indexedDBKey{}, false
			}
			items := make([]indexedDBKey, 0, int(length))
			for index := int64(0); index < length; index++ {
				item, ok := r.toIndexedDBKey(object.Get(strconv.Itoa(int(index))), depth+1)
				if !ok {
					return indexedDBKey{}, false
				}
				items = append(items, item)
			}
			return indexedDBKey{kind: idbArray, array: items}, true
		}
		if info, ok := r.vm.ArrayBufferViewInfo(object); ok {
			if info.OutOfBounds || info.Buffer == nil {
				return indexedDBKey{}, false
			}
			buffer, ok := info.Buffer.Export().(goja.ArrayBuffer)
			if !ok || buffer.Detached() || buffer.Shared() {
				return indexedDBKey{}, false
			}
			bytes := buffer.Bytes()
			start, end := info.ByteOffset, info.ByteOffset+info.ByteLength
			if start < 0 || end < start || end > len(bytes) {
				return indexedDBKey{}, false
			}
			return indexedDBKey{kind: idbBinary, binary: append([]byte(nil), bytes[start:end]...)}, true
		}
		return indexedDBKey{}, false
	}
	switch exported := value.Export().(type) {
	case int64:
		return indexedDBKey{kind: idbNumber, number: float64(exported)}, true
	case float64:
		if math.IsNaN(exported) || math.IsInf(exported, 0) {
			return indexedDBKey{}, false
		}
		return indexedDBKey{kind: idbNumber, number: exported}, true
	case string:
		return indexedDBKey{kind: idbString, text: exported}, true
	default:
		return indexedDBKey{}, false
	}
}

func compareIndexedDBKeys(left, right indexedDBKey) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	switch left.kind {
	case idbNumber, idbDate:
		if left.number < right.number {
			return -1
		}
		if left.number > right.number {
			return 1
		}
	case idbString:
		if cmp := compareUTF16(left.text, right.text); cmp != 0 {
			return cmp
		}
	case idbBinary:
		return sign(bytes.Compare(left.binary, right.binary))
	case idbArray:
		limit := len(left.array)
		if len(right.array) < limit {
			limit = len(right.array)
		}
		for index := 0; index < limit; index++ {
			if cmp := compareIndexedDBKeys(left.array[index], right.array[index]); cmp != 0 {
				return cmp
			}
		}
		if len(left.array) < len(right.array) {
			return -1
		}
		if len(left.array) > len(right.array) {
			return 1
		}
	}
	return 0
}

func compareUTF16(left, right string) int {
	a, b := utf16CodeUnits(left), utf16CodeUnits(right)
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for index := 0; index < limit; index++ {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func utf16CodeUnits(value string) []uint16 {
	// Goja's String export is UTF-8. This conversion preserves IndexedDB's
	// UTF-16 code-unit order, including supplementary characters.
	return utf16.Encode([]rune(value))
}

func sign(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}

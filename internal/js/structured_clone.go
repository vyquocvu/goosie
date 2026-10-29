package js

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

const (
	maxBroadcastCloneNodes = 100_000
	maxBroadcastCloneDepth = 128
	maxBroadcastCloneBytes = 8 << 20
)

var errBroadcastDataClone = errors.New("value could not be structured-cloned")

type broadcastCloneValue struct {
	Kind         string                 `json:"kind"`
	ID           int                    `json:"id,omitempty"`
	Ref          int                    `json:"ref,omitempty"`
	Bool         bool                   `json:"bool,omitempty"`
	String       string                 `json:"string,omitempty"`
	Bytes        string                 `json:"bytes,omitempty"`
	Offset       int                    `json:"offset,omitempty"`
	Length       int                    `json:"length,omitempty"`
	Buffer       *broadcastCloneValue   `json:"buffer,omitempty"`
	Name         string                 `json:"name,omitempty"`
	LastModified int64                  `json:"lastModified,omitempty"`
	Items        []*broadcastCloneValue `json:"items,omitempty"`
	Props        []broadcastCloneProp   `json:"props,omitempty"`
	Pairs        []broadcastClonePair   `json:"pairs,omitempty"`
}

type broadcastCloneProp struct {
	Name  string               `json:"name"`
	Value *broadcastCloneValue `json:"value"`
}

type broadcastClonePair struct {
	Key   *broadcastCloneValue `json:"key"`
	Value *broadcastCloneValue `json:"value"`
}

type broadcastCloneEncoder struct {
	r     *Runtime
	seen  map[*goja.Object]int
	nodes int
}

func serializeBroadcastMessage(r *Runtime, value goja.Value) ([]byte, error) {
	encoder := broadcastCloneEncoder{r: r, seen: make(map[*goja.Object]int)}
	cloned, err := encoder.encode(value, 0)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(cloned)
	if err != nil || len(data) > maxBroadcastCloneBytes {
		return nil, errBroadcastDataClone
	}
	return data, nil
}

func (e *broadcastCloneEncoder) encode(value goja.Value, depth int) (*broadcastCloneValue, error) {
	if depth > maxBroadcastCloneDepth {
		return nil, errBroadcastDataClone
	}
	switch {
	case goja.IsUndefined(value):
		return &broadcastCloneValue{Kind: "undefined"}, nil
	case goja.IsNull(value):
		return &broadcastCloneValue{Kind: "null"}, nil
	}
	if _, ok := value.(*goja.Symbol); ok {
		return nil, errBroadcastDataClone
	}
	if _, callable := goja.AssertFunction(value); callable {
		return nil, errBroadcastDataClone
	}
	if object, ok := value.(*goja.Object); ok {
		return e.encodeObject(object, depth)
	}
	switch exported := value.Export().(type) {
	case bool:
		return &broadcastCloneValue{Kind: "boolean", Bool: exported}, nil
	case string:
		return &broadcastCloneValue{Kind: "string", String: exported}, nil
	case int64:
		return &broadcastCloneValue{Kind: "number", String: strconv.FormatInt(exported, 10)}, nil
	case float64:
		var number string
		switch {
		case math.IsNaN(exported):
			number = "NaN"
		case math.IsInf(exported, 1):
			number = "+Infinity"
		case math.IsInf(exported, -1):
			number = "-Infinity"
		case exported == 0 && math.Signbit(exported):
			number = "-0"
		default:
			number = strconv.FormatFloat(exported, 'g', -1, 64)
		}
		return &broadcastCloneValue{Kind: "number", String: number}, nil
	case *big.Int:
		return &broadcastCloneValue{Kind: "bigint", String: exported.String()}, nil
	}
	return nil, errBroadcastDataClone
}

func (e *broadcastCloneEncoder) encodeObject(object *goja.Object, depth int) (*broadcastCloneValue, error) {
	if id, exists := e.seen[object]; exists {
		return &broadcastCloneValue{Kind: "ref", Ref: id}, nil
	}
	e.nodes++
	if e.nodes > maxBroadcastCloneNodes {
		return nil, errBroadcastDataClone
	}
	id := e.nodes
	e.seen[object] = id
	if file := e.r.fileValues[object]; file != nil {
		if len(file.data) > maxBroadcastCloneBytes {
			return nil, errBroadcastDataClone
		}
		kind := "blob"
		if file.isFile {
			kind = "file"
		}
		return &broadcastCloneValue{
			Kind: kind, ID: id, Bytes: base64.StdEncoding.EncodeToString(file.data),
			String: file.typeValue, Name: file.name, LastModified: file.lastModified,
		}, nil
	}
	if tag, err := e.objectTag(object); err == nil && tag == "[object DataView]" {
		return e.encodeDataView(object, id, depth)
	}
	switch object.ExportType() {
	case reflect.TypeOf([][2]interface{}{}):
		return e.encodeMap(object, id, depth)
	case reflect.TypeOf([]interface{}{}):
		if object.ClassName() == "Object" {
			return e.encodeSet(object, id, depth)
		}
	case reflect.TypeOf(goja.ArrayBuffer{}):
		buffer, ok := object.Export().(goja.ArrayBuffer)
		if !ok || buffer.Detached() {
			return nil, errBroadcastDataClone
		}
		return &broadcastCloneValue{Kind: "arraybuffer", ID: id, Bytes: base64.StdEncoding.EncodeToString(buffer.Bytes())}, nil
	case reflect.TypeOf([]byte{}), reflect.TypeOf([]int8{}), reflect.TypeOf([]int16{}), reflect.TypeOf([]uint16{}), reflect.TypeOf([]int32{}), reflect.TypeOf([]uint32{}), reflect.TypeOf([]int64{}), reflect.TypeOf([]uint64{}), reflect.TypeOf([]float32{}), reflect.TypeOf([]float64{}):
		return e.encodeArrayBufferView(object, id, depth)
	}
	switch object.ClassName() {
	case "Array":
		length := object.Get("length").ToInteger()
		if length < 0 || length > maxBroadcastCloneNodes {
			return nil, errBroadcastDataClone
		}
		cloned := &broadcastCloneValue{Kind: "array", ID: id, Items: make([]*broadcastCloneValue, int(length))}
		present := make(map[int]bool)
		for _, key := range object.GetOwnPropertyNames() {
			index, err := strconv.Atoi(key)
			if err == nil && index >= 0 && index < int(length) && strconv.Itoa(index) == key {
				present[index] = true
			}
		}
		for index := 0; index < int(length); index++ {
			if !present[index] {
				cloned.Items[index] = &broadcastCloneValue{Kind: "hole"}
				continue
			}
			item, err := e.encode(object.Get(strconv.Itoa(index)), depth+1)
			if err != nil {
				return nil, err
			}
			cloned.Items[index] = item
		}
		return cloned, nil
	case "Object":
		cloned := &broadcastCloneValue{Kind: "object", ID: id}
		for _, key := range object.Keys() {
			item, err := e.encode(object.Get(key), depth+1)
			if err != nil {
				return nil, err
			}
			cloned.Props = append(cloned.Props, broadcastCloneProp{Name: key, Value: item})
		}
		return cloned, nil
	case "Date":
		getTime, ok := goja.AssertFunction(object.Get("getTime"))
		if !ok {
			return nil, errBroadcastDataClone
		}
		timestamp, err := getTime(object)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		value, err := e.encode(timestamp, depth+1)
		if err != nil || value.Kind != "number" {
			return nil, errBroadcastDataClone
		}
		return &broadcastCloneValue{Kind: "date", ID: id, String: value.String}, nil
	case "RegExp":
		return &broadcastCloneValue{Kind: "regexp", ID: id, String: object.Get("source").String(), Bytes: object.Get("flags").String()}, nil
	default:
		return nil, errBroadcastDataClone
	}
}

func (e *broadcastCloneEncoder) encodeMap(object *goja.Object, id, depth int) (*broadcastCloneValue, error) {
	cloned := &broadcastCloneValue{Kind: "map", ID: id}
	var cloneErr error
	e.r.vm.ForOf(object, func(entry goja.Value) bool {
		pair := entry.ToObject(e.r.vm)
		key, err := e.encode(pair.Get("0"), depth+1)
		if err != nil {
			cloneErr = err
			return false
		}
		value, err := e.encode(pair.Get("1"), depth+1)
		if err != nil {
			cloneErr = err
			return false
		}
		cloned.Pairs = append(cloned.Pairs, broadcastClonePair{Key: key, Value: value})
		return true
	})
	if cloneErr != nil {
		return nil, cloneErr
	}
	return cloned, nil
}

func (e *broadcastCloneEncoder) encodeSet(object *goja.Object, id, depth int) (*broadcastCloneValue, error) {
	cloned := &broadcastCloneValue{Kind: "set", ID: id}
	var cloneErr error
	e.r.vm.ForOf(object, func(value goja.Value) bool {
		item, err := e.encode(value, depth+1)
		if err != nil {
			cloneErr = err
			return false
		}
		cloned.Items = append(cloned.Items, item)
		return true
	})
	if cloneErr != nil {
		return nil, cloneErr
	}
	return cloned, nil
}

func deserializeBroadcastMessage(r *Runtime, data []byte) (goja.Value, error) {
	var cloned broadcastCloneValue
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, errBroadcastDataClone
	}
	return decodeBroadcastCloneValue(r, &cloned, make(map[int]goja.Value))
}

func decodeBroadcastCloneValue(r *Runtime, cloned *broadcastCloneValue, objects map[int]goja.Value) (goja.Value, error) {
	if cloned == nil {
		return nil, errBroadcastDataClone
	}
	switch cloned.Kind {
	case "undefined":
		return goja.Undefined(), nil
	case "null":
		return goja.Null(), nil
	case "boolean":
		return r.vm.ToValue(cloned.Bool), nil
	case "string":
		return r.vm.ToValue(cloned.String), nil
	case "number":
		switch cloned.String {
		case "NaN":
			return goja.NaN(), nil
		case "+Infinity":
			return goja.PositiveInf(), nil
		case "-Infinity":
			return goja.NegativeInf(), nil
		case "-0":
			return r.vm.ToValue(math.Copysign(0, -1)), nil
		default:
			number, err := strconv.ParseFloat(cloned.String, 64)
			if err != nil {
				return nil, errBroadcastDataClone
			}
			return r.vm.ToValue(number), nil
		}
	case "bigint":
		bigInt, ok := new(big.Int).SetString(cloned.String, 10)
		if !ok {
			return nil, errBroadcastDataClone
		}
		return r.vm.ToValue(bigInt), nil
	case "arraybuffer":
		data, err := base64.StdEncoding.DecodeString(cloned.Bytes)
		if err != nil || len(data) > maxBroadcastCloneBytes {
			return nil, errBroadcastDataClone
		}
		value := r.vm.ToValue(r.vm.NewArrayBuffer(data))
		objects[cloned.ID] = value
		return value, nil
	case "typedarray":
		buffer, err := decodeBroadcastCloneValue(r, cloned.Buffer, objects)
		if err != nil || cloned.Offset < 0 || cloned.Length < 0 || cloned.Length > maxBroadcastCloneNodes {
			return nil, errBroadcastDataClone
		}
		view, err := r.vm.New(r.vm.Get(cloned.String), buffer, r.vm.ToValue(cloned.Offset), r.vm.ToValue(cloned.Length))
		if err != nil {
			return nil, errBroadcastDataClone
		}
		objects[cloned.ID] = view
		return view, nil
	case "dataview":
		buffer, err := decodeBroadcastCloneValue(r, cloned.Buffer, objects)
		if err != nil {
			return nil, err
		}
		length, err := strconv.Atoi(cloned.String)
		if err != nil || length < 0 || length > maxBroadcastCloneBytes {
			return nil, errBroadcastDataClone
		}
		view, err := r.vm.New(r.vm.Get("DataView"), buffer, r.vm.ToValue(cloned.Offset), r.vm.ToValue(length))
		if err != nil {
			return nil, errBroadcastDataClone
		}
		objects[cloned.ID] = view
		return view, nil
	case "blob", "file":
		data, err := base64.StdEncoding.DecodeString(cloned.Bytes)
		if err != nil || len(data) > maxBroadcastCloneBytes {
			return nil, errBroadcastDataClone
		}
		isFile := cloned.Kind == "file"
		value := r.makeBlob(r.vm.NewObject(), data, cloned.String, isFile, cloned.Name, cloned.LastModified)
		objects[cloned.ID] = value
		return value, nil
	case "ref":
		value, ok := objects[cloned.Ref]
		if !ok {
			return nil, errBroadcastDataClone
		}
		return value, nil
	case "array":
		array := r.vm.NewArray()
		objects[cloned.ID] = array
		for index, item := range cloned.Items {
			if item != nil && item.Kind == "hole" {
				continue
			}
			value, err := decodeBroadcastCloneValue(r, item, objects)
			if err != nil {
				return nil, err
			}
			if err := array.Set(strconv.Itoa(index), value); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		return array, nil
	case "object":
		object := r.vm.NewObject()
		objects[cloned.ID] = object
		for _, prop := range cloned.Props {
			value, err := decodeBroadcastCloneValue(r, prop.Value, objects)
			if err != nil {
				return nil, err
			}
			if err := object.DefineDataProperty(prop.Name, value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		return object, nil
	case "date":
		timestamp, err := decodeBroadcastCloneValue(r, &broadcastCloneValue{Kind: "number", String: cloned.String}, objects)
		if err != nil {
			return nil, err
		}
		date, err := r.vm.New(r.vm.Get("Date"), timestamp)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		objects[cloned.ID] = date
		return date, nil
	case "regexp":
		regexp, err := r.vm.New(r.vm.Get("RegExp"), r.vm.ToValue(cloned.String), r.vm.ToValue(cloned.Bytes))
		if err != nil {
			return nil, errBroadcastDataClone
		}
		objects[cloned.ID] = regexp
		return regexp, nil
	case "map":
		m, err := r.vm.New(r.vm.Get("Map"))
		if err != nil {
			return nil, errBroadcastDataClone
		}
		objects[cloned.ID] = m
		set, ok := goja.AssertFunction(m.Get("set"))
		if !ok {
			return nil, errBroadcastDataClone
		}
		for _, pair := range cloned.Pairs {
			key, err := decodeBroadcastCloneValue(r, pair.Key, objects)
			if err != nil {
				return nil, err
			}
			value, err := decodeBroadcastCloneValue(r, pair.Value, objects)
			if err != nil {
				return nil, err
			}
			if _, err := set(m, key, value); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		return m, nil
	case "set":
		setObject, err := r.vm.New(r.vm.Get("Set"))
		if err != nil {
			return nil, errBroadcastDataClone
		}
		objects[cloned.ID] = setObject
		add, ok := goja.AssertFunction(setObject.Get("add"))
		if !ok {
			return nil, errBroadcastDataClone
		}
		for _, item := range cloned.Items {
			value, err := decodeBroadcastCloneValue(r, item, objects)
			if err != nil {
				return nil, err
			}
			if _, err := add(setObject, value); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		return setObject, nil
	default:
		return nil, errBroadcastDataClone
	}
}

func (e *broadcastCloneEncoder) encodeArrayBufferView(object *goja.Object, id, depth int) (*broadcastCloneValue, error) {
	var name string
	switch object.ExportType() {
	case reflect.TypeOf([]int8{}):
		name = "Int8Array"
	case reflect.TypeOf([]int16{}):
		name = "Int16Array"
	case reflect.TypeOf([]uint16{}):
		name = "Uint16Array"
	case reflect.TypeOf([]int32{}):
		name = "Int32Array"
	case reflect.TypeOf([]uint32{}):
		name = "Uint32Array"
	case reflect.TypeOf([]int64{}):
		name = "BigInt64Array"
	case reflect.TypeOf([]uint64{}):
		name = "BigUint64Array"
	case reflect.TypeOf([]float32{}):
		name = "Float32Array"
	case reflect.TypeOf([]float64{}):
		name = "Float64Array"
	case reflect.TypeOf([]byte{}):
		tag, err := e.objectTag(object)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		name = strings.TrimSuffix(strings.TrimPrefix(tag, "[object "), "]")
		if name != "Uint8Array" && name != "Uint8ClampedArray" {
			return nil, errBroadcastDataClone
		}
	default:
		return nil, errBroadcastDataClone
	}
	length := object.Get("length").ToInteger()
	offset := object.Get("byteOffset").ToInteger()
	if length < 0 || length > maxBroadcastCloneNodes || offset < 0 || offset > maxBroadcastCloneBytes {
		return nil, errBroadcastDataClone
	}
	buffer, err := e.encode(object.Get("buffer"), depth+1)
	if err != nil {
		return nil, err
	}
	return &broadcastCloneValue{Kind: "typedarray", ID: id, String: name, Offset: int(offset), Length: int(length), Buffer: buffer}, nil
}

func (e *broadcastCloneEncoder) encodeDataView(object *goja.Object, id, depth int) (*broadcastCloneValue, error) {
	buffer, err := e.encode(object.Get("buffer"), depth+1)
	if err != nil {
		return nil, errBroadcastDataClone
	}
	length := object.Get("byteLength").ToInteger()
	offset := object.Get("byteOffset").ToInteger()
	if length < 0 || length > maxBroadcastCloneBytes || offset < 0 || offset > maxBroadcastCloneBytes {
		return nil, errBroadcastDataClone
	}
	return &broadcastCloneValue{Kind: "dataview", ID: id, String: strconv.FormatInt(length, 10), Offset: int(offset), Buffer: buffer}, nil
}

func (e *broadcastCloneEncoder) objectTag(object *goja.Object) (string, error) {
	objectConstructor, ok := e.r.vm.Get("Object").(*goja.Object)
	if !ok {
		return "", errBroadcastDataClone
	}
	prototype, ok := objectConstructor.Get("prototype").(*goja.Object)
	if !ok {
		return "", errBroadcastDataClone
	}
	toString, ok := goja.AssertFunction(prototype.Get("toString"))
	if !ok {
		return "", errBroadcastDataClone
	}
	tagValue, err := toString(object)
	if err != nil {
		return "", errBroadcastDataClone
	}
	return tagValue.String(), nil
}

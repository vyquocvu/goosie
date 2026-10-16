package js

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/dop251/goja"
)

// URL and URLSearchParams API implementation.

// setupURLAPI installs URL and URLSearchParams constructors.
func (r *Runtime) setupURLAPI() {
	// URLSearchParams constructor.
	spCtor := func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		// Use ordered entries to preserve insertion order and support sort().
		type entry struct{ key, val string }
		entries := make([]entry, 0)
		params := make(map[string][]string) // for fast lookup

		rebuild := func() {
			parts := make([]string, 0, len(entries))
			for _, e := range entries {
				parts = append(parts, url.QueryEscape(e.key)+"="+url.QueryEscape(e.val))
			}
			_ = obj.Set("__string__", strings.Join(parts, "&"))
		}

		if len(call.Arguments) >= 1 {
			arg := call.Arguments[0]
			switch v := arg.Export().(type) {
			case string:
				if parsed, err := url.ParseQuery(v); err == nil {
					// ParseQuery returns a map, so we lose original order.
					// Sort keys for deterministic output.
					keys := make([]string, 0, len(parsed))
					for k := range parsed {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						for _, val := range parsed[k] {
							entries = append(entries, entry{k, val})
							params[k] = append(params[k], val)
						}
					}
				}
			case map[string]interface{}:
				keys := make([]string, 0, len(v))
				for k := range v {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					val := fmt.Sprintf("%v", v[k])
					entries = append(entries, entry{k, val})
					params[k] = []string{val}
				}
			}
		}
		rebuild()

		_ = obj.Set("append", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			key := call.Arguments[0].String()
			val := call.Arguments[1].String()
			entries = append(entries, entry{key, val})
			params[key] = append(params[key], val)
			rebuild()
			return goja.Undefined()
		})

		_ = obj.Set("set", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			key := call.Arguments[0].String()
			val := call.Arguments[1].String()
			// Remove existing entries with this key.
			filtered := make([]entry, 0, len(entries))
			for _, e := range entries {
				if e.key != key {
					filtered = append(filtered, e)
				}
			}
			entries = append(filtered, entry{key, val})
			params[key] = []string{val}
			rebuild()
			return goja.Undefined()
		})

		_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Null()
			}
			key := call.Arguments[0].String()
			vals, ok := params[key]
			if !ok || len(vals) == 0 {
				return goja.Null()
			}
			return r.vm.ToValue(vals[0])
		})

		_ = obj.Set("getAll", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return r.vm.NewArray()
			}
			key := call.Arguments[0].String()
			vals := params[key]
			arr := r.vm.NewArray()
			for i, v := range vals {
				_ = arr.Set(fmt.Sprintf("%d", i), v)
			}
			return arr
		})

		_ = obj.Set("has", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return r.vm.ToValue(false)
			}
			key := call.Arguments[0].String()
			_, ok := params[key]
			return r.vm.ToValue(ok)
		})

		_ = obj.Set("delete", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			key := call.Arguments[0].String()
			delete(params, key)
			filtered := make([]entry, 0, len(entries))
			for _, e := range entries {
				if e.key != key {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
			rebuild()
			return goja.Undefined()
		})

		_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
			s := obj.Get("__string__")
			if s == nil || goja.IsUndefined(s) {
				return r.vm.ToValue("")
			}
			return r.vm.ToValue(s.String())
		})

		_ = obj.Set("forEach", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return goja.Undefined()
			}
			fn, ok := goja.AssertFunction(call.Arguments[0])
			if !ok {
				return goja.Undefined()
			}
			for _, e := range entries {
				_, _ = fn(goja.Undefined(), r.vm.ToValue(e.val), r.vm.ToValue(e.key), obj)
			}
			return goja.Undefined()
		})

		_ = obj.Set("keys", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			for i, e := range entries {
				_ = arr.Set(fmt.Sprintf("%d", i), e.key)
			}
			return arr
		})

		_ = obj.Set("values", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			for i, e := range entries {
				_ = arr.Set(fmt.Sprintf("%d", i), e.val)
			}
			return arr
		})

		_ = obj.Set("entries", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			for i, e := range entries {
				entry := r.vm.NewArray()
				_ = entry.Set("0", e.key)
				_ = entry.Set("1", e.val)
				_ = arr.Set(fmt.Sprintf("%d", i), entry)
			}
			return arr
		})

		_ = obj.Set("sort", func(call goja.FunctionCall) goja.Value {
			// Sort entries by key, preserving relative order for equal keys.
			sort.SliceStable(entries, func(i, j int) bool {
				return entries[i].key < entries[j].key
			})
			rebuild()
			return goja.Undefined()
		})

		return obj
	}

	spVal := r.vm.ToValue(spCtor)
	_ = r.vm.Set("URLSearchParams", spVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("URLSearchParams", spVal)
		}
	}

	// URL constructor.
	urlCtor := func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("URL requires a url argument"))
		}

		input := call.Arguments[0].String()
		var base string
		if len(call.Arguments) >= 2 {
			base = call.Arguments[1].String()
		}

		var parsed *url.URL
		var err error

		if base != "" {
			baseURL, berr := url.Parse(base)
			if berr != nil {
				panic(r.vm.NewTypeError("Invalid base URL: " + berr.Error()))
			}
			parsed, err = baseURL.Parse(input)
		} else {
			parsed, err = url.Parse(input)
		}

		if err != nil {
			panic(r.vm.NewTypeError("Invalid URL: " + err.Error()))
		}

		obj := call.This

		_ = obj.Set("href", parsed.String())
		_ = obj.Set("protocol", parsed.Scheme+":")
		_ = obj.Set("host", parsed.Host)
		_ = obj.Set("hostname", parsed.Hostname())

		port := parsed.Port()
		_ = obj.Set("port", port)

		_ = obj.Set("pathname", parsed.Path)
		if parsed.Path == "" {
			_ = obj.Set("pathname", "/")
		}

		_ = obj.Set("search", parsed.RawQuery)
		if parsed.RawQuery != "" && !strings.HasPrefix(parsed.RawQuery, "?") {
			_ = obj.Set("search", "?"+parsed.RawQuery)
		}

		_ = obj.Set("hash", parsed.Fragment)
		if parsed.Fragment != "" && !strings.HasPrefix(parsed.Fragment, "#") {
			_ = obj.Set("hash", "#"+parsed.Fragment)
		}

		username := parsed.User.Username()
		_ = obj.Set("username", username)
		password, hasPassword := parsed.User.Password()
		_ = obj.Set("password", password)
		if !hasPassword {
			_ = obj.Set("password", "")
		}

		_ = obj.Set("origin", parsed.Scheme+"://"+parsed.Host)

		// searchParams property.
		spCall := goja.ConstructorCall{This: r.vm.NewObject()}
		if parsed.RawQuery != "" {
			spCall.Arguments = []goja.Value{r.vm.ToValue(parsed.RawQuery)}
		}
		spObj := spCtor(spCall)
		_ = obj.Set("searchParams", spObj)

		// toString method.
		_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
			return r.vm.ToValue(obj.Get("href").String())
		})

		// toJSON method.
		_ = obj.Set("toJSON", func(call goja.FunctionCall) goja.Value {
			return r.vm.ToValue(obj.Get("href").String())
		})

		return nil
	}

	urlVal := r.vm.ToValue(urlCtor)
	_ = r.vm.Set("URL", urlVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("URL", urlVal)
		}
	}
}

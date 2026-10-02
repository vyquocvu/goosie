package js

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dop251/goja"
)

// URL and URLSearchParams API implementation.

// setupURLAPI installs URL and URLSearchParams constructors.
func (r *Runtime) setupURLAPI() {
	// URLSearchParams constructor.
	spCtor := func(call goja.ConstructorCall) *goja.Object {
		obj := call.This
		params := make(map[string][]string)

		if len(call.Arguments) >= 1 {
			arg := call.Arguments[0]
			switch v := arg.Export().(type) {
			case string:
				if parsed, err := url.ParseQuery(v); err == nil {
					for k, vals := range parsed {
						params[k] = vals
					}
				}
			case map[string]interface{}:
				for k, val := range v {
					params[k] = []string{fmt.Sprintf("%v", val)}
				}
			}
		}

		rebuild := func() {
			parts := make([]string, 0)
			for k, vals := range params {
				for _, v := range vals {
					parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
				}
			}
			_ = obj.Set("__string__", strings.Join(parts, "&"))
		}
		rebuild()

		_ = obj.Set("append", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			key := call.Arguments[0].String()
			val := call.Arguments[1].String()
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
			for k, vals := range params {
				for _, v := range vals {
					_, _ = fn(goja.Undefined(), r.vm.ToValue(v), r.vm.ToValue(k), obj)
				}
			}
			return goja.Undefined()
		})

		_ = obj.Set("keys", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			i := 0
			for k := range params {
				_ = arr.Set(fmt.Sprintf("%d", i), k)
				i++
			}
			return arr
		})

		_ = obj.Set("values", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			i := 0
			for _, vals := range params {
				for _, v := range vals {
					_ = arr.Set(fmt.Sprintf("%d", i), v)
					i++
				}
			}
			return arr
		})

		_ = obj.Set("entries", func(call goja.FunctionCall) goja.Value {
			arr := r.vm.NewArray()
			i := 0
			for k, vals := range params {
				for _, v := range vals {
					entry := r.vm.NewArray()
					_ = entry.Set("0", k)
					_ = entry.Set("1", v)
					_ = arr.Set(fmt.Sprintf("%d", i), entry)
					i++
				}
			}
			return arr
		})

		_ = obj.Set("sort", func(call goja.FunctionCall) goja.Value {
			sorted := make(map[string][]string)
			keys := make([]string, 0, len(params))
			for k := range params {
				keys = append(keys, k)
			}
			for j := 0; j < len(keys)-1; j++ {
				for k := j + 1; k < len(keys); k++ {
					if keys[j] > keys[k] {
						keys[j], keys[k] = keys[k], keys[j]
					}
				}
			}
			for _, k := range keys {
				sorted[k] = params[k]
			}
			for k := range params {
				delete(params, k)
			}
			for k, v := range sorted {
				params[k] = v
			}
			rebuild()
			return goja.Undefined()
		})

		return nil
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

package js

import (
	"time"

	"github.com/dop251/goja"
)

// setupPerformanceAPI installs the Performance API on window and global scope.
func (r *Runtime) setupPerformanceAPI() {
	startTime := time.Now()
	nowFn := func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(float64(time.Since(startTime).Milliseconds()))
	}

	perf := r.vm.NewObject()
	perf.Set("now", nowFn)
	perf.Set("timeOrigin", float64(startTime.UnixNano())/1e6)

	timing := r.vm.NewObject()
	startMs := float64(startTime.UnixNano()) / 1e6
	timing.Set("navigationStart", startMs)
	timing.Set("fetchStart", startMs)
	timing.Set("responseStart", startMs)
	timing.Set("responseEnd", startMs)
	timing.Set("domLoading", startMs)
	timing.Set("domInteractive", startMs)
	timing.Set("domContentLoadedEventStart", startMs)
	timing.Set("domContentLoadedEventEnd", startMs)
	timing.Set("domComplete", startMs)
	timing.Set("loadEventStart", startMs)
	timing.Set("loadEventEnd", startMs)
	perf.Set("timing", timing)

	emptyArrayFn := func(goja.FunctionCall) goja.Value {
		return r.vm.NewArray()
	}
	perf.Set("getEntriesByType", emptyArrayFn)
	perf.Set("getEntriesByName", emptyArrayFn)
	perf.Set("getEntries", emptyArrayFn)

	noopFn := func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}
	perf.Set("mark", noopFn)
	perf.Set("measure", noopFn)
	perf.Set("clearMarks", noopFn)
	perf.Set("clearMeasures", noopFn)

	r.vm.Set("performance", perf)
	r.vm.GlobalObject().Set("performance", perf)
}

// setupFontsAPI installs document.fonts stub.
func (r *Runtime) setupFontsAPI() {
	script := `
	(function() {
		if (typeof document !== 'undefined') {
			document.fonts = {
				load: function() { return Promise.resolve([]); },
				check: function() { return true; },
				ready: Promise.resolve(),
				addEventListener: function() {},
				removeEventListener: function() {}
			};
		}
	})();
	`
	_, _ = r.vm.RunString(script)
}

// setupXHRAPI installs XMLHttpRequest constructor stub.
func (r *Runtime) setupXHRAPI() {
	script := `
	(function() {
		class XMLHttpRequest {
			constructor() {
				this.readyState = 0;
				this.status = 0;
				this.statusText = "";
				this.responseText = "";
				this.response = "";
				this.onreadystatechange = null;
				this.onload = null;
				this.onerror = null;
				this.headers = {};
			}
			open(method, url, async) {
				this.method = method;
				this.url = url;
				this.readyState = 1;
				if (this.onreadystatechange) this.onreadystatechange();
			}
			setRequestHeader(k, v) {
				this.headers[k] = v;
			}
			send(body) {
				this.readyState = 4;
				this.status = 200;
				this.statusText = "OK";
				if (this.onreadystatechange) this.onreadystatechange();
				if (this.onload) this.onload();
			}
			abort() {}
			getResponseHeader(name) { return null; }
			getAllResponseHeaders() { return ""; }
		}
		window.XMLHttpRequest = XMLHttpRequest;
		globalThis.XMLHttpRequest = XMLHttpRequest;
	})();
	`
	_, _ = r.vm.RunString(script)
}

// setupWebCompat stubs common site-specific globals that prevent script execution errors.
func (r *Runtime) setupWebCompat() {
	script := `
	(function() {
		window.google = window.google || {};
		window.google.c = window.google.c || {};
		window.google.c.maft = window.google.c.maft || function() {};
	})();
	`
	_, _ = r.vm.RunString(script)
}

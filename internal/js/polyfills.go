package js

// polyfillsJS is injected into the Goja runtime before any page scripts.
const polyfillsJS = `
(function(global) {
  'use strict';

  // ─── queueMicrotask ──────────────────────────────────────────────────────
  var _microtaskQueue = [];
  global.queueMicrotask = function(fn) { _microtaskQueue.push(fn); };
  global.__flushMicrotasks = function() {
    while (_microtaskQueue.length > 0) {
      var tasks = _microtaskQueue.splice(0);
      for (var i = 0; i < tasks.length; i++) {
        try { tasks[i](); } catch(e) { console.error(e); }
      }
    }
  };

  // ─── Promise ─────────────────────────────────────────────────────────────
  function Promise(executor) {
    this._state = 'pending';
    this._value = undefined;
    this._callbacks = [];
    var self = this;
    function resolve(val) {
      if (self._state !== 'pending') return;
      if (val && typeof val.then === 'function') { val.then(resolve, reject); return; }
      self._state = 'fulfilled';
      self._value = val;
      queueMicrotask(function() {
        self._callbacks.forEach(function(cb) { if (cb.onFulfilled) cb.onFulfilled(self._value); });
      });
    }
    function reject(reason) {
      if (self._state !== 'pending') return;
      self._state = 'rejected';
      self._value = reason;
      queueMicrotask(function() {
        self._callbacks.forEach(function(cb) { if (cb.onRejected) cb.onRejected(self._value); });
      });
    }
    try { executor(resolve, reject); } catch(e) { reject(e); }
  }
  Promise.prototype.then = function(onFulfilled, onRejected) {
    var self = this;
    return new Promise(function(resolve, reject) {
      function handle(fn, val, fallback) {
        if (typeof fn !== 'function') { fallback(val); return; }
        queueMicrotask(function() { try { resolve(fn(val)); } catch(e) { reject(e); } });
      }
      if (self._state === 'fulfilled') {
        handle(onFulfilled, self._value, resolve);
      } else if (self._state === 'rejected') {
        handle(onRejected, self._value, reject);
      } else {
        self._callbacks.push({
          onFulfilled: function(v) { handle(onFulfilled, v, resolve); },
          onRejected:  function(r) { handle(onRejected, r, reject); }
        });
      }
    });
  };
  Promise.prototype.catch   = function(fn) { return this.then(undefined, fn); };
  Promise.prototype.finally = function(fn) {
    return this.then(
      function(v) { return Promise.resolve(fn()).then(function() { return v; }); },
      function(r) { return Promise.resolve(fn()).then(function() { throw r; }); }
    );
  };
  Promise.resolve = function(val) {
    if (val instanceof Promise) return val;
    return new Promise(function(res) { res(val); });
  };
  Promise.reject = function(reason) { return new Promise(function(_, rej) { rej(reason); }); };
  Promise.all = function(promises) {
    return new Promise(function(resolve, reject) {
      var results = [], remaining = promises.length;
      if (remaining === 0) { resolve(results); return; }
      promises.forEach(function(p, i) {
        Promise.resolve(p).then(function(v) { results[i] = v; if (--remaining === 0) resolve(results); }, reject);
      });
    });
  };
  Promise.allSettled = function(promises) {
    return Promise.all(promises.map(function(p) {
      return Promise.resolve(p).then(
        function(v) { return { status: 'fulfilled', value: v }; },
        function(r) { return { status: 'rejected', reason: r }; }
      );
    }));
  };
  Promise.race = function(promises) {
    return new Promise(function(resolve, reject) {
      promises.forEach(function(p) { Promise.resolve(p).then(resolve, reject); });
    });
  };
  Promise.any = function(promises) {
    return new Promise(function(resolve, reject) {
      var errors = [], remaining = promises.length;
      if (remaining === 0) { reject(new Error('All promises rejected')); return; }
      promises.forEach(function(p, i) {
        Promise.resolve(p).then(resolve, function(e) {
          errors[i] = e;
          if (--remaining === 0) reject(new Error('All promises rejected'));
        });
      });
    });
  };
  global.Promise = Promise;

  // ─── Map ─────────────────────────────────────────────────────────────────
  function Map(iterable) {
    this._keys = []; this._vals = [];
    if (iterable) for (var i = 0; i < iterable.length; i++) this.set(iterable[i][0], iterable[i][1]);
  }
  Map.prototype.set = function(k, v) {
    var i = this._keys.indexOf(k);
    if (i === -1) { this._keys.push(k); this._vals.push(v); } else { this._vals[i] = v; }
    return this;
  };
  Map.prototype.get    = function(k) { var i = this._keys.indexOf(k); return i === -1 ? undefined : this._vals[i]; };
  Map.prototype.has    = function(k) { return this._keys.indexOf(k) !== -1; };
  Map.prototype.delete = function(k) { var i = this._keys.indexOf(k); if (i !== -1) { this._keys.splice(i,1); this._vals.splice(i,1); return true; } return false; };
  Map.prototype.clear  = function() { this._keys = []; this._vals = []; };
  Object.defineProperty(Map.prototype, 'size', { get: function() { return this._keys.length; } });
  Map.prototype.forEach = function(fn) { for (var i=0;i<this._keys.length;i++) fn(this._vals[i], this._keys[i], this); };
  Map.prototype.keys    = function() { return this._keys.slice(); };
  Map.prototype.values  = function() { return this._vals.slice(); };
  Map.prototype.entries = function() { return this._keys.map(function(k,i) { return [k, this._vals[i]]; }, this); };
  global.Map = Map;

  // ─── Set ─────────────────────────────────────────────────────────────────
  function Set(iterable) {
    this._items = [];
    if (iterable) for (var i=0;i<iterable.length;i++) this.add(iterable[i]);
  }
  Set.prototype.add    = function(v) { if (this._items.indexOf(v) === -1) this._items.push(v); return this; };
  Set.prototype.has    = function(v) { return this._items.indexOf(v) !== -1; };
  Set.prototype.delete = function(v) { var i=this._items.indexOf(v); if(i!==-1){this._items.splice(i,1);return true;} return false; };
  Set.prototype.clear  = function() { this._items = []; };
  Object.defineProperty(Set.prototype, 'size', { get: function() { return this._items.length; } });
  Set.prototype.forEach = function(fn) { this._items.forEach(function(v) { fn(v,v,this); },this); };
  Set.prototype.values  = function() { return this._items.slice(); };
  Set.prototype.keys    = Set.prototype.values;
  Set.prototype.entries = function() { return this._items.map(function(v){return [v,v];}); };
  global.Set = Set;

  // WeakMap/WeakSet — no GC semantics but API-compatible
  global.WeakMap = Map;
  global.WeakSet = Set;

  // ─── Object methods ───────────────────────────────────────────────────────
  if (!Object.assign) Object.assign = function(target) {
    for (var i=1;i<arguments.length;i++) { var s=arguments[i]; if(s) for(var k in s) if(Object.prototype.hasOwnProperty.call(s,k)) target[k]=s[k]; }
    return target;
  };
  if (!Object.entries) Object.entries = function(o) { return Object.keys(o).map(function(k){return[k,o[k]];}); };
  if (!Object.values)  Object.values  = function(o) { return Object.keys(o).map(function(k){return o[k];}); };
  if (!Object.fromEntries) Object.fromEntries = function(e) { var r={}; for(var i=0;i<e.length;i++) r[e[i][0]]=e[i][1]; return r; };
  if (!Object.is) Object.is = function(a,b){ return a===b||(a!==a&&b!==b); };

  // ─── Array methods ────────────────────────────────────────────────────────
  if (!Array.from) Array.from = function(it,fn) { var a=[]; for(var i=0;i<it.length;i++) a.push(fn?fn(it[i],i):it[i]); return a; };
  var ap = Array.prototype;
  if (!ap.find)      ap.find      = function(fn){ for(var i=0;i<this.length;i++) if(fn(this[i],i,this)) return this[i]; };
  if (!ap.findIndex) ap.findIndex = function(fn){ for(var i=0;i<this.length;i++) if(fn(this[i],i,this)) return i; return -1; };
  if (!ap.includes)  ap.includes  = function(v){ return this.indexOf(v)!==-1; };
  if (!ap.flat) ap.flat = function(d){ d=d===undefined?1:d; function f(a,n){ return a.reduce(function(acc,v){return acc.concat(Array.isArray(v)&&n>0?f(v,n-1):v);},[]); } return f(this,d); };
  if (!ap.flatMap)   ap.flatMap   = function(fn){ return this.map(fn).flat(1); };
  if (!ap.at)        ap.at        = function(i){ return i<0?this[this.length+i]:this[i]; };

  // ─── String methods ───────────────────────────────────────────────────────
  var sp = String.prototype;
  if (!sp.startsWith) sp.startsWith = function(s,p){ return this.indexOf(s,p||0)===(p||0); };
  if (!sp.endsWith)   sp.endsWith   = function(s){ return this.indexOf(s,this.length-s.length)!==-1; };
  if (!sp.includes)   sp.includes   = function(s,p){ return this.indexOf(s,p||0)!==-1; };
  if (!sp.padStart)   sp.padStart   = function(n,f){ f=f||' '; var s=String(this); while(s.length<n) s=f+s; return s.slice(s.length-Math.max(n,s.length)); };
  if (!sp.padEnd)     sp.padEnd     = function(n,f){ f=f||' '; var s=String(this); while(s.length<n) s+=f; return s.slice(0,n); };
  if (!sp.trimStart)  sp.trimStart  = function(){ return this.replace(/^\s+/,''); };
  if (!sp.trimEnd)    sp.trimEnd    = function(){ return this.replace(/\s+$/,''); };
  if (!sp.repeat)     sp.repeat     = function(n){ var s=''; for(var i=0;i<n;i++) s+=this; return s; };

  // ─── Globals ─────────────────────────────────────────────────────────────
  global.globalThis = global;
  global.structuredClone = function(v){ try{return JSON.parse(JSON.stringify(v));}catch(e){return v;} };

  // ─── Browser APIs ─────────────────────────────────────────────────────────

  // navigator
  global.navigator = {
    userAgent: "Goosie/1.0 (like Chrome/100.0)",
    language: "en-US",
    languages: ["en-US", "en"],
    cookieEnabled: true,
    onLine: true,
    platform: "GoosieOS",
    hardwareConcurrency: 4,
    deviceMemory: 8
  };

  // Base64
  var b64chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=';
  global.btoa = function(input) {
    var str = String(input);
    for (var block, charCode, idx = 0, map = b64chars, output = ''; str.charAt(idx | 0) || (map = '=', idx % 1); output += map.charAt(63 & block >> 8 - idx % 1 * 8)) {
      charCode = str.charCodeAt(idx += 3/4);
      if (charCode > 0xFF) throw new Error("'btoa' failed: The string to be encoded contains characters outside of the Latin1 range.");
      block = block << 8 | charCode;
    }
    return output;
  };
  global.atob = function(input) {
    var str = String(input).replace(/[=]+$/, '');
    if (str.length % 4 == 1) throw new Error("'atob' failed: The string to be decoded is not correctly encoded.");
    for (var bc = 0, bs, buffer, idx = 0, output = ''; buffer = str.charAt(idx++); ~buffer && (bs = bc % 4 ? bs * 64 + buffer : buffer, bc++ % 4) ? output += String.fromCharCode(255 & bs >> (-2 * bc & 6)) : 0) {
      buffer = b64chars.indexOf(buffer);
    }
    return output;
  };

  // Dialogs
  global.alert = function(msg) { console.log("ALERT: " + msg); };
  global.prompt = function(msg, defaultText) { console.log("PROMPT: " + msg); return defaultText || ""; };
  global.confirm = function(msg) { console.log("CONFIRM: " + msg); return true; };

  // URLSearchParams
  global.URLSearchParams = function(init) {
    this._params = [];
    if (typeof init === 'string') {
      if (init.startsWith('?')) init = init.slice(1);
      var pairs = init.split('&');
      for (var i = 0; i < pairs.length; i++) {
        if (!pairs[i]) continue;
        var idx = pairs[i].indexOf('=');
        if (idx === -1) this.append(decodeURIComponent(pairs[i]), '');
        else this.append(decodeURIComponent(pairs[i].slice(0, idx)), decodeURIComponent(pairs[i].slice(idx + 1)));
      }
    }
  };
  global.URLSearchParams.prototype.append = function(name, value) { this._params.push([String(name), String(value)]); };
  global.URLSearchParams.prototype.delete = function(name) {
    name = String(name);
    this._params = this._params.filter(function(p) { return p[0] !== name; });
  };
  global.URLSearchParams.prototype.get = function(name) {
    name = String(name);
    for (var i = 0; i < this._params.length; i++) if (this._params[i][0] === name) return this._params[i][1];
    return null;
  };
  global.URLSearchParams.prototype.getAll = function(name) {
    name = String(name);
    return this._params.filter(function(p) { return p[0] === name; }).map(function(p) { return p[1]; });
  };
  global.URLSearchParams.prototype.has = function(name) { return this.get(name) !== null; };
  global.URLSearchParams.prototype.set = function(name, value) {
    this.delete(name);
    this.append(name, value);
  };
  global.URLSearchParams.prototype.toString = function() {
    return this._params.map(function(p) { return encodeURIComponent(p[0]) + '=' + encodeURIComponent(p[1]); }).join('&');
  };

  // FormData (Mock)
  global.FormData = function(form) {
    this._data = [];
    // Note: populating from HTMLFormElement is not fully implemented in this mock
  };
  global.FormData.prototype.append = function(name, value) { this._data.push([name, value]); };
  global.FormData.prototype.delete = function(name) { this._data = this._data.filter(function(p) { return p[0] !== name; }); };
  global.FormData.prototype.get = function(name) {
    for (var i = 0; i < this._data.length; i++) if (this._data[i][0] === name) return this._data[i][1];
    return null;
  };
  global.FormData.prototype.getAll = function(name) {
    return this._data.filter(function(p) { return p[0] === name; }).map(function(p) { return p[1]; });
  };
  global.FormData.prototype.has = function(name) { return this.get(name) !== null; };
  global.FormData.prototype.set = function(name, value) { this.delete(name); this.append(name, value); };

  // URL (basic implementation based on document.createElement('a'))
  global.URL = function(url, base) {
    if (base) {
      if (base.endsWith('/')) url = base + url.replace(/^\//, '');
      else url = base + '/' + url.replace(/^\//, '');
    }
    this.href = url;
    // Very basic parsing for properties
    var match = url.match(/^(https?:)\/\/([^\/:]+)(:\d+)?(\/[^?]*)?(\?[^#]*)?(#.*)?$/);
    if (match) {
      this.protocol = match[1] || "";
      this.hostname = match[2] || "";
      this.port = (match[3] || "").substring(1);
      this.host = this.hostname + (this.port ? ":" + this.port : "");
      this.pathname = match[4] || "/";
      this.search = match[5] || "";
      this.hash = match[6] || "";
      this.searchParams = new global.URLSearchParams(this.search);
    } else {
      this.pathname = url;
    }
  };
  global.URL.prototype.toString = function() { return this.href; };

  // Headers (Mock)
  global.Headers = function(init) {
    this._map = {};
    if (init instanceof global.Headers) {
      var self = this;
      init.forEach(function(v, k) { self.append(k, v); });
    } else if (init && typeof init === 'object') {
      for (var k in init) this.append(k, init[k]);
    }
  };
  global.Headers.prototype.append = function(n, v) { var k = n.toLowerCase(); this._map[k] = this._map[k] ? this._map[k] + ', ' + v : v; };
  global.Headers.prototype.delete = function(n) { delete this._map[n.toLowerCase()]; };
  global.Headers.prototype.get = function(n) { return this._map[n.toLowerCase()] || null; };
  global.Headers.prototype.has = function(n) { return this._map[n.toLowerCase()] !== undefined; };
  global.Headers.prototype.set = function(n, v) { this._map[n.toLowerCase()] = v; };
  global.Headers.prototype.forEach = function(cb, thisArg) {
    for (var k in this._map) cb.call(thisArg, this._map[k], k, this);
  };

  // Blob / File / FileReader (Stubs)
  global.Blob = function(parts, options) {
    this.size = parts ? parts.join('').length : 0;
    this.type = options && options.type ? options.type : "";
  };
  global.File = function(parts, filename, options) {
    global.Blob.call(this, parts, options);
    this.name = filename;
  };
  global.FileReader = function() {};
  global.FileReader.prototype.readAsText = function(blob) {
    var self = this;
    setTimeout(function() {
      self.result = "[Blob Data]";
      if (self.onload) self.onload({ target: self });
    }, 10);
  };

  // Image
  global.Image = function(w, h) {
    if (typeof document !== "undefined") {
      var img = document.createElement("img");
      if (w !== undefined) img.width = w;
      if (h !== undefined) img.height = h;
      return img;
    }
    return { width: w, height: h };
  };

  // ─── Browser APIs ─────────────────────────────────────────────────────────

  // navigator
  global.navigator = {
    userAgent: "Goosie/1.0 (like Chrome/100.0)",
    language: "en-US",
    languages: ["en-US", "en"],
    cookieEnabled: true,
    onLine: true,
    platform: "GoosieOS",
    hardwareConcurrency: 4,
    deviceMemory: 8
  };

  // Base64
  var b64chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=';
  global.btoa = function(input) {
    var str = String(input);
    for (var block, charCode, idx = 0, map = b64chars, output = ''; str.charAt(idx | 0) || (map = '=', idx % 1); output += map.charAt(63 & block >> 8 - idx % 1 * 8)) {
      charCode = str.charCodeAt(idx += 3/4);
      if (charCode > 0xFF) throw new Error("'btoa' failed: The string to be encoded contains characters outside of the Latin1 range.");
      block = block << 8 | charCode;
    }
    return output;
  };
  global.atob = function(input) {
    var str = String(input).replace(/[=]+$/, '');
    if (str.length % 4 == 1) throw new Error("'atob' failed: The string to be decoded is not correctly encoded.");
    for (var bc = 0, bs, buffer, idx = 0, output = ''; buffer = str.charAt(idx++); ~buffer && (bs = bc % 4 ? bs * 64 + buffer : buffer, bc++ % 4) ? output += String.fromCharCode(255 & bs >> (-2 * bc & 6)) : 0) {
      buffer = b64chars.indexOf(buffer);
    }
    return output;
  };

  // Dialogs
  global.alert = function(msg) { console.log("ALERT: " + msg); };
  global.prompt = function(msg, defaultText) { console.log("PROMPT: " + msg); return defaultText || ""; };
  global.confirm = function(msg) { console.log("CONFIRM: " + msg); return true; };

  // URLSearchParams
  global.URLSearchParams = function(init) {
    this._params = [];
    if (typeof init === 'string') {
      if (init.startsWith('?')) init = init.slice(1);
      var pairs = init.split('&');
      for (var i = 0; i < pairs.length; i++) {
        if (!pairs[i]) continue;
        var idx = pairs[i].indexOf('=');
        if (idx === -1) this.append(decodeURIComponent(pairs[i]), '');
        else this.append(decodeURIComponent(pairs[i].slice(0, idx)), decodeURIComponent(pairs[i].slice(idx + 1)));
      }
    }
  };
  global.URLSearchParams.prototype.append = function(name, value) { this._params.push([String(name), String(value)]); };
  global.URLSearchParams.prototype.delete = function(name) {
    name = String(name);
    this._params = this._params.filter(function(p) { return p[0] !== name; });
  };
  global.URLSearchParams.prototype.get = function(name) {
    name = String(name);
    for (var i = 0; i < this._params.length; i++) if (this._params[i][0] === name) return this._params[i][1];
    return null;
  };
  global.URLSearchParams.prototype.getAll = function(name) {
    name = String(name);
    return this._params.filter(function(p) { return p[0] === name; }).map(function(p) { return p[1]; });
  };
  global.URLSearchParams.prototype.has = function(name) { return this.get(name) !== null; };
  global.URLSearchParams.prototype.set = function(name, value) {
    this.delete(name);
    this.append(name, value);
  };
  global.URLSearchParams.prototype.toString = function() {
    return this._params.map(function(p) { return encodeURIComponent(p[0]) + '=' + encodeURIComponent(p[1]); }).join('&');
  };

  // FormData (Mock)
  global.FormData = function(form) {
    this._data = [];
    // Note: populating from HTMLFormElement is not fully implemented in this mock
  };
  global.FormData.prototype.append = function(name, value) { this._data.push([name, value]); };
  global.FormData.prototype.delete = function(name) { this._data = this._data.filter(function(p) { return p[0] !== name; }); };
  global.FormData.prototype.get = function(name) {
    for (var i = 0; i < this._data.length; i++) if (this._data[i][0] === name) return this._data[i][1];
    return null;
  };
  global.FormData.prototype.getAll = function(name) {
    return this._data.filter(function(p) { return p[0] === name; }).map(function(p) { return p[1]; });
  };
  global.FormData.prototype.has = function(name) { return this.get(name) !== null; };
  global.FormData.prototype.set = function(name, value) { this.delete(name); this.append(name, value); };

  // URL (basic implementation based on document.createElement('a'))
  global.URL = function(url, base) {
    if (base) {
      if (base.endsWith('/')) url = base + url.replace(/^\//, '');
      else url = base + '/' + url.replace(/^\//, '');
    }
    this.href = url;
    // Very basic parsing for properties
    var match = url.match(/^(https?:)\/\/([^\/:]+)(:\d+)?(\/[^?]*)?(\?[^#]*)?(#.*)?$/);
    if (match) {
      this.protocol = match[1] || "";
      this.hostname = match[2] || "";
      this.port = (match[3] || "").substring(1);
      this.host = this.hostname + (this.port ? ":" + this.port : "");
      this.pathname = match[4] || "/";
      this.search = match[5] || "";
      this.hash = match[6] || "";
      this.searchParams = new global.URLSearchParams(this.search);
    } else {
      this.pathname = url;
    }
  };
  global.URL.prototype.toString = function() { return this.href; };

  // Headers (Mock)
  global.Headers = function(init) {
    this._map = {};
    if (init instanceof global.Headers) {
      var self = this;
      init.forEach(function(v, k) { self.append(k, v); });
    } else if (init && typeof init === 'object') {
      for (var k in init) this.append(k, init[k]);
    }
  };
  global.Headers.prototype.append = function(n, v) { var k = n.toLowerCase(); this._map[k] = this._map[k] ? this._map[k] + ', ' + v : v; };
  global.Headers.prototype.delete = function(n) { delete this._map[n.toLowerCase()]; };
  global.Headers.prototype.get = function(n) { return this._map[n.toLowerCase()] || null; };
  global.Headers.prototype.has = function(n) { return this._map[n.toLowerCase()] !== undefined; };
  global.Headers.prototype.set = function(n, v) { this._map[n.toLowerCase()] = v; };
  global.Headers.prototype.forEach = function(cb, thisArg) {
    for (var k in this._map) cb.call(thisArg, this._map[k], k, this);
  };

  // Blob / File / FileReader (Stubs)
  global.Blob = function(parts, options) {
    this.size = parts ? parts.join('').length : 0;
    this.type = options && options.type ? options.type : "";
  };
  global.File = function(parts, filename, options) {
    global.Blob.call(this, parts, options);
    this.name = filename;
  };
  global.FileReader = function() {};
  global.FileReader.prototype.readAsText = function(blob) {
    var self = this;
    setTimeout(function() {
      self.result = "[Blob Data]";
      if (self.onload) self.onload({ target: self });
    }, 10);
  };

  // Image
  global.Image = function(w, h) {
    if (typeof document !== "undefined") {
      var img = document.createElement("img");
      if (w !== undefined) img.width = w;
      if (h !== undefined) img.height = h;
      return img;
    }
    return { width: w, height: h };
  };

})(this);
`

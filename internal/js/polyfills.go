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

})(this);
`

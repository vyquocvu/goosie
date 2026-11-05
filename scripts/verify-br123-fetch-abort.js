// BR-123 anchor: what does an aborted fetch actually reject with?
//
// The catalog claims fetch honours RequestInit.signal, and the open defect
// GOOSIE-FETCH-ABORT-001 claims the opposite. Both cannot be true, and neither
// may be settled from memory: this script drives a real Chromium against a
// gated same-origin server and prints, verbatim, every value the page can see.
//
// Sections:
//   1. abort() mid-flight - the rejection value's whole shape (typeof,
//      constructor.name, name, message, String(err), instanceof DOMException /
//      Error), plus whether the request reached the server.
//   2. abort('stopped'), abort({code:7}), abort(new Error('boom')), abort(0),
//      abort(null) - does the reason arrive by identity, or stringified?
//   3. an already-aborted signal - server hits and rejection shape.
//   4. the late response - after the abort, does any handler ever run?
//   5. signal.reason before and after abort().
//   6. non-signal `signal` values: 'abc', {}, 7, true, null, undefined - and
//      whether the TypeError is thrown synchronously or delivered as a
//      rejection.
//   7. the DOMException object itself (keys, stack, tag, prototype chain) and
//      abort(undefined).
//   8. AbortSignal.abort() / AbortSignal.timeout(ms) reasons, and a fetch cut
//      short by a timeout.
//   9. a live, never-aborted signal (regression).
//  10. transport-level cancellation - whether the server sees the socket torn
//      down when the page aborts, or only the promise settling.
//
// Run: node scripts/verify-br123-fetch-abort.js
//      (needs playwright chromium: `npm i -D playwright && npx playwright install chromium`)
const { chromium } = require('playwright');
const http = require('http');

// A gate-blocked server: /slow records the hit and holds the response until the
// gate opens, so an abort can be observed while the request is still in flight.
// /fast answers immediately, for the cases that must actually resolve.
const held = new Map();
const hits = {};
const routes = { '/': '<!doctype html><meta charset="utf-8"><title>br123</title><body>ok' };

const server = http.createServer((req, res) => {
  const key = req.url.split('?')[0];
  hits[key] = (hits[key] || 0) + 1;
  if (key === '/slow') {
    held.set(res, true);
    return;
  }
  if (key === '/fast') {
    res.writeHead(200, { 'content-type': 'text/plain' });
    res.end('fast');
    return;
  }
  res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
  res.end(routes[key] || 'not found');
});

function openGates() {
  const pending = [...held.keys()];
  held.clear();
  for (const res of pending) {
    res.writeHead(200, { 'content-type': 'text/plain' });
    res.end('late');
  }
}

// Shape of any value delivered to a page handler, as the page can read it.
const SHAPE = `(function(v){
  var out = {typeof: typeof v, str: String(v), json: null, ctor: null,
    name: v && v.name, message: v && v.message, code: v && v.code,
    isDOMException: v instanceof DOMException, isError: v instanceof Error,
    isTypeError: v instanceof TypeError,
    isAbortError: !!(v && v.name === 'AbortError'),
    isTimeoutError: !!(v && v.name === 'TimeoutError')};
  if (v && v.constructor) out.ctor = v.constructor.name;
  try { out.json = JSON.stringify(v); } catch (e) { out.json = '<unserializable>'; }
  return out;
})`;

let origin = '';

async function section(browser, title, body) {
  const ctx = await browser.newContext();
  const p = await ctx.newPage();
  p.on('pageerror', (e) => console.log('  pageerror:', e.message));
  await p.goto(`${origin}/`);
  const result = await p.evaluate(`(async function(){
    var out = {};
    var SHAPE = ${SHAPE};
    ${body}
    return out;
  })()`);
  console.log(`\n### ${title}`);
  console.log(JSON.stringify(result, null, 2));
  await ctx.close();
  return result;
}

(async () => {
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const browser = await chromium.launch();
  origin = `http://127.0.0.1:${server.address().port}`;

  // 1. abort() while the request is in flight.
  const mid = await section(browser, 'abort() mid-flight', `
    var c = new AbortController();
    var seen = {state: 'pending'};
    var settle = fetch(originUrl + '/slow', {signal: c.signal}).then(
      function(r) { seen.state = 'resolved'; return r.text().then(function(b){ seen.body = b; }); },
      function(e) { seen.state = 'rejected'; seen.err = SHAPE(e); }
    );
    await new Promise(function(r){ setTimeout(r, 150); });
    c.abort();
    await settle;
    out.rejection = seen;
    out.signalAborted = c.signal.aborted;
  `.replace('originUrl', JSON.stringify(origin)));
  const midHits = hits['/slow'] || 0;
  openGates();

  // 2. custom reasons.
  const custom = await section(browser, 'custom reasons', `
    async function probe(args) {
      var c = new AbortController();
      var got = {};
      var settle = fetch(originUrl + '/slow', {signal: c.signal}).then(
        function() { got.state = 'resolved'; },
        function(e) { got.state = 'rejected'; got.err = SHAPE(e); }
      );
      await new Promise(function(r){ setTimeout(r, 60); });
      c.abort.apply(c, args);
      await settle;
      return got;
    }
    out.string = await probe(['stopped']);
    out.object = await probe([{code: 7}]);
    out.error = await probe([new Error('boom')]);
    out.number = await probe([0]);
    out.nullReason = await probe([null]);
    // identity: does the rejected value compare === with the abort() argument?
    var marker = {marker: 'mine'};
    out.identity = await fetch(originUrl + '/slow2', {signal: (function(){
      var cc = new AbortController(); cc.abort(marker); return cc.signal;
    })()}).then(function(){ return 'resolved'; }, function(e){ return e === marker; });
  `.replace(/originUrl/g, JSON.stringify(origin)));
  openGates();

  // 3. already-aborted signal: does the request reach the server at all?
  const beforePre = hits['/slow'] || 0;
  const pre = await section(browser, 'already-aborted signal', `
    var c = new AbortController();
    c.abort();
    var got = {};
    await fetch(originUrl + '/slow', {signal: c.signal}).then(
      function() { got.state = 'resolved'; },
      function(e) { got.state = 'rejected'; got.err = SHAPE(e); }
    );
    out.rejection = got;
    out.throwIfAborted = (function(){
      var cc = new AbortController(); cc.abort();
      try { cc.signal.throwIfAborted(); return 'did not throw'; }
      catch (e) { return SHAPE(e); }
    })();
  `.replace(/originUrl/g, JSON.stringify(origin)));
  const preHits = (hits['/slow'] || 0) - beforePre;
  openGates();

  // 4. the late response after an abort.
  const late = await section(browser, 'late response after abort', `
    var c = new AbortController();
    var log = [];
    var settle = fetch(originUrl + '/slow', {signal: c.signal}).then(
      function(r) { log.push('then'); return r.text().then(function(b){ log.push('body:' + b); }); },
      function(e) { log.push('catch:' + (e && e.name)); }
    );
    await new Promise(function(r){ setTimeout(r, 120); });
    c.abort();
    await settle;
    out.logAtReject = log.slice();
    await new Promise(function(r){ setTimeout(r, 500); });
    out.logLater = log.slice();
  `.replace(/originUrl/g, JSON.stringify(origin)));
  openGates();

  // 5. signal.reason before and after abort().
  const reason = await section(browser, 'signal.reason before and after abort()', `
    var c = new AbortController();
    out.beforeAbort = SHAPE(c.signal.reason);
    c.abort();
    out.afterAbort = SHAPE(c.signal.reason);
    var d = new AbortController();
    d.abort('mine');
    out.afterCustom = SHAPE(d.signal.reason);
  `);

  // 6. non-signal `signal` values.
  const bad = await section(browser, 'non-signal signal values', `
    async function probe(init) {
      var got = {};
      try {
        var r = await fetch(originUrl + '/fast', init);
        got.state = 'resolved:' + r.status + ':' + (await r.text());
      } catch (e) { got.state = 'rejected'; got.err = SHAPE(e); }
      return got;
    }
    out.string = await probe({signal: 'abc'});
    out.syncProbe = (function () {
      try {
        var p = fetch(originUrl + '/fast', { signal: 'abc' });
        return { threwSync: false, isThenable: !!(p && typeof p.then === 'function') };
      } catch (e) { return { threwSync: true, err: SHAPE(e) }; }
    })();
    out.plainObject = await probe({signal: {}});
    out.number = await probe({signal: 7});
    out.trueValue = await probe({signal: true});
    out.nullValue = await probe({signal: null});
    out.undefinedValue = await probe({signal: undefined});
    out.noSignalKey = await probe({});
  `.replace(/originUrl/g, JSON.stringify(origin)));

  // 7. the exception object itself, and abort(undefined).
  const shape = await section(browser, 'DOMException shape and abort(undefined)', `
    var d = new DOMException('signal is aborted without reason', 'AbortError');
    var chain = [], proto = Object.getPrototypeOf(d), i = 0;
    while (proto && i < 5) { chain.push((proto.constructor && proto.constructor.name) || 'null'); proto = Object.getPrototypeOf(proto); i++; }
    out.domException = {
      str: String(d), name: d.name, message: d.message, code: d.code,
      ctor: d.constructor.name, isError: d instanceof Error, isDOM: d instanceof DOMException,
      ownKeys: Object.keys(d).join(','), hasStack: typeof d.stack,
      tag: Object.prototype.toString.call(d), protoChain: chain.join(' -> ')
    };
    var bare = new DOMException('', '');
    out.bareDOMException = {str: String(bare), name: bare.name, code: bare.code};
    var c = new AbortController();
    c.abort(undefined);
    out.abortUndefined = SHAPE(c.signal.reason);
    var e = new AbortController();
    e.abort();
    out.abortNoArgs = SHAPE(e.signal.reason);
  `);

  // 8. AbortSignal.abort() / AbortSignal.timeout(ms).
  const statics = await section(browser, 'AbortSignal.abort / timeout', `
    var s = AbortSignal.abort();
    out.abortStatic = SHAPE(s.reason);
    out.abortStaticAborted = s.aborted;
    var t = AbortSignal.timeout(60);
    out.timeoutBefore = SHAPE(t.reason);
    out.timeoutAfter = await new Promise(function(r){
      t.addEventListener('abort', function(){ r(SHAPE(t.reason)); });
    });
    var got = {};
    await fetch(originUrl + '/slow', {signal: AbortSignal.timeout(80)}).then(
      function() { got.state = 'resolved'; },
      function(e) { got.state = 'rejected'; got.err = SHAPE(e); }
    );
    out.timeoutFetch = got;
  `.replace(/originUrl/g, JSON.stringify(origin)));
  openGates();

  // 9. a live signal that never fires.
  const live = await section(browser, 'live signal, no abort', `
    var c = new AbortController();
    var r = await fetch(originUrl + '/fast', {signal: c.signal});
    out.status = r.status;
    out.body = await r.text();
    out.ok = r.ok;
  `.replace(/originUrl/g, JSON.stringify(origin)));

  // 10. transport-level cancellation: does the server see the request die?
  const cancelled = await section(browser, 'server-observed cancellation', `
    var c = new AbortController();
    var settled = 'pending';
    var p = fetch(originUrl + '/slow', {signal: c.signal});
    p.then(function () { settled = 'resolved'; }, function () { settled = 'rejected'; });
    await new Promise(function (r) { setTimeout(r, 150); });
    c.abort();
    await new Promise(function (r) { setTimeout(r, 400); });
    out.settled = settled;
  `.replace(/originUrl/g, JSON.stringify(origin)));
  const destroyedWithoutResponse = held.size;
  const stillWritable = [];
  for (const res of held.keys()) {
    stillWritable.push(res.destroyed ? 'destroyed' : 'writable');
  }
  openGates();

  await browser.close();
  openGates();
  server.close();

  console.log('\n### server hit counts');
  console.log(JSON.stringify({
    ...hits,
    slowHitsAfterMidFlight: midHits,
    preAbortedSlowHits: preHits,
    heldResponsesAtAbort: destroyedWithoutResponse,
    heldStateAfterWait: stillWritable.join(','),
  }, null, 2));

  const r = (x) => x && x.rejection && x.rejection.err;
  const checks = [
    ['abort() mid-flight rejects rather than resolves', mid.rejection && mid.rejection.state === 'rejected'],
    ['the rejection is a DOMException', r(mid) && r(mid).isDOMException],
    ['its name is AbortError', r(mid) && r(mid).isAbortError],
    ['String(err) is not the bare string "AbortError"', r(mid) && r(mid).str !== 'AbortError'],
    ['a string reason arrives verbatim', custom.string && custom.string.err.str === 'stopped' && custom.string.err.typeof === 'string'],
    ['an object reason arrives by identity', custom.identity === true],
    ['an Error reason stays an Error', custom.error && custom.error.err.isError && custom.error.err.message === 'boom'],
    ['a number reason stays a number', custom.number && custom.number.err.typeof === 'number'],
    ['an explicit null reason stays null', custom.nullReason && custom.nullReason.err.json === 'null'],
    ['signal.reason after abort() is a DOMException', reason.afterAbort && reason.afterAbort.isDOMException],
    ['signal.reason before abort() is undefined', reason.beforeAbort && reason.beforeAbort.typeof === 'undefined'],
    ['a pre-aborted signal never reaches the server', preHits === 0],
    ['a pre-aborted rejection is a DOMException', r(pre) && r(pre).isDOMException],
    ['the late response never runs a handler', late.logLater && late.logLater.join('|').indexOf('then') === -1],
    ['a non-signal signal value is a TypeError', bad.string && bad.string.err.isTypeError],
    ['the TypeError arrives as a rejection, not a sync throw', bad.syncProbe && bad.syncProbe.threwSync === false && bad.syncProbe.isThenable === true],
    ['a plain object signal is a TypeError', bad.plainObject && bad.plainObject.err.isTypeError],
    ['signal:null fetches normally', bad.nullValue && bad.nullValue.state === 'resolved:200:fast'],
    ['AbortSignal.abort() reason is a DOMException', statics.abortStatic && statics.abortStatic.isDOMException],
    ['AbortSignal.timeout() reason is a TimeoutError DOMException', statics.timeoutAfter && statics.timeoutAfter.isTimeoutError && statics.timeoutAfter.isDOMException],
    ['a live signal does not disturb a normal fetch', live.status === 200 && live.body === 'fast'],
    ['abort() tears the socket down at the server', cancelled.settled === 'rejected' && stillWritable.length > 0 && stillWritable.every(function (s) { return s === 'destroyed'; })],
    ['DOMException carries no own enumerable keys', shape.domException && shape.domException.ownKeys === ''],
    ['DOMException has no stack', shape.domException && shape.domException.hasStack === 'undefined'],
    ['its toString tag is DOMException', shape.domException && shape.domException.tag === '[object DOMException]'],
    ['its prototype chain reaches Error', shape.domException && shape.domException.protoChain.indexOf('DOMException -> Error') === 0],
    ['abort(undefined) installs the same default DOMException', shape.abortUndefined && shape.abortUndefined.str === shape.abortNoArgs.str && shape.abortUndefined.isDOMException],
  ];
  let failed = 0;
  for (const [name, ok] of checks) {
    if (ok) console.log(`ok  : ${name}`);
    else { failed++; console.log(`FAIL: ${name}`); }
  }
  console.log(failed ? `\n${failed} CHECK(S) FAILED` : '\nBR-123 ORACLE OK');
  process.exit(failed ? 1 : 0);
})().catch((e) => { console.error('harness error', e); process.exit(2); });

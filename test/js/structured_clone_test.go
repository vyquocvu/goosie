package js_test

import (
	"testing"
)

// qc:BR-292
func TestStructuredCloneDeepCopiesSupportedTypes(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		const src = { s: "hi", n: 42, b: true, nil: null, nested: { arr: [1,2,{x:3}] }, when: new Date(1700000000000), view: new Int32Array([5,6,7]), re: /ab+c/gi };
		const c = structuredClone(src);
		function chk(cond, msg) { if (!cond) throw new Error(msg); }
		chk(c !== src, "clone is not a new object");
		chk(c.s === "hi" && c.n === 42 && c.b === true && c.nil === null, "primitives");
		chk(c.nested.arr[2].x === 3, "nested object");
		chk(c.when instanceof Date && c.when.getTime() === 1700000000000, "date");
		chk(c.view instanceof Int32Array && c.view[1] === 6, "typed array");
		chk(c.re instanceof RegExp && c.re.source === "ab+c" && c.re.flags === "gi", "regexp");
		src.nested.arr[2].x = 99;
		src.s = "bye";
		chk(c.nested.arr[2].x === 3 && c.s === "hi", "clone is independent of source");
	`, "inline"); err != nil {
		t.Errorf("structuredClone deep copy: %v", err)
	}
}

// qc:BR-292
func TestStructuredCloneThrowsDataCloneErrorForUncloneableValues(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		function nameOf(fn) { try { fn(); return "no-throw"; } catch (e) { return e.name; } }
		function chk(name, msg) { if (name !== "DataCloneError") throw new Error(msg + " (got " + name + ")"); }
		chk(nameOf(function(){ structuredClone(function(){}); }), "function");
		chk(nameOf(function(){ structuredClone(Symbol("s")); }), "symbol");
		chk(nameOf(function(){ structuredClone(document.createElement("div")); }), "dom node");
	`, "inline"); err != nil {
		t.Errorf("structuredClone DataCloneError: %v", err)
	}
}

// qc:BR-292
func TestStructuredClonePreservesSharedAndCyclicReferences(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		const shared = { v: 1 };
		const obj = { a: shared, b: shared };
		obj.self = obj;
		const c = structuredClone(obj);
		if (c.a !== c.b) throw new Error("shared reference identity lost in clone");
		if (c.a === shared) throw new Error("clone aliases the source object");
		if (c.self !== c) throw new Error("cycle not preserved");
	`, "inline"); err != nil {
		t.Errorf("structuredClone references: %v", err)
	}
}

// qc:BR-292
func TestStructuredCloneDetachesTransferredArrayBuffer(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		const buf = new ArrayBuffer(4);
		new Uint8Array(buf)[0] = 7;
		const holder = { buf: buf };
		const c = structuredClone(holder, { transfer: [buf] });
		if (buf.byteLength !== 0) throw new Error("source ArrayBuffer was not detached");
		if (!(c.buf instanceof ArrayBuffer)) throw new Error("clone buffer is not an ArrayBuffer");
		if (c.buf.byteLength !== 4) throw new Error("clone buffer wrong length");
		if (new Uint8Array(c.buf)[0] !== 7) throw new Error("clone lost transferred bytes");
	`, "inline"); err != nil {
		t.Errorf("structuredClone transfer: %v", err)
	}
}

// qc:BR-308
func TestStructuredClonePreservesMapAndSetCollections(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		function chk(cond, msg) { if (!cond) throw new Error(msg); }
		const src = new Map([["k","v"],[2,{a:1}]]);
		const m = structuredClone(src);
		chk(Object.prototype.toString.call(m) === "[object Map]", "clone is not branded [object Map]");
		chk(m instanceof Map, "clone lost instanceof Map");
		chk(m.size === 2, "Map size wrong");
		chk(m.get("k") === "v", "Map lost string-keyed entry");
		chk(m.get(2) && m.get(2).a === 1, "Map lost number-keyed object value");
		chk(m.get(2) !== src.get(2), "Map value aliases the source object");
		src.get(2).a = 99;
		chk(m.get(2).a === 1, "Map value is not independent of source");
		const s = structuredClone(new Set([1,2,3]));
		chk(Object.prototype.toString.call(s) === "[object Set]", "clone is not branded [object Set]");
		chk(s instanceof Set, "clone lost instanceof Set");
		chk(s.size === 3 && s.has(2) && !s.has(4), "Set contents wrong");
		const nested = new Map([["o",{x:1}]]);
		const cn = structuredClone(nested);
		chk(cn.get("o") !== nested.get("o") && cn.get("o").x === 1, "Map nested value not deep-cloned");
		const shared = { v: 1 };
		const sm = structuredClone(new Map([["a", shared], ["b", shared]]));
		chk(sm.get("a") === sm.get("b") && sm.get("a") !== shared, "Map lost shared-reference identity");
		const cyc = new Map(); cyc.set("self", cyc);
		const cc = structuredClone(cyc);
		chk(cc instanceof Map && cc.get("self") === cc, "Map cycle not preserved");
		chk(structuredClone(new Map()).size === 0, "empty Map clone wrong size");
		chk(structuredClone(new Set()).size === 0, "empty Set clone wrong size");
	`, "inline"); err != nil {
		t.Errorf("structuredClone Map/Set: %v", err)
	}
}

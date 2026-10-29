package js

// setupFloat16Array provides the Float16Array typed-array constructor where
// the embedded JS runtime does not yet expose the standard constructor. It
// stores IEEE 754 binary16 values in a Uint16Array backing store, so it remains
// an ArrayBuffer view and Blob/structured byte consumers see the right bytes.
func (r *Runtime) setupFloat16Array() {
	_, _ = r.vm.RunString(`(function(global) {
	if (typeof global.Float16Array === "function") return;
	function toHalf(value) {
		var x = Number(value);
		if (Number.isNaN(x)) return 0x7e00;
		var sign = x < 0 || Object.is(x, -0) ? 0x8000 : 0;
		x = Math.abs(x);
		if (x === Infinity) return sign | 0x7c00;
		if (x === 0) return sign;
		if (x < Math.pow(2, -14)) {
			var sub = x / Math.pow(2, -24);
			var rounded = Math.floor(sub);
			var fraction = sub - rounded;
			if (fraction > 0.5 || (fraction === 0.5 && rounded % 2)) rounded++;
			return sign | Math.min(rounded, 0x400);
		}
		var exponent = Math.floor(Math.log2(x));
		var significand = x / Math.pow(2, exponent) - 1;
		var mantissa = significand * 1024;
		var roundedMantissa = Math.floor(mantissa);
		var remainder = mantissa - roundedMantissa;
		if (remainder > 0.5 || (remainder === 0.5 && roundedMantissa % 2)) roundedMantissa++;
		if (roundedMantissa === 1024) { roundedMantissa = 0; exponent++; }
		if (exponent > 15) return sign | 0x7c00;
		return sign | ((exponent + 15) << 10) | roundedMantissa;
	}
	function Float16Array(source, byteOffset, length) {
		if (!new.target) throw new TypeError("Constructor Float16Array requires 'new'");
		var view;
		if (source instanceof ArrayBuffer || (typeof SharedArrayBuffer !== "undefined" && source instanceof SharedArrayBuffer)) {
			view = new Uint16Array(source, byteOffset, length);
		} else if (typeof source === "number") {
			view = new Uint16Array(source);
		} else {
			var values = Array.from(source || []);
			view = new Uint16Array(values.length);
			for (var i = 0; i < values.length; i++) view[i] = toHalf(values[i]);
		}
		Object.setPrototypeOf(view, Float16Array.prototype);
		return view;
	}
	Float16Array.prototype = Object.create(Uint16Array.prototype);
	Object.defineProperty(Float16Array.prototype, "constructor", { value: Float16Array, writable: true, configurable: true });
	Object.defineProperty(Float16Array.prototype, Symbol.toStringTag, { value: "Float16Array", configurable: true });
	Object.defineProperty(Float16Array, "BYTES_PER_ELEMENT", { value: 2 });
	global.Float16Array = Float16Array;
})(globalThis);`)
}

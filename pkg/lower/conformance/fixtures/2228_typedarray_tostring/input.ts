// A typed array's toString joins its elements with a comma, the same rendering
// Array.prototype.toString gives, and Object.prototype.toString.call reads its class
// tag "[object <Name>]" off the concrete constructor name. toLocaleString groups its
// digits by the runtime locale, so it stays on the engine rather than lower here.
const a = new Int32Array([1, 2, 3]);
console.log(a.toString());

// A float view stringifies each element as a Number, so a fractional value keeps its
// decimal digits.
const b = Float64Array.of(1.5, 2.5);
console.log(b.toString());

// An empty view joins to the empty string, the same as an empty array.
const empty = new Uint16Array(0);
console.log(empty.toString());

// Object.prototype.toString.call reads the class tag off the concrete constructor,
// which the numeric family carries...
console.log(Object.prototype.toString.call(a));
console.log(Object.prototype.toString.call(b));

// ...and the Uint8Array byte view carries the same way.
const u = new Uint8Array([9, 8, 7]);
console.log(Object.prototype.toString.call(u));

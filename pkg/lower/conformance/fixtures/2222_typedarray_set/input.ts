// set copies a source into the view at an offset, coercing each element. The
// source is either a plain array-like or another typed array, and when the source
// is a view of the same buffer the copy still comes out right because set reads the
// whole source before it writes any element.
const d = new Int16Array(4);
d.set([100, 200, 300], 1);
console.log(d.join(","));

// set from another typed array of a different element type converts each element
// through the destination's store rule.
const src = new Float64Array([1.5, 2.5, 3.9]);
const dst = new Int32Array(3);
dst.set(src);
console.log(dst.join(","));

// overlapping set: copy a leading view onto a later position in the same buffer.
const buf = new ArrayBuffer(6 * 4);
const a = new Int32Array(buf);
a.set([1, 2, 3, 4, 5, 6]);
a.set(a.subarray(0, 4), 2);
console.log(a.join(","));

// overlapping set the other way: copy a trailing view onto an earlier position.
const b = new Int32Array([1, 2, 3, 4, 5, 6]);
b.set(b.subarray(2), 0);
console.log(b.join(","));

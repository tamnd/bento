// Int32Array.of builds an array from its arguments, one to one; Int32Array.from
// copies a source list into a fresh array of this element kind. Both coerce each
// value into the element type the way an indexed write would, and the generic
// %TypedArray% forms called off the abstract base are a later slice.
const a = Int32Array.of(1, 2, 3);
console.log(a.join(","));

// of coerces each argument into the element kind, truncating toward zero.
const b = Int32Array.of(1.9, 2.9, 300);
console.log(b.join(","));

// from over a number-array literal.
const c = Int32Array.from([10, 20, 30]);
console.log(c.join(","));

// from over a number-array value.
const nums = [4, 5, 6];
const d = Int32Array.from(nums);
console.log(d.join(","));

// from over another numeric typed array, widening each element then coercing it
// into this kind; the source array is untouched.
const f = Float64Array.of(1.5, 2.5, 3.5);
const g = Int32Array.from(f);
console.log(g.join(","));
console.log(f.join(","));

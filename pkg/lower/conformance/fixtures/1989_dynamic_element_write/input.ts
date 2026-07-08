// A write on a dynamic receiver has no static field to store into, so it
// dispatches at runtime through the boxed value's setter, the store mirror of the
// dynamic read the sibling fixtures lock: a string-literal key and a computed key
// both reach the same named property, a number index reaches an array element, and
// a read afterward sees the stored value. This locks the read and write halves of
// the dynamic member and index path together, both a named property and a computed
// key on an untyped value.

const o: any = {};
o["k"] = 1;
o.named = 2;
let key: any = "dyn";
o[key] = 3;
console.log(o["k"], o.named, o[key]);
console.log(o.k, o["named"], o["dyn"]);

const a: any = [];
a[0] = 5;
a[2] = 9;
console.log(a[0], a[1], a[2], a.length);

// A compound write o.k <op>= v on a fixed-shape object lowers to the same field
// selector a plain write o.k = v takes, so the compound arithmetic runs against
// the stored field and the result stores back in place. combineBinary builds
// o.K <op> v the way the class-field compound store already does, and a step of one
// collapses to Go's o.N++. A string field's += concatenates.
const o = { n: 10, s: "a" };
o.n += 5;
o.n *= 2;
o.n -= 1;
o.n += 1;
o.s += "b";
console.log(String(o.n));
console.log(o.s);

// Spread of a string in an array literal splices its code points, each a
// one-code-point string, the same walk for...of over a string takes. An astral
// character counts as one code point, so a surrogate pair is not split.
const a = [..."abc"];
console.log(a.length);
console.log(a[0] + a[1] + a[2]);

const b = ["x", ..."yz", "w"];
console.log(b.join("-"));

const c = [..."a\u{1F600}b"];
console.log(c.length);

// The null and undefined literals have no Go value of their own, so in a dynamic
// slot they lower to the value.Null and value.Undefined singletons. Once boxed
// they carry their runtime tag, so a presence test reads the right answer: null
// is null and not undefined, undefined is the mirror, and the two are loosely
// equal (both nullish) but not strictly equal (different tags).
let a: any = null;
let b: any = undefined;

console.log(a === null);
console.log(a === undefined);
console.log(b === undefined);
console.log(b === null);
console.log(a == b);
console.log(a === b);
console.log(a != b);

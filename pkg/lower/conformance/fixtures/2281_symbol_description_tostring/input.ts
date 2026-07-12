// Symbol(desc) records its description: toString renders it as "Symbol(desc)", and
// the description getter reports whether one was given, undefined for a bare Symbol().
const a = Symbol("hello");
const b = Symbol();
console.log(a.toString());
console.log(b.toString());
console.log(a.description === undefined);
console.log(b.description === undefined);

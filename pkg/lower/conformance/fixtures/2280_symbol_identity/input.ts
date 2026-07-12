// A symbol is a unique primitive: typeof reports "symbol", each Symbol() call is a
// fresh identity that is never === to another, and a symbol is always === to itself.
const a: symbol = Symbol("shared");
const b: symbol = Symbol("shared");
const t: string = typeof a;
console.log(t);
console.log(a === b);
console.log(a === a);

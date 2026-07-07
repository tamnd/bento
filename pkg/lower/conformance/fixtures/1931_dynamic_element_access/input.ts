// A bracket read a[i] on a dynamic receiver dispatches at runtime: the receiver
// carries its own kind, so the read routes through GetIndex for a number index
// and GetElem for a dynamic one, the same dispatch a static read would take. A
// boxed string indexes to its one-code-unit strings and answers its length, the
// way compareArray reads its any-typed arguments element by element to format a
// mismatch.
let s: any = "hello";
let i: number = 1;
console.log(s[i]);
console.log(s[0]);
let k: any = 4;
console.log(s[k]);
console.log(s["length"]);

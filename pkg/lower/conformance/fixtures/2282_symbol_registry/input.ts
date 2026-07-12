// Symbol.for interns one symbol per string key in the global registry, so two calls
// with an equal key return the same reference, while different keys and a fresh Symbol
// never share identity with a registered one. Symbol.keyFor reads the key back at run
// time; consuming its string-or-undefined result waits on union support, so the
// registry's identity guarantee is what this fixture pins down.
const a: symbol = Symbol.for("shared");
const b: symbol = Symbol.for("shared");
const c: symbol = Symbol.for("other");
const d: symbol = Symbol("shared");
console.log(a === b);
console.log(a === c);
console.log(a === d);

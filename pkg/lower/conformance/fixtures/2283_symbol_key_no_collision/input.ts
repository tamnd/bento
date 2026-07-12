// A symbol keys an object by identity, not by its description, so a symbol key never
// collides with a string key that spells the same text, and two symbols that share a
// description stay distinct keys with independent slots beside the string key.
const a: symbol = Symbol("k");
const b: symbol = Symbol("k");
const o: any = {};
o[a] = 1;
o[b] = 2;
o["k"] = 3;
console.log(o[a]);
console.log(o[b]);
console.log(o["k"]);
console.log(a === b);

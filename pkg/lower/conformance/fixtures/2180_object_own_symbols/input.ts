const s1 = Symbol("one");
const s2 = Symbol("two");
const o: any = {};
o[s1] = 1;
o[s2] = 2;
o.plain = 3;

const syms = Object.getOwnPropertySymbols(o);
console.log(syms.length);
console.log(syms[0] === s1);
console.log(syms[1] === s2);

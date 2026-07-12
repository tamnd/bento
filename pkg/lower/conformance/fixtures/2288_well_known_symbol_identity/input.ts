// Each well-known symbol is one interned identity, so two reads of the same one are
// the same reference while different well-known symbols never share identity. Tests
// that only read a well-known symbol's identity, as match, replace, search, split, and
// unscopables here, rely on this equality holding.
console.log(Symbol.match === Symbol.match);
console.log(Symbol.replace === Symbol.replace);
console.log(Symbol.search === Symbol.search);
console.log(Symbol.split === Symbol.split);
console.log(Symbol.unscopables === Symbol.unscopables);
const m: symbol = Symbol.match;
const r: symbol = Symbol.replace;
console.log(m === r);

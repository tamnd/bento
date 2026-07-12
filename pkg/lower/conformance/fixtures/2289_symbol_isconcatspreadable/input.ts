// Array.prototype.concat spreads an argument that is an array or that carries a truthy
// Symbol.isConcatSpreadable, and appends any other argument whole. The default over
// two plain arrays already lowers correctly, spreading each. Honoring an explicit
// Symbol.isConcatSpreadable, whether to keep a spreadable array-like whole or to
// spread a non-array that opts in, needs concat to read that symbol flag off each
// argument at run time. The argument that would carry the flag is an array-like typed
// any, which the typed concat lowering does not accept, so the call hands back rather
// than ignore the flag and spread by the static shape. Reading isConcatSpreadable in
// concat is a later slice.
const parts: any = [1, 2];
parts[Symbol.isConcatSpreadable] = false;
const joined = [0].concat(parts);
console.log(joined.length);

// A Map or Set iterator stored in a local and spread into exactly one consumer ranges
// the receiver's insertion-ordered snapshot the direct [...m.keys()] form ranges. The
// iterator-object type does not lower, so the declaration emits nothing and the spread
// sees through the identifier to the receiver and accessor. The store is single-use,
// so the local is read exactly once, at the spread, over a receiver never reassigned.
const m = new Map<string, number>([["a", 1], ["b", 2]]);
const mk = m.keys();
const ka = [...mk];
console.log(ka.length, ka[0], ka[1]);
const mv = m.values();
const va = [...mv];
console.log(va.length, va[0], va[1]);
const me = m.entries();
const ea = [...me];
console.log(ea.length, ea[0][0], ea[0][1]);
const s = new Set<number>([7, 8, 9]);
const sv = s.values();
function count(...xs: number[]): number {
  return xs.length;
}
console.log(count(...sv));

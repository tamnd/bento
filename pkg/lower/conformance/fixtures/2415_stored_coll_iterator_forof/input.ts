// A Map or Set iterator stored in a local and driven by exactly one for...of ranges
// the receiver's insertion-ordered snapshot the direct m.entries() form ranges. The
// iterator-object type does not lower, so the declaration emits nothing and the loop
// sees through the identifier to the receiver and accessor. The store is single-use,
// so the local is read exactly once, at the loop, over a receiver never reassigned.
const m = new Map<string, number>([["a", 1], ["b", 2]]);
const me = m.entries();
for (const [k, v] of me) {
  console.log(k, v);
}
const mk = m.keys();
for (const k of mk) {
  console.log(k);
}
const mv = m.values();
for (const v of mv) {
  console.log(v);
}
const s = new Set<number>([7, 8]);
const sv = s.values();
for (const x of sv) {
  console.log(x);
}
const se = s.entries();
for (const [a, b] of se) {
  console.log(a, b);
}

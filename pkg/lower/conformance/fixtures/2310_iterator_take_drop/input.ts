// Iterator.prototype.take yields at most the first limit values and then stops;
// drop skips the first limit values and yields the rest. Both are lazy and chain,
// so drop(2).take(2) skips two then keeps the next two. A limit larger than the
// source takes the whole source, and dropping more than the source holds yields
// nothing. Both coerce the limit through ToNumber and reject a NaN or negative
// count with a RangeError before pulling a value.
const a = [1, 2, 3, 4, 5, 6, 7];
for (const x of a.values().take(3)) {
  console.log(x);
}
for (const y of a.values().drop(5)) {
  console.log(y);
}
for (const z of a.values().drop(2).take(2)) {
  console.log(z);
}
for (const w of a.values().take(100)) {
  console.log(w);
}
let dropAll = 0;
for (const q of a.values().drop(100)) {
  dropAll += q;
}
console.log(dropAll);
try {
  a.values().take(-1);
} catch (e: any) {
  console.log(e.constructor.name);
}
try {
  a.values().drop(NaN);
} catch (e: any) {
  console.log(e.constructor.name);
}

// Only the empty new Set() lowers today. The iterable-argument form, new
// Set([1, 2, 3]), has to walk its argument and add each element, which is the
// for-of-over-an-array path a later slice brings, so this whole unit hands back
// rather than emit a Set that silently drops its initial members.
const s = new Set<number>([1, 2, 3]);
console.log(s.size);

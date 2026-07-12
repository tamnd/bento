// new Set(iterable) walks its argument and adds each element, so new Set([1, 2, 3])
// builds a set of the three members and size reports three. The array literal is
// ranged in order, deduping by SameValueZero as it goes.
const s = new Set<number>([1, 2, 3]);
console.log(s.size);

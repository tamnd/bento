// Iterator.from wraps an iterable as an iterator helper the lazy methods drive. It
// drives an array over its indices and a string over its code points, so the helper
// chain reads from either the same way. A foreign iterable (a Set, a generator) is a
// later slice: only a static array or string lowers here.
console.log(Iterator.from([10, 20, 30]).map((n: number): number => n + 1).toArray());
console.log(Iterator.from("abc").toArray());
console.log(Iterator.from([1, 2, 3, 4]).filter((n: number): boolean => n % 2 === 0).toArray());
for (const c of Iterator.from("xy")) {
  console.log(c);
}

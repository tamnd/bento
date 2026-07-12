// Iterator.prototype.reduce folds the source to a single value and toArray collects it
// into an array. reduce with a seed folds from it; without a seed the first value seeds
// the accumulator and an empty source throws a TypeError. Both return a boxed value, so
// they print straight through console.log.
const a = [1, 2, 3, 4];
console.log(a.values().reduce((acc: number, n: number): number => acc + n, 100));
console.log(a.values().reduce((acc: number, n: number): number => acc + n));
console.log(a.values().map((n: number): number => n * 2).toArray());
console.log(a.values().filter((n: number): boolean => n % 2 === 0).toArray());
const empty: number[] = [];
try {
  empty.values().reduce((acc: number, n: number): number => acc + n);
} catch (e: any) {
  console.log(e.constructor.name);
}

// Iterator.prototype.map and filter are lazy: they return a new iterator that pulls
// from the source one value at a time as it is consumed, rather than build an array.
// map lifts each value through its callback, filter keeps the values whose callback
// is truthy, and the two chain, each wrapping the iterator below it. Both callbacks
// receive the value and its zero-based index among the values the source yielded.
const a = [1, 2, 3, 4, 5, 6];
for (const x of a.values().map((n: number): number => n * 2)) {
  console.log(x);
}
for (const y of a.values().filter((n: number): boolean => n % 2 === 0)) {
  console.log(y);
}
for (const z of a.values().map((n: number): number => n + 1).filter((n: number): boolean => n % 3 === 0)) {
  console.log(z);
}
const it = a.values().map((n: number, i: number): number => n * 10 + i);
console.log(it.next().value);
console.log(it.next().value);

// Iterator.prototype.forEach, some, every, and find drive the source with a callback.
// forEach visits every value for its side effect, some and every short-circuit to a
// boolean, and find returns the first passing value or undefined.
const a = [1, 2, 3, 4, 5];
a.values().forEach((n: number): void => { console.log(n); });
console.log(a.values().some((n: number): boolean => n > 3));
console.log(a.values().some((n: number): boolean => n > 9));
console.log(a.values().every((n: number): boolean => n > 0));
console.log(a.values().every((n: number): boolean => n > 3));
console.log(a.values().find((n: number): boolean => n % 2 === 0));
console.log(a.values().find((n: number): boolean => n > 9));

// Iterator.prototype.flatMap maps each value to an iterable and flattens the results.
// Its flatten step uses reject-primitives handling: a mapper that returns a value that
// is not an iterable object throws a TypeError. Boxing a statically typed array back
// out of a mapper is a general lowering gap, so the flatten-an-array case is not lowered
// yet; this pins the reject-primitives error, which the runtime raises before it yields.
const a = [1, 2, 3];
try {
  for (const x of a.values().flatMap((n: any): any => n)) {
    console.log(x);
  }
} catch (e: any) {
  console.log(e.constructor.name);
}

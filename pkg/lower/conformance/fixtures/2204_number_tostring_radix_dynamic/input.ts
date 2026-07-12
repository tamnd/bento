// n.toString(radix) with a runtime radix applies ToInteger to the radix and throws
// the RangeError the spec raises for a radix outside 2..36. The radix is a
// parameter here, so it runs through the runtime path rather than a compile-time
// guard: a hex render, a fractional value that exercises the dtoa-in-base fraction
// loop, and an out-of-range radix caught as a RangeError.
function radix(x: number, r: number): string {
  return x.toString(r);
}
console.log(radix(255, 16));
console.log(radix(0.5, 2));
console.log(radix(1000000, 36));
try {
  radix(1, 1);
} catch (e: any) {
  console.log(e.name);
}

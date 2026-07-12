// A number formatter with a runtime digit count applies ToInteger to the count,
// so a fractional count truncates, and throws the RangeError the spec raises when
// the count lands outside the method's valid range. The count is a parameter here,
// so it cannot be range-checked at compile time and runs through the runtime path.
function fixed(x: number, d: number): string {
  return x.toFixed(d);
}
function exp(x: number, d: number): string {
  return x.toExponential(d);
}
function prec(x: number, p: number): string {
  return x.toPrecision(p);
}
console.log(fixed(123.456, 2));
console.log(fixed(123.456, 2.9));
console.log(exp(1234.5, 2));
console.log(prec(123.456, 3));
try {
  fixed(1, 101);
} catch (e: any) {
  console.log(e.name);
}
try {
  prec(1, 0);
} catch (e: any) {
  console.log(e.name);
}

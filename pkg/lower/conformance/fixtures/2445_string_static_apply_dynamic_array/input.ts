// String.fromCodePoint.apply and String.fromCharCode.apply reach a code-point array whose
// element type is dynamic in the JS-as-TS harness, an any[] rather than a number[], the
// shape regExpUtils.js's buildString takes when its array is untyped. apply coerces each
// element with ToNumber before the character is taken, so a numeric string joins a plain
// number, and the coerced elements spread into the coercing variadic value constructor.
function fromPoints(a: any, b: any): string {
  const points: any[] = [a, b];
  return String.fromCodePoint.apply(null, points);
}
function fromCodes(a: any, b: any, c: any): string {
  const codes: any[] = [a, b, c];
  return String.fromCharCode.apply(null, codes);
}
function emptyChars(): string {
  const codes: any[] = [];
  return String.fromCharCode.apply(null, codes);
}
console.log(fromPoints(104, 105)); // hi, plain numbers through a dynamic array
console.log(fromPoints(65, "66")); // AB, a numeric string coerces the way ToNumber does
console.log(fromPoints(0x1f600, 0x1f4a9)); // two astral points, each a surrogate pair
console.log(fromCodes(88, 89, 90)); // XYZ through fromCharCode
console.log(emptyChars()); // empty string over an empty array

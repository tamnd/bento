// join on an array whose element type is dynamic. The lowerer cannot pick a
// fixed element ToString (NumberToString, BoolToString, or the string identity),
// so it runs the abstract ToString on each boxed element at runtime, with join's
// own rule that undefined and null contribute the empty string rather than their
// names. A rest parameter typed any[] is the shape that reaches this, the same
// dynamic-element array the assert prelude's compareArray.format joins.

function joined(...xs: any[]): string {
  return xs.join(", ");
}
console.log(joined(1, "x", true));
console.log(joined(1, null, undefined, 2));

function packed(...xs: any[]): string {
  return xs.join();
}
console.log(packed(10, 20, 30));

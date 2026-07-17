// Audit wave W1c: a value-returning || or && whose operands are unlike primitives, a
// string and a number, returns their union, the left operand boxed into its arm and
// returned when the operator short-circuits to it and the right otherwise. || yields
// the left when it is truthy and the right when the left is falsy; && yields the left
// when it is falsy and the right when the left is truthy. The union reads its
// truthiness through the ToBoolean method every arm grows, so the surviving operand is
// observable end to end.

function coalesce(s: string, n: number): string {
  const x = s || n;
  if (x) return "kept";
  return "empty";
}

// A truthy left short-circuits ||, so the string arm survives.
console.log(coalesce("hi", 5));
// A falsy left falls through to the number arm, itself truthy.
console.log(coalesce("", 5));
// A falsy left and a falsy right leave a falsy union.
console.log(coalesce("", 0));

function both(s: string, n: number): string {
  const y = s && n;
  if (y) return "on";
  return "off";
}

// A truthy left falls through && to the number arm, itself truthy.
console.log(both("hi", 5));
// A falsy left short-circuits &&, so the empty string survives.
console.log(both("", 5));
// A truthy left falls through to a falsy number arm.
console.log(both("hi", 0));

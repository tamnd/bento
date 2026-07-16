// A tagged-sum union in boolean position reads its truth through the ToBoolean method
// the union grows: the tag selects the active arm, and each arm reports its JavaScript
// truthiness, a number falsy at zero and NaN, a string falsy when empty, and the
// undefined and null sentinels always falsy. This fixture drives a number | string |
// undefined through an if condition and a ternary, and a number | string | null through
// a negation, so every boolean position over a union lowers to the same test.

function classify(x: number | string | undefined): string {
  if (x) return "truthy";
  return "falsy";
}

console.log(classify(1));
console.log(classify(0));
console.log(classify("hi"));
console.log(classify(""));
console.log(classify(undefined));

function label(x: number | string | null): string {
  const tag = x ? "present" : "absent";
  if (!x) return "none:" + tag;
  return "some:" + tag;
}

console.log(label(3));
console.log(label(0));
console.log(label("x"));
console.log(label(null));

// The comparison kernel of test262's assert.js: SameValue over dynamic
// operands, the negative-zero probe, and the primitive test built from
// truthiness and typeof tags.
function isNegativeZero(value: any): boolean {
  return value === 0 && 1 / value === -Infinity;
}

function isSameValue(a: any, b: any): boolean {
  if (a === b) {
    return a !== 0 || 1 / a === 1 / b;
  }
  return a !== a && b !== b;
}

function isPrimitive(value: any): boolean {
  return !value || (typeof value !== 'object' && typeof value !== 'function');
}

console.log(isNegativeZero(-0));
console.log(isNegativeZero(0));
console.log(isSameValue(0, -0));
console.log(isSameValue(NaN, NaN));
console.log(isSameValue("a", "a"));
console.log(isSameValue(1, "1"));
console.log(isPrimitive(7));
console.log(isPrimitive("s"));

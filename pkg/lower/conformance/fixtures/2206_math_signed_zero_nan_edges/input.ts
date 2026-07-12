// The Math functions pin their behavior at the signed-zero, NaN, and infinity
// edges the way the spec fixes them, not the way a naive Go call would. Math.min
// with no argument is +Infinity and Math.max with none is -Infinity, the identity
// elements. round, sign, and min preserve a negative zero, which the 1/x test
// distinguishes from +0 by its -Infinity reciprocal. max propagates a NaN operand,
// abs clears the sign of a negative zero, and pow(-1, Infinity) is NaN.
console.log(Math.min());
console.log(Math.max());
console.log(1 / Math.round(-0.5) === -Infinity);
console.log(1 / Math.sign(-0) === -Infinity);
console.log(1 / Math.min(0, -0) === -Infinity);
console.log(Number.isNaN(Math.max(NaN, 1)));
console.log(1 / Math.abs(-0) === Infinity);
console.log(Math.pow(-1, Infinity));

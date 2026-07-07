// The ** operator is ECMAScript's Number::exponentiate, which returns NaN in two
// spots where Go's math.Pow returns one. A NaN exponent is always NaN, even at a
// base of one, but math.Pow(1, NaN) is 1. A base whose magnitude is one raised to
// an infinite exponent is NaN, but math.Pow(-1, +Inf) and math.Pow(1, -Inf) are
// both 1. Lowering ** and Math.pow through value.Pow rather than math.Pow keeps
// these cases spec-accurate while every ordinary power still agrees. A test262
// exponentiation probe drives the unit-base and NaN-exponent shapes; a plain power
// and a fractional exponent confirm the common path is untouched.
function pow(a: number, b: number): number {
  return a ** b;
}
console.log(pow(2, 10));
console.log(pow(9, 0.5));
console.log(pow(1, NaN));
console.log(pow(-1, Infinity));
console.log(pow(1, -Infinity));
console.log(Math.pow(1, NaN));
console.log(Math.pow(-1, -Infinity));

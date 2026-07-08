// A bitwise operator or ** over a dynamic operand coerces each side the way the
// language does before it operates: the bitwise operators run ToInt32 (ToUint32 on
// the left of >>>) so both sides narrow to a 32-bit integer, and ** runs ToNumber
// and raises to the power. A numeric string coerces the same as a number, so "6" & 3
// is 2, not a concatenation.

function and(a: any, b: any): number {
  return a & b;
}

function ushr(a: any, b: any): number {
  return a >>> b;
}

function pow(a: any, b: any): number {
  return a ** b;
}

console.log(and(6, 3));
console.log(and("12", 10));
console.log(ushr(-1, 28));
console.log(pow(2, 10));

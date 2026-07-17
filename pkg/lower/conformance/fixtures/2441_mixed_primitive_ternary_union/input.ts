// Audit wave W1c: a conditional whose branches are unlike primitives, a number and a
// string, has no single Go type carrying both, so the checker types the whole
// expression as their union and the ternary lowers to an IIFE returning the tagged-sum
// union with each branch boxed into its arm. The union then reads its truthiness
// through the ToBoolean method every arm grows, so the value is observable end to end.

function pick(useNum: boolean, n: number, s: string): string {
  const x = useNum ? n : s;
  if (x) return "truthy";
  return "falsy";
}

// The true branch is the number arm.
console.log(pick(true, 7, "hi"));
// A zero in the number arm is falsy.
console.log(pick(true, 0, "hi"));
// The false branch is the string arm.
console.log(pick(false, 7, "hi"));
// An empty string in the string arm is falsy.
console.log(pick(false, 7, ""));

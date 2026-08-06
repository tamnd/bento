// A bigint in boolean position is falsy only at 0n, the way a number is falsy only at
// zero and NaN. Nothing else about it is falsy: a negative bigint is truthy, and so is
// one too large for any Go integer. The test is value.BigIntToBool over the *big.Int a
// bigint lowers to, since Go has no == against a literal for a pointer to a big.Int,
// and the same call answers each arm of a union and each present inner of an optional.

let b: bigint = 0n;
if (b) console.log("zero-truthy");
else console.log("zero-falsy");

b = 5n;
console.log(!!b);
b = -3n;
console.log(!!b);
b = 9007199254740993n;
console.log(!!b, b ? "big" : "small");

b = 0n;
let spins = 0;
while (b) spins++;
console.log(spins);

// A union carrying a bigint arm reads its truth through the ToBoolean method the union
// grows, which switches the tag to the active arm's own falsy rule.
function classify(x: string | bigint): string {
  return x ? "truthy" : "falsy";
}

console.log(classify(1n));
console.log(classify(0n));
console.log(classify("hi"));
console.log(classify(""));

// An optional bigint is falsy two ways, absent or present and zero.
function present(x: bigint | undefined): boolean {
  if (x) return true;
  return false;
}

console.log(present(4n), present(0n), present(undefined));

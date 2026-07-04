// A bigint renders its digits with toString(): no argument gives the decimal form,
// the same digits String(b) produces, and a radix argument gives the digits in that
// base with the same lowercase 0-9a-z alphabet and leading minus JavaScript uses, so
// (-255n).toString(16) is "-ff". valueOf() returns the bigint itself, the identity,
// so a value read back through it prints unchanged.
function base(x: bigint, radix: number): string {
  return x.toString(radix);
}

function run(): void {
  console.log((255n).toString());
  console.log((255n).toString(16));
  console.log((255n).toString(2));
  console.log((255n).toString(8));
  console.log((-255n).toString(16));
  console.log((35n).toString(36));
  console.log(base(1000000n, 16));
  console.log((123n).valueOf());
}

run();

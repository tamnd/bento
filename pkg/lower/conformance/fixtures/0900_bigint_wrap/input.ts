// BigInt.asIntN(bits, x) wraps a bigint into the signed two's-complement integer of
// the given width, so the top half of the range folds to negatives: asIntN(8, 255n)
// is -1n and asIntN(8, 128n) is -128n. BigInt.asUintN(bits, x) wraps into the
// unsigned integer of that width, so a negative value comes back in range and a
// value past the width wraps down by the modulus. A width of zero wraps everything
// to 0n. Both are static calls on the global BigInt, not methods on a bigint value.
function signedByte(x: bigint): bigint {
  return BigInt.asIntN(8, x);
}

function unsignedWord(x: bigint): bigint {
  return BigInt.asUintN(16, x);
}

function run(): void {
  console.log(signedByte(127n));
  console.log(signedByte(128n));
  console.log(signedByte(255n));
  console.log(signedByte(256n));
  console.log(signedByte(-1n));

  console.log(unsignedWord(0n));
  console.log(unsignedWord(-1n));
  console.log(unsignedWord(65536n));
  console.log(unsignedWord(70000n));

  console.log(BigInt.asIntN(0, 123n));
  console.log(BigInt.asUintN(0, 123n));
}

run();

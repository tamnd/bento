// The throw side and the catch side meet: throwing an object literal boxes it into
// the ThrownValue carrier, and a destructuring catch binding reads a field off the
// caught value's boxed form. Throwing { code: 1 } and catching it as { code } reads
// code back as 1, so the value round-trips from the throw through the destructure.
try {
  throw { code: 1 };
} catch ({ code }: any) {
  console.log(code);
}

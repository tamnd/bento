// arguments[i] reads the i-th argument the call passed, off the store the body
// materializes from its parameters. Because the checker forces an all-required,
// rest-free signature to receive exactly one argument per parameter, the i-th
// parameter is the i-th argument, so the store read answers what the source reads.
// This proves arguments indexing at a literal and at a variable index end to end.
function first(a: number, b: number): unknown {
  return arguments[0];
}

// arguments[0] is the first argument, 10.
console.log(first(10, 20));

function pick(a: number, b: number, c: number): unknown {
  const i = 2;
  return arguments[i];
}

// arguments[2] is the third argument, 30.
console.log(pick(10, 20, 30));

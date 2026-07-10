// A function body that reads arguments materializes a backing store at body entry so
// arguments.length answers the count of arguments the call passed. The checker forces
// every call to an all-required, rest-free signature to pass exactly one argument per
// parameter, so the parameter count is the call arity and the store built from the
// parameters stands in for the passed arguments. This proves arguments.length end to
// end.
function two(a: number, b: number): number {
  return arguments.length;
}

// two is called with both parameters, so arguments.length is 2.
console.log(two(10, 20));

function one(x: number): number {
  return arguments.length;
}

// one is called with its single parameter, so arguments.length is 1.
console.log(one(7));

// A call may pass more arguments than the callee declares. JavaScript evaluates
// every argument left to right and then ignores the ones past the parameter
// count, so the callee runs on the fixed arguments and the extras vanish. This
// proves the AOT path admits the arity mismatch and drops extra arguments that
// are literals, which reference nothing, rather than refusing the call. An extra
// that reads a binding is not dropped, because removing the read would leave the
// local unused, so this fixture keeps every extra a literal.
function add(a: number, b: number): number {
  return a + b;
}

// Two extra literal arguments the two-parameter callee never sees.
console.log(add(2, 3, 4, 5));

// A single-parameter callee ignores every argument past the first.
function first(a: number): number {
  return a;
}
console.log(first(7, 8, 9));

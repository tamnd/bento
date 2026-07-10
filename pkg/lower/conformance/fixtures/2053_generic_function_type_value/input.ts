// A generic function type used as a value or a parameter type lowers at the call
// sites that fix its type arguments, since monomorphizing the enclosing generic
// resolves the callback's own type parameter to a concrete type. apply<T> takes a
// callback typed (x: T) => T; at apply(inc, 5) T is number, so the parameter
// lowers to func(float64) float64 and the named function inc passes as that Go
// value; at apply(s => s, "hi") T is string, so a second specialization lowers the
// callback over value.BStr. twice<T> returns a () => T, a function-typed value
// whose result resolves to the concrete type the call fixed.
function apply<T>(f: (x: T) => T, v: T): T {
  return f(v);
}

function inc(n: number): number {
  return n + 1;
}

function twice<T>(x: T): () => T {
  return () => x;
}

console.log(apply(inc, 5));
console.log(apply(s => s, "hi"));
console.log(twice(9)());

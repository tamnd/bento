// A call whose callee is not a bare name but a larger expression that evaluates to
// a function value lowers to that expression applied to its arguments. The three
// forms here are an array element (fs[0](x)), the result of another call (mk(5)()),
// and a parenthesized arrow applied in place (((n) => n * n)(4)). Each callee lowers
// by its own rules and the argument list is bridged the same way a named call's is,
// so a program that stores callbacks in an array, returns a closure and calls it, or
// writes an inline IIFE compiles instead of handing the unit back to the interpreter.
function mk(n: number): () => number {
  return () => n + 1;
}

function add(a: number): (b: number) => number {
  return (b: number) => a + b;
}

const fs: ((n: number) => number)[] = [(n: number) => n + 1, (n: number) => n * 2];
console.log(fs[0](3));
console.log(fs[1](3));
console.log(mk(5)());
console.log(add(2)(3));
console.log(((n: number) => n * n)(4));

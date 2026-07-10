// A function's .length is its arity: the count of parameters before the first
// defaulted or rest one. A required-only function counts every parameter, a defaulted
// tail stops the count at the first default, and a rest parameter never counts. bento
// lowers each read to the numeric constant of the declaration's arity rather than fold
// it to undefined the way a missing struct field would.
function add(a: number, b: number): number {
  return a + b;
}

function greet(name: string, greeting = "hi"): string {
  return greeting + name;
}

function sum(first: number, ...rest: number[]): number {
  return first + rest.length;
}

console.log(add.length);
console.log(greet.length);
console.log(sum.length);

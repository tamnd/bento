// Function.prototype.apply invokes a function the same way call does, but gathers the
// positional arguments in an array rather than spelling them inline. bento reads the
// elements of a plain array literal as the positional arguments, so add.apply(null,
// [2, 3]) lowers to the direct call Add(2, 3) with the this argument dropped. A null
// or undefined this evaluates to a constant, so dropping it changes nothing observed.
function add(a: number, b: number): number {
  return a + b;
}

function pi(): number {
  return 3;
}

console.log(add.apply(null, [2, 3]));
console.log(add.apply(undefined, [4, 5]));
console.log(pi.apply(null));

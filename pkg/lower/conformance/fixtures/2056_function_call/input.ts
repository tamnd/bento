// Function.prototype.call invokes a function with an explicit this and the remaining
// positional arguments. bento's plain functions take no this, since a body that
// reads this hands back when the function is lowered, so add.call(null, 2, 3) lowers
// to the direct call Add(2, 3) with the this argument dropped. A null or undefined
// this evaluates to a constant, so dropping it changes nothing the program observes.
function add(a: number, b: number): number {
  return a + b;
}

function pi(): number {
  return 3;
}

console.log(add.call(null, 2, 3));
console.log(add.call(undefined, 4, 5));
console.log(pi.call(null));

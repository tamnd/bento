// An object in boolean position has no falsy member: an object and an array are
// both always truthy, so a condition over one takes the truthy branch with no
// runtime test. The checker proves the type carries no null or undefined, so the
// lowering collapses the condition to a constant rather than testing a value that
// can only be truthy. This holds in an if, in a while, and in a ternary.

function hasShape(o: { x: number }): number {
  if (o) {
    return o.x;
  }
  return -1;
}

function hasItems(a: number[]): number {
  while (a) {
    return a.length;
  }
  return -1;
}

function firstOr(a: number[]): number {
  return a ? a[0] : -1;
}

console.log(hasShape({ x: 7 }));
console.log(hasItems([10, 20, 30]));
console.log(firstOr([9, 8]));

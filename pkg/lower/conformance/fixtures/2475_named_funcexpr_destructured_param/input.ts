// A named function expression with a destructured parameter used to hand back: the
// self-reference two-step (bind the closure to a Go local so a recursive body can call
// itself) wraps the body, and the guard bailed before reaching it. The closure the
// two-step wraps is blockBodyArrow, which already injects the entry bindings a pattern
// parameter reads its names out of, so the destructured parameter rides along. Both a
// non-recursive named expression and a recursive one that calls itself with a rebuilt
// pattern argument lower.
const add = function pair({ x, y }: { x: number; y: number }): number {
  return x + y;
};
const fac = function f([n, acc]: number[]): number {
  return n <= 1 ? acc : f([n - 1, acc * n]);
};
console.log(add({ x: 3, y: 4 }));
console.log(fac([5, 1]));

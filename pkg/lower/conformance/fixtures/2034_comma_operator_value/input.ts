// The comma operator in value position: JavaScript evaluates each left operand
// for its effect, discards it, and yields the right. A side-effecting left
// mutates as it runs; a pure left, which the checker flags as unused, still
// evaluates and is discarded. This proves both shapes lower and run left to
// right, the form the AOT front door admits and the renderer wraps in a closure
// because Go has no comma operator.
let a = 0;
let b = ((a = a + 1), (a = a + 10), 5);
console.log(a);
console.log(b);

// A pure left has no side effect, so only the final operand contributes a value.
let x = (1, 2, 3);
console.log(x);

// The chain runs strictly left to right: each assignment lands before the next.
let n = 0;
let last = ((n = n + 100), (n = n + 20), (n = n + 3), n);
console.log(last);

// A for statement's initializer may assign an existing binding rather than
// declare a new one. That expression initializer folds into Go's for init clause
// with no wrapping block, and a comma of assignments fuses into one parallel
// assignment the way a comma post clause does.

// Single assignment initializer, the counter declared above the loop.
let i = 0;
let sum = 0;
for (i = 0; i < 4; i++) {
  sum = sum + i;
}
console.log(sum);

// The counter keeps whatever the loop left it, so the binding outlives the loop.
console.log(i);

// A comma of assignments seeds two counters at once.
let a = 0;
let b = 0;
let steps = 0;
for (a = 0, b = 10; a < b; a++) {
  steps = steps + 1;
}
console.log(steps);

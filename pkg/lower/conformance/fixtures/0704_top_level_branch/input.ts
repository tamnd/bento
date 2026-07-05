// A do...while and a labeled loop are unnamed statements the frontend does not
// classify. Inside a function they already lower; at the module top level they now
// route into main the same way, so these forms run whether or not a function wraps
// them. The do...while runs its body once before testing the condition, and the
// labeled continue jumps to the next iteration of the outer loop from inside the
// inner one.
let i = 0;
let total = 0;
do {
  total = total + i;
  i = i + 1;
} while (i < 4);
console.log(total);

let once = 0;
do {
  once = once + 1;
} while (false);
console.log(once);

let n = 0;
outer: for (let a = 0; a < 3; a = a + 1) {
  for (let b = 0; b < 3; b = b + 1) {
    if (b === 1) {
      continue outer;
    }
    n = n + 1;
  }
}
console.log(n);

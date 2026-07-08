// A for statement may leave out any of its three header clauses. Each shape here
// exercises a different omission so the lowering that reads the clauses by role,
// rather than by walking children, is proven against the JavaScript results.

// No clauses at all: an infinite loop a guarded break ends.
let i = 0;
for (;;) {
  i = i + 1;
  if (i >= 3) break;
}
console.log(i);

// Condition only, the shape Go writes a while as.
let j = 0;
for (; j < 3; ) {
  j = j + 1;
}
console.log(j);

// Initializer and incrementor, no condition, ended by a guard.
let s = 0;
for (let k = 0; ; k++) {
  s = s + k;
  if (k >= 4) break;
}
console.log(s);

// Initializer and condition, the step done in the body.
let t = 0;
for (let m = 0; m < 4; ) {
  t = t + m;
  m = m + 1;
}
console.log(t);

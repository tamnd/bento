// A do...while runs its body once before it tests the condition, so the body always
// runs at least once even when the condition is false at entry. It lowers to the Go
// loop hand-written Go uses for the same shape, a bare for whose body ends by
// breaking once the condition no longer holds. A boolean condition breaks on its
// negation, and a bare number condition rides JavaScript truthiness the way a while
// with the same condition does.
function countUp(): number {
  let s = 0;
  do {
    s = s + 1;
  } while (s < 3);
  return s;
}

function runsOnce(): number {
  let n = 0;
  do {
    n = n + 1;
  } while (n > 5);
  return n;
}

function drainTruthy(): number {
  let s = 3;
  let steps = 0;
  do {
    s = s - 1;
    steps = steps + 1;
  } while (s);
  return steps;
}

function run(): void {
  console.log(countUp());
  console.log(runsOnce());
  console.log(drainTruthy());
}

run();

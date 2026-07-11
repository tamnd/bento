// A for...of that breaks out of a generator before it is exhausted must close it, so
// the suspended body unwinds through its finally block right at the break rather than
// leaving the coroutine parked on its next yield. The cleanup line prints between the
// last value the loop saw and the line after the loop, the order JavaScript's
// iterator-close protocol fixes when a loop abandons a generator early.
function* counts(): Generator<number> {
  try {
    yield 1;
    yield 2;
    yield 3;
    yield 4;
  } finally {
    console.log("cleanup");
  }
}

for (const n of counts()) {
  console.log("saw " + String(n));
  if (n === 2) {
    break;
  }
}
console.log("done");

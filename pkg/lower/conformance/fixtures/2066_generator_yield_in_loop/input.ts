// A yield inside a loop suspends the coroutine wherever it sits in the control
// flow, not only at the top level of the body: the goroutine blocks at the yield
// mid-loop and resumes there on the next pull, so the loop advances one turn per
// value the consumer asks for. This generator yields each square in turn, and the
// for...of over it prints them in order.
function* squares(limit: number): Generator<number> {
  for (let i = 0; i < limit; i++) {
    yield i * i;
  }
}

for (const s of squares(4)) {
  console.log(String(s));
}

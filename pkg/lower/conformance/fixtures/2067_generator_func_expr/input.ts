// A generator function expression is the value form of a generator function: the
// same coroutine, written as a function* stored in a const rather than declared by
// name. bento lowers it to a closure that returns a running *value.Gen, so calling
// the const hands back the coroutine a for...of drives, one value per pull, exactly
// as the declaration form does.
const range = function* (): Generator<number> {
  yield 4;
  yield 5;
  yield 6;
};

for (const n of range()) {
  console.log(String(n));
}

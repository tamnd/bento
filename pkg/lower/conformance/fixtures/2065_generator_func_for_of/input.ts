// A generator function suspends between the values it yields and resumes when the
// consumer pulls the next one. bento lowers it to a Go function that returns a
// running coroutine, a *value.Gen, whose body runs in a goroutine that blocks on
// each yield until the next pull. A for...of over the result drives that coroutine,
// pulling one value at a time and stopping when the body runs off its end, so the
// loop prints each yielded value in order.
function* count(): Generator<number> {
  yield 1;
  yield 2;
  yield 3;
}

for (const n of count()) {
  console.log(String(n));
}

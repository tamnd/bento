// yield* delegates to another generator: every value the delegate yields flows out
// through the outer generator as if the delegate's body were spliced in, and the
// yield* expression itself evaluates to the value the delegate returns. Here inner
// yields 2 and 3 then returns 5, so the outer generator prints 1, then the delegated
// 2 and 3, then 4, and the captured return value 5 as its final yield.
function* inner(): Generator<number, number> {
  yield 2;
  yield 3;
  return 5;
}

function* g(): Generator<number> {
  yield 1;
  const r = yield* inner();
  yield 4;
  yield r;
}

for (const x of g()) {
  console.log(String(x));
}

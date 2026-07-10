// yield is an expression: it evaluates to the value the consumer sends back through
// next(v). A for...of drive always sends undefined on each pull, so every yield here
// evaluates to undefined while the loop still sees each yielded value in turn. This
// pins that the sent value threads through, even when it is undefined.
function* g(): Generator<number> {
  const a = yield 1;
  console.log("a=" + String(a));
  const b = yield 2;
  console.log("b=" + String(b));
}

for (const v of g()) {
  console.log("v=" + String(v));
}

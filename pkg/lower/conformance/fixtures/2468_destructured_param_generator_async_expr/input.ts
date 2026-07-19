// A generator function expression, an await-free async function expression, and a
// block-body await-free async arrow can each take a destructured parameter: the names
// the pattern binds are read out of the synthesized parameter field at the top of the
// coroutine or value.Async body, the same entry bindings a plain function injects.
const gen = function* ({ a, b }: { a: number; b: number }) {
  yield a;
  yield b;
};
const asyncFn = async function ({ x }: { x: number }): Promise<number> {
  return x * 2;
};
const asyncArrow = async ({ y }: { y: number }): Promise<number> => {
  return y + 1;
};

console.log("start");
for (const v of gen({ a: 1, b: 2 })) {
  console.log(String(v));
}
asyncFn({ x: 5 }).then((v) => console.log(v));
asyncArrow({ y: 10 }).then((v) => console.log(v));
console.log("end");

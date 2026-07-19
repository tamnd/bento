// An async generator with a destructured parameter used to hand back: its coroutine
// body is built by a separate builder that did not inject the entry bindings a
// pattern parameter reads its names out of. asyncGeneratorCoroutine now prepends
// paramDestructureBindings the same way the plain and generator forms do, so both the
// declaration and the expression form lower, and the bindings coexist with an await
// between yields since the async generator drives both protocols through one handle.
async function* fromArray([a, b]: number[]) {
  yield a;
  await Promise.resolve(0);
  yield b;
}
const fromObject = async function* ({ p, q }: { p: number; q: number }) {
  yield p;
  yield q;
};
async function main(): Promise<void> {
  const it = fromArray([10, 20]);
  console.log((await it.next()).value);
  console.log((await it.next()).value);
  const jt = fromObject({ p: 5, q: 6 });
  console.log((await jt.next()).value);
  console.log((await jt.next()).value);
}
main();

// An awaiting async body with a destructured parameter used to hand back: the coroutine
// builder asyncCoroutineBody wraps the body in value.RunAsync without injecting the entry
// bindings a pattern parameter reads its names out of. It now prepends
// paramDestructureBindings the same way the await-free async path does, so the awaiting
// declaration, expression, and block-body arrow forms all lower and the bound names stay
// in scope across the await. A concise-body async arrow still has no block for those
// bindings and stays on the handback.
async function fromArray([a, b]: number[]): Promise<number> {
  return a + (await Promise.resolve(b));
}
const fromObject = async function ({ x }: { x: number }): Promise<number> {
  return (await Promise.resolve(x)) + 1;
};
const fromArrow = async ({ p, q }: { p: number; q: number }): Promise<number> => {
  return (await Promise.resolve(p)) * q;
};
async function main(): Promise<void> {
  console.log(await fromArray([10, 20]));
  console.log(await fromObject({ x: 41 }));
  console.log(await fromArrow({ p: 6, q: 7 }));
}
main();

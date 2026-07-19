// A concise-body async arrow with a destructured parameter used to hand back: asyncConciseBody
// builds a value.Async closure with a single return and had no block for the entry bindings a
// pattern parameter reads its names out of. It now prepends paramDestructureBindings inside that
// closure ahead of the return, so the object-pattern and array-pattern forms both lower. This
// closes the destructured-parameter family across every function form. A concise body that
// awaits still hands back at the await site, since it sets up no coroutine handle to park on.
const fromObject = async ({ x }: { x: number }): Promise<number> => x + 1;
const fromArray = async ([a, b]: number[]): Promise<number> => a * b;
async function main(): Promise<void> {
  console.log(await fromObject({ x: 41 }));
  console.log(await fromArray([6, 7]));
}
main();

// Array.fromAsync collects a source into a promise of an array. Over a sync iterable
// of promises it awaits each to its fulfilled value; over a sync iterable of plain
// values it awaits each to itself, one microtask hop later. The collected array flows
// out at the await, so the sum reads the fulfilled elements.
async function collect(): Promise<number> {
  const proms = [Promise.resolve(1), Promise.resolve(2), Promise.resolve(3)];
  const a = await Array.fromAsync(proms);
  const b = await Array.fromAsync([10, 20, 30]);
  return a[0] + a[1] + a[2] + b[0] + b[1] + b[2];
}

collect().then((v) => console.log(v));

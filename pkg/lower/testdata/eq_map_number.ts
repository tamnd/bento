// A Map with number keys filled in a loop, read back through the optional get with
// an undefined guard, then an existing key overwritten to prove set updates in
// place rather than appending. The running total folds in the size so a wrong
// insert or a wrong overwrite shows up in the sum.
export function tally(n: number): number {
  const m = new Map<number, number>();
  for (let i = 0; i < n; i++) {
    m.set(i, i * i);
  }
  let sum = 0;
  for (let i = 0; i < n; i++) {
    const v = m.get(i);
    if (v !== undefined) {
      sum += v;
    }
  }
  if (n > 0) {
    m.set(0, 100);
    const z = m.get(0);
    if (z !== undefined) {
      sum += z;
    }
  }
  return sum + m.size;
}

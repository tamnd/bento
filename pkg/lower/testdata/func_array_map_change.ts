export function labelWidth(n: number): number {
  const xs = [1, 2, 3];
  const ys = xs.map(x => (x * n).toString());
  let total = 0;
  for (const y of ys) {
    total = total + y.length;
  }
  return total;
}

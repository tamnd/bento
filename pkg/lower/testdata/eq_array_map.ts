export function scaled(n: number): number {
  const xs = [1, 2, 3];
  const ys = xs.map(x => x * n);
  let t = 0;
  for (const y of ys) {
    t = t + y;
  }
  return t;
}

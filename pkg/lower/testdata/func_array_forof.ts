export function total(n: number): number {
  const ys = [10, 20, 30, 40];
  let t = 0;
  for (const x of ys) {
    t = t + x * n;
  }
  return t + ys.length;
}

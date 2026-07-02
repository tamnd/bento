export function middle(n: number): number {
  const xs = [n, n + 1, n + 2, n + 3, n + 4];
  const a = xs.slice(1, 3);
  const b = xs.slice(-2);
  const c = xs.slice();
  return a.length * 100 + b.length * 10 + c.length;
}

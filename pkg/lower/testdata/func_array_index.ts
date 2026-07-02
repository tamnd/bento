export function pick(n: number): number {
  const xs = [10, 20, 30, 40];
  const i = n & 3;
  return xs[i] + [5, 6, 7, 8][n & 1];
}

export function lastPlusOne(n: number): number {
  const xs = [n, n + 1];
  const last = xs.pop();
  if (last !== undefined) {
    return last + 1;
  }
  return 0;
}

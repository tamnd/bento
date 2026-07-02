export function hasLast(n: number): boolean {
  const xs = [n, n + 1];
  const last = xs.pop();
  return last !== undefined;
}

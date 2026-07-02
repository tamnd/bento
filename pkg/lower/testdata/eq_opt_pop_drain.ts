export function drain(n: number): number {
  const xs = [n, n + 1, n + 2];
  let count = 0;
  while (xs.pop() !== undefined) {
    count = count + 1;
  }
  return count;
}

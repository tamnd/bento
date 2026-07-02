export function negsum(n: number): number {
  let t = 0;
  for (let i = 1; i <= n; i = i + 1) {
    t = t + i;
  }
  return -t;
}

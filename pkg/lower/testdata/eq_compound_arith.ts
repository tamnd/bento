export function accumulate(n: number): number {
  let t = 0;
  for (let i = 0; i < n; i++) {
    t += i;
  }
  t -= 1;
  t *= 2;
  return t;
}

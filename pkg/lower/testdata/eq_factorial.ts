export function fact(n: number): number {
  let acc = 1;
  let i = 2;
  while (i <= n) {
    acc = acc * i;
    i = i + 1;
  }
  return acc;
}

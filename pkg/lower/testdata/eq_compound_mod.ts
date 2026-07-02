export function wrap(x: number, m: number): number {
  x %= m;
  return x;
}

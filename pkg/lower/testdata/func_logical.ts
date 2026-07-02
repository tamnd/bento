export function between(x: number, lo: number, hi: number): number {
  if (x >= lo && x <= hi) {
    return 1;
  }
  return 0;
}

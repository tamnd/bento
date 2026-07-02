export function above(n: number): number {
  const xs = [1, 2, 3, 4];
  const kept = xs.filter(x => x > n);
  return kept.length;
}

// A Set with number members filled in a loop where every value is added twice, so
// the dedup must drop the repeats for the size to come out right. The running total
// folds in has probes for a present and an absent member and the final size, so a
// membership miss or a failed dedup diverges the sum.
export function distinct(n: number): number {
  const s = new Set<number>();
  for (let i = 0; i < n; i++) {
    s.add(i);
    s.add(i);
  }
  let sum = 0;
  for (let i = 0; i < n; i++) {
    if (s.has(i)) {
      sum += i;
    }
  }
  if (s.has(n)) {
    sum += 1000;
  }
  return sum + s.size;
}

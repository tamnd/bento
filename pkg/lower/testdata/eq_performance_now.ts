// performance.now() reads a monotonic clock, so a second reading is never before
// the first. The function does a little work between the two readings and reports
// whether time moved forward, which is true on both the engine and the compiled
// path since both read the same host clock. The elapsed value itself is not
// deterministic, so the case tests the one property that is: the ordering.
export function elapsedNonNeg(n: number): boolean {
  const a = performance.now();
  let s = 0;
  for (let i = 0; i < n; i++) s += i;
  const b = performance.now();
  return b >= a && s >= 0;
}

export function build(n: number): number {
  const xs = Array.of(n, n + 1, n + 2);
  let t = 0;
  for (const x of xs) {
    t = t + x;
  }
  return t + xs.length;
}

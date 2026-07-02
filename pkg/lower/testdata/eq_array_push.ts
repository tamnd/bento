export function build(n: number): number {
  const xs = [n];
  xs.push(n + 1);
  xs.push(n + 2, n + 3);
  let t = 0;
  for (const x of xs) {
    t = t + x;
  }
  return t + xs.length;
}

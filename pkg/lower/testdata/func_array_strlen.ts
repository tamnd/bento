export function widths(words: string[]): number {
  let n = 0;
  for (const w of words) {
    n = n + w.length;
  }
  return n;
}

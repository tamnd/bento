export function label(n: number): string {
  const even = (n & 1) === 0;
  return "n=" + n + " even=" + even + "!";
}

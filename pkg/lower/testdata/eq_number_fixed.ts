export function money(n: number): string {
  const v = n / 8;
  return v.toFixed(0) + " " + v.toFixed(2) + " " + v.toFixed(4);
}

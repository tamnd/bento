export function radices(n: number): string {
  const m = n & 0xff;
  return m.toString(16) + " " + m.toString(2) + " " + m.toString();
}

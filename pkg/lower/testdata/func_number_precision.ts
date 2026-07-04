export function sig(n: number): string {
  return n.toPrecision(1) + " " + n.toPrecision(3) + " " + n.toPrecision(5);
}

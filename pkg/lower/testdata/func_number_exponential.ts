export function sci(n: number): string {
  return n.toExponential(0) + " " + n.toExponential(2) + " " + n.toExponential(4);
}

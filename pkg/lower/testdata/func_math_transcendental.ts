export function curve(x: number, y: number): number {
  return Math.sin(x) + Math.log(y) + Math.atan2(y, x);
}

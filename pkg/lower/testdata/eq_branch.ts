export function clamp(x: number): number {
  if (x < 0) {
    return 0;
  } else if (x > 100) {
    return 100;
  }
  return x;
}

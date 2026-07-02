export function box(w: number, h: number): number {
  const width = w * 2;
  const r = { width, height: h };
  return r.width + r.height;
}

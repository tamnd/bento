function box(w: number, h: number): number {
  const r = { width: w, height: h };
  return r.width * r.height;
}

console.log(box(3, 4));
console.log(box(5, 6));

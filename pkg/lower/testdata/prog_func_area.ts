function area(w: number, h: number): number {
  return w * h;
}
let sum = 0;
for (let i = 1; i <= 3; i++) {
  sum += area(i, i);
}
console.log("total area", sum);

function head([a, b = 9]: number[]): number {
  return a + b;
}
function box({ w = 2, h }: { w?: number; h: number }): number {
  return w * h;
}
const full: { w?: number; h: number } = { w: 3, h: 4 };
console.log(head([10, 5]));
console.log(head([10]));
console.log(box(full));

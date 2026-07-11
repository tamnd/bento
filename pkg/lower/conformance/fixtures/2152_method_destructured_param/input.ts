class Rect {
  area({ w, h }: { w: number; h: number }): number {
    return w * h;
  }
  diff([x, y]: number[]): number {
    return x - y;
  }
}
const r = new Rect();
console.log(r.area({ w: 3, h: 4 }));
console.log(r.diff([9, 4]));

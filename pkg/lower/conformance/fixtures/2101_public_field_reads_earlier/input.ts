class Rect {
  width: number = 5;
  height: number = 3;
  area: number = this.width * this.height;
  perimeter: number = (this.width + this.height) * 2;
}
const r = new Rect();
console.log(r.area + "," + r.perimeter);

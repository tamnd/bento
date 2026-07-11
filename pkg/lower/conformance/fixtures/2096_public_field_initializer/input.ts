class Point {
  x: number = 3;
  y: number = 4;
  constructor() {
    console.log("in ctor: " + this.x + "," + this.y);
  }
}
const p = new Point();
console.log(p.x + "," + p.y);

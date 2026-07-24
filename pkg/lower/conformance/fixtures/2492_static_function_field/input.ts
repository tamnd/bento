// A static field whose value is a function is a package var holding the closure, so
// calling it, ClassName.field(args), lowers to that var applied to the arguments, the
// value twin of a static method's package-function call. A body-declared function
// field, static make = function () {}, stores the closure in the static init function
// and every call reads the same var. Each argument bridges to its parameter type the
// same way a static method call's does.
class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
  sum(): number {
    return this.x + this.y;
  }
  static make = function (x: number, y: number): Point {
    return new Point(x, y);
  };
  static origin = function (): Point {
    return new Point(0, 0);
  };
}
const p = Point.make(3, 4);
console.log(String(p.sum()));
const o = Point.origin();
console.log(String(o.x) + "," + String(o.y));

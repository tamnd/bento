// A semicolon in a class body is a no-op member the grammar allows between the
// real ones. Sprinkled through an otherwise ordinary class, each is skipped
// rather than misread as heritage, and the class lowers as if they were absent.
class Point {
  ;
  x: number;
  ;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
  ;
  sum(): number {
    return this.x + this.y;
  }
  ;
}

const p = new Point(3, 4);
console.log(String(p.sum()));

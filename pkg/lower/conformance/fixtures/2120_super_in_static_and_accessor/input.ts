class Shape {
  get kind(): string {
    return "shape";
  }
  static origin(): number {
    return 0;
  }
}
class Circle extends Shape {
  describe(): string {
    return super.kind + "-circle";
  }
  static origin(): number {
    return super.origin() + 1;
  }
}
console.log(new Circle().describe());
console.log(String(Circle.origin()));

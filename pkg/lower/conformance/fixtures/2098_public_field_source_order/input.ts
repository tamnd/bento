function mark(tag: string, v: number): number {
  console.log(tag);
  return v;
}
class Seq {
  a: number = mark("field a", 1);
  b: number = mark("field b", 2);
  constructor() {
    console.log("ctor body");
  }
}
new Seq();

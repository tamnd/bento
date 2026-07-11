function mark(tag: string, v: number): number {
  console.log(tag);
  return v;
}
class Seq {
  static a: number = mark("field a", 1);
  static {
    console.log("static block");
  }
  static b: number = mark("field b", 2);
}
console.log(Seq.a + "," + Seq.b);

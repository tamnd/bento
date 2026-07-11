function mark(): string {
  console.log("derived field init");
  return "t";
}
class Base {
  constructor() {
    console.log("base ctor");
  }
}
class Derived extends Base {
  tag: string = mark();
  constructor() {
    super();
    console.log("derived body");
  }
}
new Derived();

// A static field initializer that reads this touches the class constructor
// object, a dynamic-world value this slice does not model. It hands back with
// its own named reason rather than running through the static init function the
// way a this-free computed initializer does.
class Counter {
  static start: number = 1;
  static self: any = (this as any);
}

console.log(String(Counter.start));

// A default parameter on a method fills the omitted argument at the call site, the
// same call-site defaulting a top-level function takes: the method lowers to a Go
// method whose defaulted parameter is a plain field, and each call supplies the
// default in the slot it left off. This proves the instance and static method forms
// end to end.
class Box {
  scale(x: number, by: number = 2): number {
    return x * by;
  }
  static make(w: number = 3): number {
    return w * w;
  }
}

const b = new Box();

// by is omitted, so it defaults to 2: 5 * 2 is 10.
console.log(b.scale(5));

// by is supplied, so the default does not run: 5 * 3 is 15.
console.log(b.scale(5, 3));

// w is omitted on the static method, so it defaults to 3: 3 * 3 is 9.
console.log(Box.make());

// w is supplied: 4 * 4 is 16.
console.log(Box.make(4));

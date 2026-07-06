// The exact shape of test262's Test262Error: an optional dynamic constructor
// parameter defaulted with ||, a static thrower that forwards it, and calls
// that supply a string or nothing. The omitted slot fills with value.Undefined
// and the || picks the empty-string default from it at runtime.
class Test262Error {
  message: string;
  constructor(message?: any) {
    this.message = message || "";
  }
  static thrower(message?: any): never {
    throw new Test262Error(message);
  }
}

try {
  Test262Error.thrower("boom");
} catch (err: any) {
  console.log(err.name + ":" + err.message);
}

try {
  Test262Error.thrower();
} catch (err: any) {
  console.log(err.name + ":" + err.message);
}

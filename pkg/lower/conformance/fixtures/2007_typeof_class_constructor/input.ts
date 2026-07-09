// typeof a class constructor is "function" in JavaScript: a class value is a
// callable. The class type carries construct signatures rather than call
// signatures, and the fold used to check only call signatures, so it answered
// "object" and this compare went the wrong way. test262 reaches this through the
// harness sta.js self-test (typeof Test262Error === "function").
class C {
  m(): number {
    return 1;
  }
  static make(): C {
    return new C();
  }
}
console.log(typeof C === "function");
console.log(typeof C.make === "function");

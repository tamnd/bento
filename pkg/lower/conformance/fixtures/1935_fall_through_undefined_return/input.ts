// A function whose declared return type is any may run off its end, which in
// JavaScript returns undefined. The switch here returns on one label and falls
// through on the rest, so Go needs the trailing return the body does not spell.
// Before the fall-through emitted return value.Undefined, the generated Go had a
// value-returning function with no final return and did not compile. This is the
// shape formatIdentityFreeValue takes in the test262 prelude: a switch over the
// value kind with no default arm.
function classify(x: string): any {
  switch (x) {
    case "num":
      return 1;
    case "str":
      return "s";
  }
}
console.log(String(classify("num")));
console.log(String(classify("str")));
console.log(String(classify("other")));

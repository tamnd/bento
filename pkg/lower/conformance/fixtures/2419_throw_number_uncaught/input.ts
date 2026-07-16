// JavaScript lets a program throw any value, not only an Error. A thrown number
// that escapes every catch reaches the top-level reporter, which prints its String
// form and exits non-zero, so the statement after the throw never runs. This is the
// shape test262 leans on when a helper throws a plain value to abort a run.
function boom(): never {
  throw 7;
}

console.log("before");
boom();
console.log("after");

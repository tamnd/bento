// JavaScript binds `throw "reason"` as the string primitive, so a catch reads the
// string itself: e === "reason" holds and the compare folds true. The runtime models
// every thrown value as a name and a message for the uncaught reporter, so a caught
// thrown string used to box as a {name, message} object and e === "reason" read
// false. Caught now stashes the primitive and ToValue hands it back, so the strict
// compare over the caught binding matches the string. test262 reaches this in the
// for eval-order tests, which throw a string and catch it to check which operand ran.
let result = "";
try {
  throw "NoInExpression";
} catch (e) {
  if (e !== "NoInExpression") {
    result = "WRONG";
  } else {
    result = "OK";
  }
}
console.log(result);

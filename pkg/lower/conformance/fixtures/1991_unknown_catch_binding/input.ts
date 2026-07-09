// Under strict checking the catch binding is typed unknown, so every use of the
// caught value draws a checker report ("'err' is of type 'unknown'") before the
// body can lower. The resolved type is dynamic exactly like any, so the front
// door tolerates the report and the body lowers through the dynamic value path:
// typeof on the caught value, String() of it, and a boolean-position test all
// cross the boundary and come back with the value the language binds.

function kindOf(): string {
  try {
    throw new TypeError("boom");
  } catch (err) {
    return typeof err;
  }
}
console.log(kindOf());

function messageOf(): string {
  try {
    throw new RangeError("out of range");
  } catch (err) {
    return String(err);
  }
}
console.log(messageOf());

function truthy(): boolean {
  try {
    throw new Error("x");
  } catch (err) {
    if (err) {
      return true;
    }
    return false;
  }
}
console.log(truthy());

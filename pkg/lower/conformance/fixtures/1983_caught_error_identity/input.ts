// A caught error stashed in a dynamic slot and compared back to itself is equal
// by identity: the box is interned on the error, so both references resolve to
// the same object and === holds. A different value is not equal, so the guard a
// helper writes to re-check a stored thrown value answers correctly.

let saved: any = undefined;
try {
  throw new RangeError("oops");
} catch (e: any) {
  saved = e;
  console.log(saved === e);
  console.log(saved === "oops");
}

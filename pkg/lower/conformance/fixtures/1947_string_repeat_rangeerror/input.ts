// String.prototype.repeat throws a RangeError when the count is negative or not
// finite. The runtime used to surface that as a bare Go panic left over from before
// the throw machinery existed, so a try/catch or a test262 assert.throws could not
// catch it and the program aborted. Throwing value.NewRangeError instead, the same
// way every other range-checked runtime method does, makes it a catchable error
// whose name is RangeError and whose message matches V8's "Invalid count value: n".
// The catch binding is typed any so reading e.name and e.message lowers through the
// caught-error path.
try {
  "ab".repeat(-1);
} catch (e: any) {
  console.log(e.name);
  console.log(e.message);
}
try {
  "ab".repeat(Infinity);
} catch (e: any) {
  console.log(e.name);
}
console.log("ok".repeat(2));

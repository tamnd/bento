// A function typed never always throws, so it lowers to a Go func with no
// results: no call site ever reads a value from it, and the panic it raises
// is what an enclosing catch recovers. test262's harness spells this shape
// in $DONOTEVALUATE and Test262Error.thrower.
function fail(reason: string): never {
  throw new Error("fail: " + reason);
}

try {
  fail("early");
} catch (err: any) {
  console.log(err.message);
}
console.log("after");

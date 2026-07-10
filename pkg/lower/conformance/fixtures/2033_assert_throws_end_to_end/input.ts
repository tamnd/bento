// The assert.throws shape every throwing test262 job composes through its
// prelude: run a function, catch what it throws, compare the caught error's
// constructor against the expected one, read both constructor names for the
// message, and rethrow a fresh error when they differ. This is the end-to-end
// proof that the caught-error surface lowers as one body rather than as
// isolated slices, so the whole prelude stops gating a throwing test.
//
// Test262Error is a plain constructor upstream, not an Error subclass, so it
// throws through the runtime's user-thrown path without leaning on the deferred
// extends-Error slice. Its message is required here so the proof stays on the
// caught-error surface rather than the separate optional-parameter defaulting
// slice.
class Test262Error {
  message: string;
  name: string = "Test262Error";
  constructor(message: string) {
    this.message = message;
  }
}

function assertThrows(expectedErrorConstructor: any, func: any): void {
  let message = "";
  try {
    func();
  } catch (thrown: any) {
    if (typeof thrown !== "object" || thrown === null) {
      throw new Test262Error("Thrown value was not an object!");
    } else if (thrown.constructor !== expectedErrorConstructor) {
      const expectedName = expectedErrorConstructor.name;
      const actualName = thrown.constructor.name;
      message += "Expected a " + expectedName + " but got a " + actualName;
      throw new Test262Error(message);
    }
    return;
  }
  message += "Expected a " + expectedErrorConstructor.name + " to be thrown but no exception was thrown at all";
  throw new Test262Error(message);
}

assertThrows(TypeError, function () {
  throw new TypeError("boom");
});
console.log("match ok");

try {
  assertThrows(TypeError, function () {
    throw new RangeError("nope");
  });
} catch (e: any) {
  console.log("mismatch: " + e.name + ": " + e.message);
}

try {
  assertThrows(TypeError, function () {
    return;
  });
} catch (e: any) {
  console.log("none: " + e.message);
}

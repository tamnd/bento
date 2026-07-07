// A caught error is a non-nil error object in the runtime, so typeof over the
// binding folds to "object" and a compare against null or undefined folds to a
// constant. This is the shape assert.throws guards with before it reads the
// error, typeof thrown !== "object" || thrown === null, so it takes the else
// branch for a real thrown error and the error's name and message read back.
try {
  throw new TypeError("boom");
} catch (thrown: any) {
  console.log(typeof thrown === "object");
  console.log(thrown === null);
  console.log(thrown !== null);
  console.log(thrown === undefined);
  if (typeof thrown !== "object" || thrown === null) {
    console.log("not an object");
  } else {
    console.log("object");
  }
  console.log(thrown.name);
  console.log(thrown.message);
}

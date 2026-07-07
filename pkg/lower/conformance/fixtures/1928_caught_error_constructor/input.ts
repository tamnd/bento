// A caught error's .constructor is the interned constructor value for its name, so
// it compares equal by identity to the matching built-in constructor and answers
// that name through .constructor.name. This is the check at the heart of
// assert.throws: after a function throws, it compares the caught error's
// constructor against the expected one and reads both names for the failure
// message.
try {
  throw new TypeError("boom");
} catch (thrown: any) {
  let matches: boolean = thrown.constructor === TypeError;
  console.log(matches);
  let wrong: boolean = thrown.constructor === RangeError;
  console.log(wrong);
  let actualName: string = thrown.constructor.name;
  console.log(actualName);
}
try {
  throw new RangeError("oops");
} catch (thrown: any) {
  let matches: boolean = thrown.constructor === RangeError;
  console.log(matches);
  let actualName: string = thrown.constructor.name;
  console.log(actualName);
}

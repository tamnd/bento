// A caught error coerces to a string through Error.prototype.toString, the name
// then ": " and the message, or the name alone when the message is empty. This is
// the coercion assert.sameValue applies when its comparison throws: it builds a
// failure message by concatenating the caught error onto a string. The three
// spellings, concatenation, String(err), and a template, all take the same
// "Name: message" form.
try {
  throw new TypeError("boom");
} catch (error: any) {
  console.log("caught " + error);
  console.log(String(error));
  console.log(`got ${error}`);
}
try {
  throw new RangeError("");
} catch (error: any) {
  console.log("caught " + error);
}

// toString and valueOf on a string both return the string unchanged, so they are
// identity. A string equals its own toString and its own valueOf, a trailing
// toString on the result of another method leaves that result alone, and valueOf
// on a freshly built string reports the same length the string has.
function run(): void {
  const s = "hello";
  console.log(s.toString() === s);
  console.log(s.valueOf() === s);
  console.log("WORLD".toLowerCase().toString());
  console.log(("a" + "b").valueOf().length);
}

run();

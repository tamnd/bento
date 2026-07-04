// btoa reads each code unit of a binary string as a byte and returns its base64
// encoding, and atob reverses it, decoding base64 back to a binary string. Both
// lower to their value runtime functions over a single string. The cases cover the
// round-trip of a whole sentence, the one, two, and three byte groups that fix the
// padding, and the forgiving decode that accepts input without padding.
function run(): void {
  const s = "Many hands make light work.";
  const e = btoa(s);
  console.log(e);
  console.log(atob(e));
  console.log(atob(e) === s);

  console.log(btoa("M"), btoa("Ma"), btoa("Man"));
  console.log(atob("TWFu"), atob("TWE"));
}

run();

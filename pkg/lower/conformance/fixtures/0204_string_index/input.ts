// A string indexes by UTF-16 code unit: s[i] reads the one-code-unit string at
// i. A direct literal index, a computed index, and a counter loop all read the
// same units, and a two-byte character is one code unit, so the loop rebuilds
// the string exactly. The length-bounded loop keeps a float counter and the
// CharAt read; the literal-bounded loop specializes its counter to an int and
// reads through CharAtI, so both forms are exercised (the counters are named
// apart because a reused name is disqualified from the int specialization).
// Every index here is in range, where the bracket read and charAt agree with
// Node character for character.
function run(): void {
  const s = "héllo";
  console.log(s[0]);
  console.log(s[1]);
  const base = 1;
  console.log(s[base + 2]);
  let out = "";
  for (let i = 0; i < s.length; i++) {
    out += s[i];
  }
  console.log(out);
  let head = "";
  for (let j = 0; j < 3; j++) {
    head += s[j];
  }
  console.log(head);
  console.log(s[s.length - 1]);
}

run();

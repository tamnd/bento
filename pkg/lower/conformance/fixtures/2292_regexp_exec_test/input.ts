// exec returns the match array, whose element zero is the whole match, whose later
// elements are the capture groups, and which carries the .index of the match and the
// .input it ran against; a failed match returns null. test reports the same match as
// a boolean. Under the global flag both resume from lastIndex and advance it, and a
// program may set lastIndex to move the resume point.
const re = /a(b+)c/;
const m = re.exec("xxabbbcyy");
if (m !== null) {
  console.log(m[0]);
  console.log(m[1]);
  console.log(m.index);
  console.log(m.input);
}
console.log(re.test("abbbc"));
console.log(re.test("nope"));

const g = /a/g;
console.log(g.test("aXa"));
console.log(g.lastIndex);
console.log(g.test("aXa"));
console.log(g.lastIndex);
console.log(g.test("aXa"));
console.log(g.lastIndex);

const s = /a/g;
s.lastIndex = 2;
console.log(s.exec("aaaa") !== null);
console.log(s.lastIndex);

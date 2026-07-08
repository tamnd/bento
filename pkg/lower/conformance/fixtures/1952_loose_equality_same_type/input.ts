// Loose == and != on operands the checker types the same are the strict comparison
// without coercion, so bento can lower them to Go's == and !=. Only cross-type
// loose equality needs the coercion machinery; with both sides number or both
// boolean there is nothing to coerce, and NaN and signed zero compare the same in
// Go as in JavaScript. Bento handed these back as a later slice before, so a test
// body using == on two numbers never ran.
function num(x: number): number {
  return x;
}
function boolOf(x: boolean): boolean {
  return x;
}
const a = num(3);
const b = num(3);
const c = num(4);
if (a == b) {
  console.log("num-eq");
}
if (a != c) {
  console.log("num-neq");
}
const t = boolOf(true);
const f = boolOf(false);
if (t == t) {
  console.log("bool-eq");
}
if (t != f) {
  console.log("bool-neq");
}

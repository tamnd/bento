// A ternary whose branches are both string literals coerces to a string. The
// checker types the whole conditional as the literal union of its branches
// ("u" | "d"), which folds no string facet the way a mixed union does, but the
// lowering emits a value.BStr IIFE, so console.log, String(), a template literal,
// and a return read it as the string it is rather than handing back. The presence
// test over an optional feeds the same shape: the comparison is a Go bool and both
// arms are strings, so `n === undefined ? "missing" : "present"` returns a string.
function pick(x: number): void {
  console.log(x > 0 ? "u" : "d");
  console.log(String(x > 0 ? "u" : "d"));
  console.log(`val=${x > 0 ? "u" : "d"}`);
}

// A chained string ternary lowers too: the inner ternary is itself a string, so
// both arms of the outer one read as strings.
function grade(x: number): void {
  console.log(x > 0 ? "pos" : (x < 0 ? "neg" : "zero"));
}

// The item-115 presence test whose result is a string ternary in return position.
function tag(n: number | undefined): string {
  return n === undefined ? "missing" : "present";
}

pick(1);
pick(-1);
grade(1);
grade(0);
grade(-1);
console.log(tag(undefined));
console.log(tag(5));

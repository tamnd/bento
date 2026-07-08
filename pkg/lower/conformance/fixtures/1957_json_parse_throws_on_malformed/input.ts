// JSON.parse throws a SyntaxError on malformed input: a value that does not
// parse, and non-whitespace content after the one top-level value. bento
// returned undefined on a parse error before the throw path landed, so a program
// that relied on the throw ran its success branch instead of its catch. It now
// throws like JavaScript, and well-formed input still parses.
try {
  JSON.parse("{ bad }");
  console.log("no throw");
} catch (e) {
  console.log("threw");
}
try {
  JSON.parse("12 34");
  console.log("no throw");
} catch (e) {
  console.log("threw");
}
console.log(String(JSON.parse("42")));
console.log(String(JSON.parse(" [1, 2, 3] ").length));

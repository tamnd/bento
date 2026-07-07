// A helper function keeps a local named the same as a top-level dynamic binding.
// The top-level result has no initializer, so its Go slot is a boxed value.Value,
// while the helper's result is a static string. The boxed-locals pre-pass must scope
// each name to its own function: it used to count the two result declarations as one
// redeclared name and drop the top-level result, so its narrowed read stayed a bare
// box that then double-boxed when it flowed into an any parameter and failed go build.
function tag(v: any): string {
  var result = "<" + String(v) + ">";
  return result;
}
var result;
result = "abc".replaceAll("b", "$$");
console.log(tag(result));
result = "hello".replaceAll("l", "L");
console.log(tag(result));

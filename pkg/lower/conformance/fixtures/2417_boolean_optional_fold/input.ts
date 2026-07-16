// TypeScript models boolean as the pair true | false, so boolean | undefined is the
// three members true | false | undefined. The two boolean members fold to boolean,
// whose Go slot is bool, so a boolean optional lowers to a value.Opt[bool] the presence
// shapes read: the undefined-compare, a narrowed read, typeof, and truthiness, where an
// absent option and a present false are both falsy.
function f(x?: boolean): string {
  return x === undefined ? "none" : x ? "yes" : "no";
}
function g(x: boolean | undefined): string {
  if (x === undefined) {
    return "u";
  }
  return x ? "T" : "F";
}
function h(x?: boolean): string {
  return typeof x === "boolean" ? "b" : "u";
}
function truthy(x?: boolean): string {
  return x ? "y" : "n";
}
console.log(f(), f(true), f(false));
console.log(g(undefined), g(true), g(false));
console.log(h(), h(true));
console.log(truthy(), truthy(true), truthy(false));
let b: boolean | undefined;
console.log(b === undefined ? "u" : "d");
b = true;
console.log(b === undefined ? "u" : "d");

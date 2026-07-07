// A function expression tracks its own any-typed parameter as a boxed value the
// way a named function does, so a read the checker narrows past a typeof guard
// unboxes through the matching accessor. Before the nested body scoped its own
// dynamic locals, only the enclosing function's set was visible, so a nested
// parameter stayed a bare box and a narrowed read emitted Go that did not
// compile. This is the shape assert.compareArray takes: its message parameter is
// narrowed inside a typeof guard in a function assigned to a member.
const classify = function (x: any): string {
  if (typeof x === "number") {
    let doubled: number = x * 2;
    return "num " + doubled;
  }
  if (typeof x === "string") {
    return "str " + x.length;
  }
  return "other";
};
console.log(classify(21));
console.log(classify("hi"));
console.log(classify(true));

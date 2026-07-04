// Object.hasOwn(o, key) on a fixed-shape object with a string-literal key folds
// at compile time: the shape names every own key, so a key that matches a
// required field is always present and folds to true, and a key the shape does
// not have folds to false. An optional field, whose presence is not known
// statically, would hand back instead.
function run(): void {
  const o = { a: 1, b: "x", c: true };
  console.log(o.a);
  console.log(Object.hasOwn(o, "a"));
  console.log(Object.hasOwn(o, "b"));
  console.log(Object.hasOwn(o, "c"));
  console.log(Object.hasOwn(o, "z"));
}

run();

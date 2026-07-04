// Object.values on a fixed-shape object returns its own enumerable property
// values in declaration order. This lowers when the values share one type, so
// the array they gather into has a single element type. A shape whose field
// types differ would need a mixed-element array and is a later slice.
function run(): void {
  const nums = { a: 10, b: 20, c: 30 };
  console.log(Object.values(nums).join(","));

  const words = { first: "hello", second: "world" };
  console.log(Object.values(words).join(" "));

  // the values keep declaration order, not sorted order
  const scores = { z: 3, a: 1, m: 2 };
  console.log(Object.values(scores).join(","));
}

run();

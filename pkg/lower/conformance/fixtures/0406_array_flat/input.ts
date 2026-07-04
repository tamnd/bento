// flat at its default depth of one concatenates the inner arrays of an array of
// arrays into a single flat array. It returns a fresh array and leaves the inner
// arrays alone. This lowers for an array whose element type is itself an array;
// deeper depths and a mixed array of arrays and values are later slices.
function run(): void {
  const nums = [[1, 2], [3], [4, 5, 6]];
  console.log(nums.flat().join(","));

  const words = [["a", "b"], ["c"], ["d", "e"]];
  console.log(words.flat().join(""));

  // the inner arrays are untouched by flat, since it copies into a new array
  const inner = [10, 20];
  const nested = [inner, [30]];
  nested.flat();
  console.log(inner.join(","));
}

run();

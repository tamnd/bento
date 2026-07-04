// with is the copying single-index write: it returns a new array with the
// element at the index replaced by the value and leaves the receiver alone,
// where a[i] = v mutates in place. A negative index counts from the end. An
// index outside the array throws a RangeError, but this fixture stays in range.
function run(): void {
  const nums = [1, 2, 3, 4];

  // replace the element at index one
  console.log(nums.with(1, 99).join(","));

  // a negative index counts from the end
  console.log(nums.with(-1, 88).join(","));

  // the source array is untouched, since with copies
  console.log(nums.join(","));

  const words = ["a", "b", "c"];
  console.log(words.with(0, "z").join(""));
}

run();

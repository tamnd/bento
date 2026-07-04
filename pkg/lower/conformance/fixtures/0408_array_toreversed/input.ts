// toReversed is the copying sibling of reverse: it returns a new array with the
// elements in reverse order and leaves the receiver in its original order, where
// reverse reorders in place. It takes no arguments and lowers for any array.
function run(): void {
  const nums = [1, 2, 3, 4];
  console.log(nums.toReversed().join(","));

  // the source array is untouched, since toReversed copies
  console.log(nums.join(","));

  const words = ["a", "b", "c"];
  console.log(words.toReversed().join(""));
}

run();

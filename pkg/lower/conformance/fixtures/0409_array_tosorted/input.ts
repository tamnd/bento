// toSorted is the copying sibling of sort: it orders a fresh copy of the array
// by the comparator and leaves the receiver in its original order, where sort
// reorders in place. It lowers when the comparator is an inline arrow of two
// parameters, the same shape sort takes.
function run(): void {
  const nums = [3, 1, 4, 1, 5, 9, 2, 6];

  // ascending by comparator
  console.log(nums.toSorted((x, y) => x - y).join(","));

  // the source array is untouched, since toSorted copies
  console.log(nums.join(","));

  // descending by flipping the comparator
  console.log(nums.toSorted((x, y) => y - x).join(","));
}

run();

// toSpliced is the copying sibling of splice: it returns the array that results
// from removing a run and inserting items in its place, and leaves the receiver
// alone, where splice mutates and returns the removed elements. The one-argument
// form removes everything from start to the end. A negative start counts from
// the end, the same way splice reads it.
function run(): void {
  const nums = [1, 2, 3, 4, 5];

  // remove two elements at index one and insert three in their place
  console.log(nums.toSpliced(1, 2, 20, 30, 40).join(","));

  // one-argument form removes from index two to the end
  console.log(nums.toSpliced(2).join(","));

  // negative start counts from the end, replacing the second-to-last element
  console.log(nums.toSpliced(-2, 1, 99).join(","));

  // the source array is untouched, since toSpliced copies
  console.log(nums.join(","));
}

run();

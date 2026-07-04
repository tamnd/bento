// copyWithin copies a block of the array to another position in the same array,
// in place, without changing the length. The target and source bounds are
// relative indices, so a negative one counts from the end, and an overlapping
// copy reads the source range before writing it. It returns the same array it
// mutated, so the change is visible through the original binding.
function run(): void {
  console.log([1, 2, 3, 4, 5].copyWithin(0, 3).join(","));
  console.log([1, 2, 3, 4, 5].copyWithin(0, 3, 4).join(","));
  console.log([1, 2, 3, 4, 5].copyWithin(-2, 0).join(","));
  console.log([1, 2, 3, 4, 5].copyWithin(2, 0).join(","));

  // the mutation is visible through the original binding, since copyWithin
  // changes the array in place rather than returning a copy
  const a = [1, 2, 3, 4, 5];
  a.copyWithin(1, 3);
  console.log(a.join(","));
}

run();

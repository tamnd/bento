// flatMap maps each element to an array and concatenates the results one level
// deep, so it can grow, shrink, or reshape an array in a single pass. It lowers
// when the callback is an inline arrow whose body returns an array; a callback
// returning a bare value is a later slice. The inner element type can differ
// from the source, which flatMap spells as its second type argument.
function run(): void {
  const nums = [1, 2, 3];

  // expand each element into a pair, doubling the length
  console.log(nums.flatMap((n) => [n, -n]).join(","));

  // expand each element into a longer run
  console.log(nums.flatMap((n) => [n, n * 10, n * 100]).join(","));

  // map to a different element type, one number to two strings
  console.log([1, 2].flatMap((n) => [String(n), String(-n)]).join(","));
}

run();

// Map.groupBy lowers the grouping loop from an inline arrow, whose parameters and
// body it reads directly to bind the item and, when present, the index. A callback
// passed as a named reference carries neither in a form the loop can read yet: it
// would need the reference resolved to a func value and its arity recovered before
// the loop could call it, so a named callback hands the whole call back rather than
// mislower the grouping. The inline-arrow form is the covered shape.
function bucket(n: number): string {
  return n % 2 === 0 ? "even" : "odd";
}

function run(): void {
  const nums = [1, 2, 3, 4];
  const grouped = Map.groupBy(nums, bucket);
  console.log(grouped.size);
}

run();

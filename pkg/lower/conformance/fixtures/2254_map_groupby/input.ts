// Map.groupBy(items, callback) groups an iterable's items into a Map keyed by the
// value the callback returns, each key holding the array of items that produced it,
// in first-seen key order and source order within a group. The callback may read
// the item alone or the item and its index. The result is a real Map, so its keys,
// values, and entries iterate the way any other Map's do. A string key groups by a
// derived label; a number key groups by a bucket the index feeds.
function run(): void {
  const nums = [1, 2, 3, 4, 5, 6, 7];

  const byParity = Map.groupBy(nums, (n): string => {
    return n % 2 === 0 ? "even" : "odd";
  });
  for (const [label, group] of byParity) {
    console.log(label + ": " + group.join(","));
  }

  const byIndexBucket = Map.groupBy(nums, (n, i) => i % 3);
  for (const [bucket, group] of byIndexBucket) {
    console.log(bucket + " -> " + group.join(","));
  }
}

run();

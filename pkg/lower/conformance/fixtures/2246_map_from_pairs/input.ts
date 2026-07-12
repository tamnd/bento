// new Map([[k, v], ...]) fills the map from an array literal of pairs: each inner
// pair's first element is the key and second the value. A repeated key keeps its
// first position and takes the last value, matching the specification's left-to-
// right insertion, so [3, "c"] then [3, "C"] leaves one entry that reads "C".
function run(): void {
  const m = new Map<number, string>([[1, "a"], [2, "b"], [3, "c"], [3, "C"]]);
  console.log(m.size);
  console.log(m.has(2));
  console.log(m.has(4));
  const v = m.get(3);
  if (v !== undefined) {
    console.log(v);
  }
}

run();

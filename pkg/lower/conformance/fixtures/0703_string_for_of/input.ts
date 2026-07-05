// A for...of over a string iterates its Unicode code points, one substring per
// code point, so it lowers to a range over the string's code points the same way
// an array for...of ranges over its elements. An astral character (the emoji here)
// is a single code point of two code units, so it counts as one iteration, not two,
// which is the difference from iterating code units. A loop that never reads its
// binding, the counting idiom, ranges with no loop variable so the generated Go
// compiles, and that rule covers an array for...of too.
function run(): void {
  let joined = "";
  for (const c of "abc") {
    joined = joined + c + "-";
  }
  console.log(joined);

  let points = 0;
  for (const c of "a\u{1F600}b") {
    points = points + 1;
  }
  console.log(points);

  let letters = 0;
  for (const c of "hello") letters = letters + 1;
  console.log(letters);

  let sum = 0;
  for (const x of [10, 20, 30]) sum = sum + 1;
  console.log(sum);
}

run();

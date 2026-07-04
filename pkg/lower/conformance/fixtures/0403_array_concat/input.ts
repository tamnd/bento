// concat returns a new array. It spreads array arguments one level and appends
// non-array arguments as single elements, so an array argument contributes its
// elements and a value argument contributes itself. The receiver is never
// changed, since concat reads its sources into a fresh array.
function run(): void {
  const a = [1, 2, 3];
  const b = [4, 5];

  console.log(a.concat(b).join(","));
  console.log(a.concat(6).join(","));
  console.log(a.concat(b, 7, 8).join(","));
  console.log(a.concat().join(","));

  // the receiver is untouched by any of the concats above
  console.log(a.join(","));

  const words = ["a", "b"];
  console.log(words.concat("c", ["d", "e"]).join(""));
}

run();

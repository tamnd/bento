// An assignment-form array destructure whose source is a tuple reads each target's
// element as the interned struct's field, [label, value] = pair becoming
// label, value = pair.E0, pair.E1, the read-into-existing-locals sibling of the
// const [a, b] = pair bind. It is a pure read of the tuple, so a pattern that binds
// fewer names than the tuple has and a heterogeneous tuple both read the right
// positions, and the swap idiom falls out because Go evaluates every right side
// before it assigns any target.
function run(): void {
  const pair: [string, number] = ["k", 7];
  let a = "";
  let b = 0;
  [a, b] = pair;
  console.log(a + ":" + b);

  // A pattern that binds fewer names than the tuple has reads only the leading
  // positions.
  let first = "";
  [first] = pair;
  console.log(first);

  // A three-element heterogeneous tuple reads each positional field in turn.
  const trip: [number, string, boolean] = [1, "two", true];
  let n = 0;
  let s = "";
  let flag = false;
  [n, s, flag] = trip;
  console.log(n + s + flag);

  // The swap idiom falls out of the parallel assignment.
  let x = 10;
  let y = 20;
  const swap: [number, number] = [y, x];
  [x, y] = swap;
  console.log(x + "," + y);
}

run();

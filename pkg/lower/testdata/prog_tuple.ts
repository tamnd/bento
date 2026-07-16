// A tuple program exercises the positional-struct lowering end to end: a function
// returns a tuple, a literal builds one, a read selects a position, and a
// destructure binds each position from a call source. The TypeScript and the
// generated Go must print the identical lines.

function minmax(xs: number[]): [number, number] {
  let lo = xs[0];
  let hi = xs[0];
  for (const x of xs) {
    if (x < lo) lo = x;
    if (x > hi) hi = x;
  }
  return [lo, hi];
}

const pair: [string, number] = ["age", 42];
console.log(pair[0]);
console.log(pair[1]);

const [label, value] = pair;
console.log(label);
console.log(value);

const [lo, hi] = minmax([3, 1, 4, 1, 5, 9, 2, 6]);
console.log(lo);
console.log(hi);

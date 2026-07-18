// A C-style for loop whose initializer is a destructuring pattern binds the pattern
// once, before the loop runs, so it lowers into the block that wraps the loop and the
// loop itself takes an empty init clause. An array pattern reads each element off the
// tuple or array source by position, an object pattern reads each field off the struct
// source, and the bound names stay in scope for the condition, body, and post clause.
let steps = 0;
for (let [i, j] = [0, 6]; i < j; i++) {
  steps += i;
}
console.log(String(steps));

let ticks = 0;
for (let { lo, hi } = { lo: 2, hi: 5 }; lo < hi; lo++) {
  ticks += lo;
}
console.log(String(ticks));

const bounds: number[] = [1, 4];
let acc = 0;
for (let [start, stop] = bounds; start < stop; start++) {
  acc += start;
}
console.log(String(acc));

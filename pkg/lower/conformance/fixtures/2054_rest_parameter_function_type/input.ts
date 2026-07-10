// A rest-parameter function type lowers to a plain Go func value whose trailing
// argument is the *value.Array[T] a rest parameter gathers, not a Go variadic, so a
// callback typed (...a: number[]) => number reads as one array field and the call
// site packs its trailing arguments into that array. run takes such a callback and
// calls it with three numbers, which pack into the array the callback reads. total
// is a pure rest-parameter function, so its own Go form is that same one-array shape
// and it passes into the slot by name with no defaulting wrapper.
function total(...xs: number[]): number {
  let s = 0;
  for (const x of xs) {
    s = s + x;
  }
  return s;
}

function run(f: (...a: number[]) => number): number {
  return f(1, 2, 3);
}

console.log(run(total));

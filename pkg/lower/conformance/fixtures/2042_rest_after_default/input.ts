// A rest parameter that follows a default parameter gathers the trailing arguments
// after the defaulted slot is filled. The call site fills the omitted default and
// packs whatever is left into the rest array, so the body reads its default and its
// gathered rest with no special casing. This proves the default-then-rest form end to
// end.
function f(a: number, b: number = 1, ...rest: number[]): number {
  let s = a + b;
  for (const r of rest) {
    s = s + r;
  }
  return s;
}

// b defaults to 1 and rest is empty: 1 + 1 is 2.
console.log(f(1));

// b is supplied and rest is empty: 1 + 2 is 3.
console.log(f(1, 2));

// b is supplied and rest gathers the tail: 1 + 2 + 3 + 4 is 10.
console.log(f(1, 2, 3, 4));

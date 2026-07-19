// JavaScript hoists a nested function declaration, so a call may sit before it in
// the source and still resolve. Go binds the closure to a local from its
// declaration onward, so calling it earlier would read an unbound local, which does
// not compile. The lowering declines this shape and runs the unit on the engine,
// where the source's hoisting holds.

function total(xs: number[]): number {
  // The call sits above the declaration it names, the one shape the local binding
  // cannot back.
  const first = pick(xs);
  function pick(a: number[]): number {
    return a.length === 0 ? 0 : a[0];
  }
  return first;
}

console.log(total([7, 8, 9]));

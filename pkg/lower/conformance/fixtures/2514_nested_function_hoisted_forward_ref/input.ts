// JavaScript hoists a function declaration and binds it at the top of its scope, so
// two helpers in the same scope may name each other whichever order they are written
// in. Go binds a closure to a local from its declaration onward, but a closure only
// reads what it captured when it runs, and a helper named from inside another
// helper's body runs after the whole block has finished binding. So the Go local is
// declared at the top of the block and the closure is assigned at the declaration's
// own position, which is the source's hoisting exactly.
//
// A call that runs while the block runs is the other case: the binding moves up to
// just before the statement that calls it, which is as early as it can go while
// still seeing everything the closure reads. A helper that reads a local declared
// below that point has nowhere to go and hands back; that is fixture 2464.

function describe(n: number): string {
  // Names a helper declared below it. The read happens when summarize is called,
  // which is after both bindings exist.
  function summarize(x: number): string {
    return classify(x) + " " + parity(x);
  }
  function classify(x: number): string {
    return x < 0 ? "negative" : "nonnegative";
  }
  // Mutual reference the other way: parity is named above and names classify below
  // itself, so both directions are covered in one scope.
  function parity(x: number): string {
    return x % 2 === 0 ? "even" : "odd";
  }
  return summarize(n);
}

function scaled(n: number): number {
  // The factor is declared above, so the helper can bind above this call.
  const factor = 10;
  // A call that runs while the block runs. adjust binds here, ahead of it, not at
  // its own declaration below.
  const first = adjust(n);
  function adjust(x: number): number {
    return x * factor;
  }
  return first;
}

console.log(describe(4));
console.log(describe(-3));
console.log(scaled(5));

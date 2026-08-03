// JavaScript hoists a nested function declaration and binds it at the top of its
// scope, so a call may sit above it and still resolve. Go binds a closure to a local
// and a closure captures lexically, so the lowering answers that by moving the
// binding up to just before the statement that calls it, which is fixture 2514.
//
// There is a limit to how far up it can go. This helper reads a local the source
// declares below the call, so binding it above the call would capture a Go variable
// that does not exist yet. JavaScript has no such limit, so the lowering declines the
// shape and runs the unit on the engine, where the source's hoisting holds.

function total(xs: number[]): number {
  // The call sits above the declaration it names.
  const first = pick(xs);
  // And the helper reads this, which is declared below the call, so the binding has
  // nowhere to go that sees both.
  const fallback = -1;
  function pick(a: number[]): number {
    return a.length === 0 ? fallback : a[0];
  }
  return first;
}

console.log(total([7, 8, 9]));
